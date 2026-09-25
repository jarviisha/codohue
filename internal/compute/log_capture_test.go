package compute

import (
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
)

func TestLogCapture_RecordsAllLevels(t *testing.T) {
	var c LogCapture
	log := slog.New(&c)
	log.Info("starting")
	log.Warn("careful")
	log.Error("boom")

	entries := c.Entries()
	if len(entries) != 3 {
		t.Fatalf("Entries: got %d, want 3", len(entries))
	}
	want := []struct{ level, msg string }{
		{"info", "starting"},
		{"warn", "careful"},
		{"error", "boom"},
	}
	for i, w := range want {
		if entries[i].Level != w.level || entries[i].Msg != w.msg {
			t.Errorf("entry %d: got (%s, %s), want (%s, %s)",
				i, entries[i].Level, entries[i].Msg, w.level, w.msg)
		}
		if entries[i].Ts == "" {
			t.Errorf("entry %d: timestamp is empty", i)
		}
	}
}

func TestLogCapture_WireFormatUnchanged(t *testing.T) {
	var c LogCapture
	slog.New(&c).Info("hello")

	raw, err := json.Marshal(c.Entries()[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(m) != 3 || m["level"] != "info" || m["msg"] != "hello" {
		t.Errorf("wire shape changed: %s", raw)
	}
	// RFC3339 with milliseconds and a literal Z, e.g. 2026-09-25T10:00:00.000Z.
	ts := m["ts"]
	if len(ts) != len("2006-01-02T15:04:05.000Z") || ts[len(ts)-1] != 'Z' || ts[len(ts)-5] != '.' {
		t.Errorf("ts format changed: %q", ts)
	}
}

func TestLogCapture_EntriesReturnsCopy(t *testing.T) {
	var c LogCapture
	slog.New(&c).Info("one")

	snapshot := c.Entries()
	snapshot[0].Msg = "mutated"

	if got := c.Entries()[0].Msg; got != "one" {
		t.Errorf("Entries should return a copy; underlying entry changed to %q", got)
	}
}

func TestLogCapture_OnEntryCallback(t *testing.T) {
	var c LogCapture
	log := slog.New(&c)
	var seen []LogEntry
	c.SetOnEntry(func(e LogEntry) { seen = append(seen, e) })

	log.Info("hello")
	log.Error("world")

	if len(seen) != 2 {
		t.Fatalf("callback fired %d times, want 2", len(seen))
	}
	if seen[0].Msg != "hello" || seen[1].Msg != "world" {
		t.Errorf("callback payloads: got %q, %q", seen[0].Msg, seen[1].Msg)
	}

	// Clearing the callback stops further notifications.
	c.SetOnEntry(nil)
	log.Info("ignored")
	if len(seen) != 2 {
		t.Errorf("callback fired after being cleared: %d entries", len(seen))
	}
	if got := len(c.Entries()); got != 3 {
		t.Errorf("entries should still accumulate after clearing callback: got %d, want 3", got)
	}
}

func TestLogCapture_NilReceiverIsSafe(t *testing.T) {
	var c *LogCapture
	// None of these should panic on a nil receiver.
	c.SetOnEntry(func(LogEntry) {})
	log := slog.New(c)
	log.Info("info")
	log.Warn("warn")
	log.Error("error")
}

func TestLogCapture_ConcurrentAddAndRead(t *testing.T) {
	var c LogCapture
	log := slog.New(&c)
	const writers = 8
	const perWriter = 50

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perWriter; j++ {
				log.Info("msg")
				_ = c.Entries()
			}
		}()
	}
	wg.Wait()

	if got := len(c.Entries()); got != writers*perWriter {
		t.Errorf("Entries after concurrent writes: got %d, want %d", got, writers*perWriter)
	}
}
