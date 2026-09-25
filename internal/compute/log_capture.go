package compute

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

// LogEntry is a single captured log line from a batch run.
type LogEntry struct {
	Ts    string `json:"ts"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// LogCapture is a slog.Handler that accumulates structured log entries during
// a batch run. It is safe for concurrent use. An optional onEntry callback
// fires after the entry is appended — used by the admin SSE stream to forward
// log lines in real time without coupling capture to the event bus.
type LogCapture struct {
	mu      sync.Mutex
	entries []LogEntry
	onEntry func(LogEntry)
}

var _ slog.Handler = (*LogCapture)(nil)

// SetOnEntry registers a callback invoked after every captured entry. Pass
// nil to clear. Safe to call before any logging.
func (c *LogCapture) SetOnEntry(fn func(LogEntry)) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onEntry = fn
}

// Enabled implements slog.Handler; every level is captured.
func (c *LogCapture) Enabled(context.Context, slog.Level) bool { return true }

// WithAttrs implements slog.Handler. LogEntry has no attribute fields, so the
// handler is returned unchanged.
func (c *LogCapture) WithAttrs([]slog.Attr) slog.Handler { return c }

// WithGroup implements slog.Handler. LogEntry has no group fields, so the
// handler is returned unchanged.
func (c *LogCapture) WithGroup(string) slog.Handler { return c }

// Handle implements slog.Handler: it appends a LogEntry and fires the onEntry
// callback after the append, outside the lock.
func (c *LogCapture) Handle(_ context.Context, r slog.Record) error {
	if c == nil {
		return nil
	}
	entry := LogEntry{
		Ts:    r.Time.UTC().Format("2006-01-02T15:04:05.000Z"),
		Level: strings.ToLower(r.Level.String()),
		Msg:   r.Message,
	}
	c.mu.Lock()
	c.entries = append(c.entries, entry)
	cb := c.onEntry
	c.mu.Unlock()
	if cb != nil {
		cb(entry)
	}
	return nil
}

// Entries returns a snapshot of all captured entries.
func (c *LogCapture) Entries() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]LogEntry, len(c.entries))
	copy(out, c.entries)
	return out
}
