package compute

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/batchrun"
)

type fakeEventSink struct {
	channel  string
	messages []string
	err      error
}

func (f *fakeEventSink) Publish(_ context.Context, channel string, message any) *redis.IntCmd {
	f.channel = channel
	if raw, ok := message.([]byte); ok {
		f.messages = append(f.messages, string(raw))
	}
	cmd := redis.NewIntCmd(context.Background())
	if f.err != nil {
		cmd.SetErr(f.err)
	}
	return cmd
}

func (f *fakeEventSink) decode(t *testing.T, i int) BatchRunEvent {
	t.Helper()
	var ev BatchRunEvent
	if err := json.Unmarshal([]byte(f.messages[i]), &ev); err != nil {
		t.Fatalf("decode event %d: %v", i, err)
	}
	return ev
}

func TestRedisBatchRunObserver_PublishesEveryCallback(t *testing.T) {
	sink := &fakeEventSink{}
	obs := newRedisBatchRunObserverWithSink(sink)

	obs.OnRunStarted(7, "ns1", batchrun.TriggerCron)
	obs.OnPhaseStarted(7, "ns1", 2)
	obs.OnPhaseCompleted(7, "ns1", 2, PhaseResult{OK: true, DurationMs: 12, Count1: 3, Count2: 4})
	obs.OnLogLine(7, "ns1", LogEntry{Ts: "2026-07-22T10:00:00Z", Level: "INFO", Msg: "hello"})
	obs.OnRunCompleted(7, "ns1", true, "")
	obs.OnRunCancelled(7, "ns1")

	if sink.channel != BatchRunEventChannel {
		t.Errorf("channel: got %q, want %q", sink.channel, BatchRunEventChannel)
	}
	wantKinds := []string{"started", "phase_started", "phase_completed", "log_line", "completed", "cancelled"}
	if len(sink.messages) != len(wantKinds) {
		t.Fatalf("published %d events, want %d", len(sink.messages), len(wantKinds))
	}
	for i, want := range wantKinds {
		ev := sink.decode(t, i)
		if ev.Kind != want {
			t.Errorf("event %d kind: got %q, want %q", i, ev.Kind, want)
		}
		if ev.RunID != 7 || ev.Namespace != "ns1" {
			t.Errorf("event %d identity: %+v", i, ev)
		}
	}

	phase := sink.decode(t, 2)
	if phase.PhaseOK == nil || !*phase.PhaseOK || phase.Count1 != 3 || phase.Count2 != 4 {
		t.Errorf("phase_completed payload: %+v", phase)
	}
	logLine := sink.decode(t, 3)
	if logLine.LogMsg != "hello" || logLine.LogLevel != "INFO" {
		t.Errorf("log_line payload: %+v", logLine)
	}
}

func TestRedisBatchRunObserver_PublishFailureIsNonFatal(t *testing.T) {
	sink := &fakeEventSink{err: errors.New("redis down")}
	obs := newRedisBatchRunObserverWithSink(sink)
	obs.OnRunCompleted(1, "ns", false, "boom") // must not panic
	if len(sink.messages) != 1 {
		t.Fatalf("expected one publish attempt, got %d", len(sink.messages))
	}
}

func TestNewRedisBatchRunObserver_NilClientYieldsNil(t *testing.T) {
	if obs := NewRedisBatchRunObserver(nil); obs != nil {
		t.Fatal("a nil redis client must yield a nil observer so callers can wire it unconditionally")
	}
}
