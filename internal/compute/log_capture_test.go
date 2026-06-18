package compute

import (
	"sync"
	"testing"
)

func TestLogCapture_RecordsAllLevels(t *testing.T) {
	var c LogCapture
	c.Info("starting")
	c.Warn("careful")
	c.Error("boom")

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

func TestLogCapture_EntriesReturnsCopy(t *testing.T) {
	var c LogCapture
	c.Info("one")

	snapshot := c.Entries()
	snapshot[0].Msg = "mutated"

	if got := c.Entries()[0].Msg; got != "one" {
		t.Errorf("Entries should return a copy; underlying entry changed to %q", got)
	}
}

func TestLogCapture_OnEntryCallback(t *testing.T) {
	var c LogCapture
	var seen []LogEntry
	c.SetOnEntry(func(e LogEntry) { seen = append(seen, e) })

	c.Info("hello")
	c.Error("world")

	if len(seen) != 2 {
		t.Fatalf("callback fired %d times, want 2", len(seen))
	}
	if seen[0].Msg != "hello" || seen[1].Msg != "world" {
		t.Errorf("callback payloads: got %q, %q", seen[0].Msg, seen[1].Msg)
	}

	// Clearing the callback stops further notifications.
	c.SetOnEntry(nil)
	c.Info("ignored")
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
	c.Info("info")
	c.Warn("warn")
	c.Error("error")
}

func TestLogCapture_ConcurrentAddAndRead(t *testing.T) {
	var c LogCapture
	const writers = 8
	const perWriter = 50

	var wg sync.WaitGroup
	wg.Add(writers)
	for i := 0; i < writers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perWriter; j++ {
				c.Info("msg")
				_ = c.Entries()
			}
		}()
	}
	wg.Wait()

	if got := len(c.Entries()); got != writers*perWriter {
		t.Errorf("Entries after concurrent writes: got %d, want %d", got, writers*perWriter)
	}
}
