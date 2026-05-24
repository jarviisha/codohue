package admin

import "github.com/jarviisha/codohue/internal/core/batchrun"

// BatchRunSummaryFromLog converts the flat batch_run_logs row shape to the
// Phase 0 summary shape consumed by list endpoints. The flat schema does not
// distinguish "phase skipped" from "phase didn't run yet" — both map to nil.
// UI tells the two cases apart from the run's terminal status:
//
//   - completed_at IS NULL → null entries are "not yet run".
//   - completed_at IS NOT NULL → null entries are "skipped".
func BatchRunSummaryFromLog(b BatchRunLog) BatchRunSummary {
	return BatchRunSummary{
		ID:                b.ID,
		Namespace:         b.Namespace,
		Kind:              deriveBatchRunKind(b.TriggerSource),
		TriggerSource:     b.TriggerSource,
		StartedAt:         b.StartedAt,
		CompletedAt:       b.CompletedAt,
		DurationMs:        b.DurationMs,
		Success:           b.Success,
		CancelRequested:   b.CancelRequested,
		EntitiesProcessed: b.EntitiesProcessed,
		PhaseStatus: [3]*string{
			derivePhaseStatus(b.Phase1OK),
			derivePhaseStatus(b.Phase2OK),
			derivePhaseStatus(b.Phase3OK),
		},
		ErrorMessage: b.ErrorMessage,
	}
}

// BatchRunDetailFromLog converts the flat shape to the structured detail
// shape with phases[] array + log_lines + target_strategy.
func BatchRunDetailFromLog(b BatchRunLog) BatchRunDetail {
	var target *TargetStrategy
	if b.TargetStrategyID != nil && b.TargetStrategyVersion != nil {
		target = &TargetStrategy{ID: *b.TargetStrategyID, Version: *b.TargetStrategyVersion}
	}
	logs := b.LogLines
	if logs == nil {
		logs = []LogEntry{}
	}
	return BatchRunDetail{
		BatchRunSummary: BatchRunSummaryFromLog(b),
		Phases: []PhaseEntry{
			phaseEntry(1, "sparse", b.Phase1OK, b.Phase1DurMs, b.Phase1Subjects, b.Phase1Objects, nil, b.Phase1Error),
			phaseEntry(2, "dense", b.Phase2OK, b.Phase2DurMs, b.Phase2Subjects, nil, b.Phase2Items, b.Phase2Error),
			phaseEntry(3, "trending", b.Phase3OK, b.Phase3DurMs, nil, nil, b.Phase3Items, b.Phase3Error),
		},
		LogLines:       logs,
		TargetStrategy: target,
	}
}

func deriveBatchRunKind(triggerSource string) string {
	if triggerSource == string(batchrun.TriggerReembed) {
		return "reembed"
	}
	return "cf"
}

func derivePhaseStatus(ok *bool) *string {
	if ok == nil {
		return nil
	}
	var s string
	if *ok {
		s = "ok"
	} else {
		s = "fail"
	}
	return &s
}

func phaseEntry(n int, name string, ok *bool, durMs *int, subjects, objects, items *int, errMsg *string) PhaseEntry {
	dur := 0
	if durMs != nil {
		dur = *durMs
	}
	return PhaseEntry{
		N:          n,
		Name:       name,
		OK:         ok,
		Skipped:    nil,
		DurationMs: dur,
		Subjects:   subjects,
		Objects:    objects,
		Items:      items,
		Error:      errMsg,
	}
}
