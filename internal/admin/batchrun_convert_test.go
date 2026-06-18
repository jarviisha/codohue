package admin

import (
	"testing"
	"time"
)

func ptrB(v bool) *bool           { return &v }
func ptrI(v int) *int             { return &v }
func ptrS(v string) *string       { return &v }
func ptrT(v time.Time) *time.Time { return &v }

func TestBatchRunSummaryFromLogPhaseStatusOK(t *testing.T) {
	b := BatchRunLog{
		ID:        1,
		Namespace: "prod",
		Phase1OK:  ptrB(true),
		Phase2OK:  ptrB(false),
		Phase3OK:  nil, // didn't run / skipped
	}
	s := BatchRunSummaryFromLog(b)

	if s.PhaseStatus[0] == nil || *s.PhaseStatus[0] != "ok" {
		t.Errorf("phase1 = %v, want ok", s.PhaseStatus[0])
	}
	if s.PhaseStatus[1] == nil || *s.PhaseStatus[1] != "fail" {
		t.Errorf("phase2 = %v, want fail", s.PhaseStatus[1])
	}
	if s.PhaseStatus[2] != nil {
		t.Errorf("phase3 = %v, want nil", s.PhaseStatus[2])
	}
}

func TestBatchRunSummaryFromLogKindDerivation(t *testing.T) {
	cases := map[string]string{
		"cron":          "cf",
		"manual":        "cf",
		"admin_reembed": "reembed",
	}
	for trigger, wantKind := range cases {
		t.Run(trigger, func(t *testing.T) {
			b := BatchRunLog{TriggerSource: trigger}
			if got := BatchRunSummaryFromLog(b).Kind; got != wantKind {
				t.Errorf("kind = %q, want %q", got, wantKind)
			}
		})
	}
}

func TestBatchRunDetailFromLogPopulatesAllThreePhases(t *testing.T) {
	now := time.Now()
	b := BatchRunLog{
		ID:             42,
		Namespace:      "prod",
		StartedAt:      now,
		CompletedAt:    ptrT(now.Add(time.Minute)),
		DurationMs:     ptrI(60000),
		Success:        true,
		TriggerSource:  "cron",
		LogLines:       []LogEntry{{Ts: "t", Level: "info", Msg: "ok"}},
		Phase1OK:       ptrB(true),
		Phase1DurMs:    ptrI(3000),
		Phase1Subjects: ptrI(5),
		Phase1Objects:  ptrI(10),
		Phase2OK:       ptrB(true),
		Phase2DurMs:    ptrI(6000),
		Phase2Items:    ptrI(10),
		Phase2Subjects: ptrI(5),
		Phase3OK:       ptrB(true),
		Phase3DurMs:    ptrI(900),
		Phase3Items:    ptrI(7),
	}
	d := BatchRunDetailFromLog(b)
	if len(d.Phases) != 3 {
		t.Fatalf("len(phases) = %d, want 3", len(d.Phases))
	}
	if d.Phases[0].Name != "sparse" || d.Phases[1].Name != "dense" || d.Phases[2].Name != "trending" {
		t.Errorf("phase names: %q %q %q", d.Phases[0].Name, d.Phases[1].Name, d.Phases[2].Name)
	}
	if d.Phases[0].Subjects == nil || *d.Phases[0].Subjects != 5 {
		t.Errorf("phase1.subjects = %v", d.Phases[0].Subjects)
	}
	if d.Phases[0].Items != nil {
		t.Errorf("phase1.items should be nil (sparse doesn't carry items): %v", d.Phases[0].Items)
	}
	if d.Phases[2].Items == nil || *d.Phases[2].Items != 7 {
		t.Errorf("phase3.items = %v", d.Phases[2].Items)
	}
	if d.Phases[2].Subjects != nil {
		t.Errorf("phase3.subjects should be nil: %v", d.Phases[2].Subjects)
	}
}

func TestBatchRunDetailFromLogTargetStrategyPresentForReembed(t *testing.T) {
	b := BatchRunLog{
		TriggerSource:         "admin_reembed",
		TargetStrategyID:      ptrS("bge"),
		TargetStrategyVersion: ptrS("v2"),
	}
	d := BatchRunDetailFromLog(b)
	if d.TargetStrategy == nil {
		t.Fatal("TargetStrategy nil")
	}
	if d.TargetStrategy.ID != "bge" || d.TargetStrategy.Version != "v2" {
		t.Errorf("TargetStrategy = %+v", d.TargetStrategy)
	}
	if d.Kind != "reembed" {
		t.Errorf("kind = %q, want reembed", d.Kind)
	}
}

func TestBatchRunDetailFromLogTargetStrategyNilForCron(t *testing.T) {
	b := BatchRunLog{TriggerSource: "cron"}
	d := BatchRunDetailFromLog(b)
	if d.TargetStrategy != nil {
		t.Errorf("TargetStrategy = %+v, want nil", d.TargetStrategy)
	}
}

func TestBatchRunDetailFromLogLogLinesNeverNil(t *testing.T) {
	b := BatchRunLog{LogLines: nil}
	d := BatchRunDetailFromLog(b)
	if d.LogLines == nil {
		t.Fatal("LogLines should be non-nil empty slice for stable JSON")
	}
}

func TestBatchRunSummaryCancelRequestedPropagates(t *testing.T) {
	b := BatchRunLog{CancelRequested: true}
	if !BatchRunSummaryFromLog(b).CancelRequested {
		t.Fatal("CancelRequested should propagate")
	}
}
