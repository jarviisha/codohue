package compute

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/batchrun"
)

// BatchRunEventChannel is the Redis pub/sub channel batch-run lifecycle
// events are published on. cmd/admin's bridge subscribes and republishes onto
// its in-process event bus so SSE handlers can fan runs out to operators.
//
// The channel exists because cron runs in a separate process from the admin
// server: an in-process observer only ever sees admin-triggered runs, so a
// cron run's stream emitted nothing but heartbeats.
const BatchRunEventChannel = "codohue:batchrun-events"

// BatchRunEvent is the JSON payload published for every observer callback.
// One envelope for all kinds keeps the bridge's decode path single-shaped;
// fields not relevant to a kind are omitted.
type BatchRunEvent struct {
	Kind          string `json:"kind"` // started | phase_started | phase_completed | log_line | completed | cancelled
	RunID         int64  `json:"run_id"`
	Namespace     string `json:"namespace"`
	TriggerSource string `json:"trigger_source,omitempty"`
	Phase         int    `json:"phase,omitempty"`
	PhaseOK       *bool  `json:"phase_ok,omitempty"`
	DurationMs    int    `json:"duration_ms,omitempty"`
	Count1        int    `json:"count1,omitempty"`
	Count2        int    `json:"count2,omitempty"`
	Success       *bool  `json:"success,omitempty"`
	ErrorMessage  string `json:"error_message,omitempty"`
	LogTs         string `json:"log_ts,omitempty"`
	LogLevel      string `json:"log_level,omitempty"`
	LogMsg        string `json:"log_msg,omitempty"`
}

// batchRunEventSink is the subset of *redis.Client the observer needs.
type batchRunEventSink interface {
	Publish(ctx context.Context, channel string, message any) *redis.IntCmd
}

// redisBatchRunObserver implements [BatchRunObserver] by publishing each
// callback to Redis pub/sub. Publish failures are logged and dropped —
// live-stream delivery is observability, never a reason to fail a run.
type redisBatchRunObserver struct {
	rdb batchRunEventSink
}

// NewRedisBatchRunObserver wraps a Redis client as a BatchRunObserver.
// A nil client yields nil so callers can wire it unconditionally.
func NewRedisBatchRunObserver(rdb *redis.Client) BatchRunObserver {
	if rdb == nil {
		return nil
	}
	return &redisBatchRunObserver{rdb: rdb}
}

func newRedisBatchRunObserverWithSink(sink batchRunEventSink) BatchRunObserver {
	return &redisBatchRunObserver{rdb: sink}
}

// OnRunStarted publishes the run-started event.
func (o *redisBatchRunObserver) OnRunStarted(runID int64, ns string, src batchrun.TriggerSource) {
	o.publish(BatchRunEvent{
		Kind: "started", RunID: runID, Namespace: ns, TriggerSource: string(src),
	})
}

// OnPhaseStarted publishes a phase-started event.
func (o *redisBatchRunObserver) OnPhaseStarted(runID int64, ns string, phase int) {
	o.publish(BatchRunEvent{Kind: "phase_started", RunID: runID, Namespace: ns, Phase: phase})
}

// OnPhaseCompleted publishes a phase-completed event with its counts.
func (o *redisBatchRunObserver) OnPhaseCompleted(runID int64, ns string, phase int, result PhaseResult) {
	ok := result.OK
	o.publish(BatchRunEvent{
		Kind: "phase_completed", RunID: runID, Namespace: ns, Phase: phase,
		PhaseOK: &ok, DurationMs: result.DurationMs,
		Count1: result.Count1, Count2: result.Count2, ErrorMessage: result.Error,
	})
}

// OnLogLine publishes one captured log line.
func (o *redisBatchRunObserver) OnLogLine(runID int64, ns string, entry LogEntry) {
	o.publish(BatchRunEvent{
		Kind: "log_line", RunID: runID, Namespace: ns,
		LogTs: entry.Ts, LogLevel: entry.Level, LogMsg: entry.Msg,
	})
}

// OnRunCompleted publishes the terminal success/failure event.
func (o *redisBatchRunObserver) OnRunCompleted(runID int64, ns string, success bool, errMsg string) {
	s := success
	o.publish(BatchRunEvent{
		Kind: "completed", RunID: runID, Namespace: ns, Success: &s, ErrorMessage: errMsg,
	})
}

// OnRunCancelled publishes the terminal operator-cancelled event.
func (o *redisBatchRunObserver) OnRunCancelled(runID int64, ns string) {
	o.publish(BatchRunEvent{Kind: "cancelled", RunID: runID, Namespace: ns})
}

func (o *redisBatchRunObserver) publish(ev BatchRunEvent) {
	raw, err := json.Marshal(ev)
	if err != nil {
		slog.Warn("batch run event marshal failed", "kind", ev.Kind, "error", err)
		return
	}
	// Detached context with a short deadline: observer callbacks fire from
	// the run goroutine, and a slow Redis must not stall the recompute.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := o.rdb.Publish(ctx, BatchRunEventChannel, raw).Err(); err != nil {
		slog.Warn("batch run event publish failed", "kind", ev.Kind, "error", err)
	}
}
