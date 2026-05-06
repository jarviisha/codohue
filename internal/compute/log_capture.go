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
// It is safe for concurrent use.
type LogCapture struct {
	mu      sync.Mutex
	entries []LogEntry
}

func (c *LogCapture) Info(msg string)  { c.add("info", msg) }
func (c *LogCapture) Warn(msg string)  { c.add("warn", msg) }
func (c *LogCapture) Error(msg string) { c.add("error", msg) }

func (c *LogCapture) add(level, msg string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = append(c.entries, LogEntry{
		Ts:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Level: level,
		Msg:   msg,
	})
}

// Entries returns a snapshot of all captured entries.
func (c *LogCapture) Entries() []LogEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]LogEntry, len(c.entries))
	copy(out, c.entries)
	return out
}
