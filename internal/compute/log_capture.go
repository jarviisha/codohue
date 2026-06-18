package compute

import (
	"sync"
	"time"
)

// LogEntry is a single captured log line from a batch run.
type LogEntry struct {
	Ts    string `json:"ts"`
	Level string `json:"level"`
	Msg   string `json:"msg"`
}

// LogCapture accumulates structured log entries during a batch run.
// It is safe for concurrent use. An optional onEntry callback fires after the
// entry is appended — used by the admin SSE stream to forward log lines in
// real time without coupling capture to the event bus.
type LogCapture struct {
	mu      sync.Mutex
	entries []LogEntry
	onEntry func(LogEntry)
}

// SetOnEntry registers a callback invoked after every captured entry. Pass
// nil to clear. Safe to call before any Info/Warn/Error.
func (c *LogCapture) SetOnEntry(fn func(LogEntry)) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onEntry = fn
}

// Info records an informational log entry.
func (c *LogCapture) Info(msg string) { c.add("info", msg) }

// Warn records a warning log entry.
func (c *LogCapture) Warn(msg string) { c.add("warn", msg) }

// Error records an error log entry.
func (c *LogCapture) Error(msg string) { c.add("error", msg) }

func (c *LogCapture) add(level, msg string) {
	if c == nil {
		return
	}
	entry := LogEntry{
		Ts:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Level: level,
		Msg:   msg,
	}
	c.mu.Lock()
	c.entries = append(c.entries, entry)
	cb := c.onEntry
	c.mu.Unlock()
	if cb != nil {
		cb(entry)
	}
}

// Entries returns a snapshot of all captured entries.
func (c *LogCapture) Entries() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]LogEntry, len(c.entries))
	copy(out, c.entries)
	return out
}
