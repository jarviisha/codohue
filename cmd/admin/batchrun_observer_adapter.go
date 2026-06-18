package main

import (
	"context"
	"strconv"

	"github.com/jarviisha/codohue/internal/admin/eventbus"
	"github.com/jarviisha/codohue/internal/compute"
	"github.com/jarviisha/codohue/internal/core/batchrun"
)

// batchRunObserverAdapter implements [compute.BatchRunObserver] by republishing
// each lifecycle callback as an event on the admin event bus. Defined at the
// wiring layer (cmd/admin) so internal/admin avoids importing internal/compute
// — that cross-domain import is forbidden by the architecture rule.
//
// Event vocabulary:
//
//	batch_run.started          { id, namespace, trigger_source }
//	batch_run.phase_started    { id, phase }
//	batch_run.phase_completed  { id, phase, ok, duration_ms, count1, count2, error }
//	batch_run.log_line         { ts, level, msg }
//	batch_run.completed        { id, namespace, success, error_message }
//	batch_run.cancelled        { id, namespace }
//
// Namespace and EntityID on the bus event let SSE handlers subscribe with a
// Filter targeting either a single run id, an entire namespace, or "every
// run in the cluster" (for the global /stream).
type batchRunObserverAdapter struct {
	bus *eventbus.Bus
}

func newBatchRunObserverAdapter(bus *eventbus.Bus) *batchRunObserverAdapter {
	return &batchRunObserverAdapter{bus: bus}
}

func (o *batchRunObserverAdapter) OnRunStarted(runID int64, ns string, src batchrun.TriggerSource) {
	o.publish("batch_run.started", runID, ns, map[string]any{
		"id":             runID,
		"namespace":      ns,
		"trigger_source": string(src),
	})
}

func (o *batchRunObserverAdapter) OnPhaseStarted(runID int64, ns string, phase int) {
	o.publish("batch_run.phase_started", runID, ns, map[string]any{
		"id":    runID,
		"phase": phase,
	})
}

func (o *batchRunObserverAdapter) OnPhaseCompleted(runID int64, ns string, phase int, result compute.PhaseResult) {
	o.publish("batch_run.phase_completed", runID, ns, map[string]any{
		"id":          runID,
		"phase":       phase,
		"ok":          result.OK,
		"duration_ms": result.DurationMs,
		"count1":      result.Count1,
		"count2":      result.Count2,
		"error":       result.Error,
	})
}

func (o *batchRunObserverAdapter) OnLogLine(runID int64, ns string, entry compute.LogEntry) {
	o.publish("batch_run.log_line", runID, ns, map[string]any{
		"ts":    entry.Ts,
		"level": entry.Level,
		"msg":   entry.Msg,
	})
}

func (o *batchRunObserverAdapter) OnRunCompleted(runID int64, ns string, success bool, errMsg string) {
	o.publish("batch_run.completed", runID, ns, map[string]any{
		"id":            runID,
		"namespace":     ns,
		"success":       success,
		"error_message": errMsg,
	})
}

func (o *batchRunObserverAdapter) OnRunCancelled(runID int64, ns string) {
	o.publish("batch_run.cancelled", runID, ns, map[string]any{
		"id":        runID,
		"namespace": ns,
	})
}

func (o *batchRunObserverAdapter) publish(kind string, runID int64, ns string, payload map[string]any) {
	o.bus.Publish(context.Background(), eventbus.Event{
		Kind:      kind,
		Namespace: ns,
		EntityID:  strconv.FormatInt(runID, 10),
		Payload:   payload,
	})
}
