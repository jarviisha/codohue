package admin

import (
	"encoding/json"
	"testing"
	"time"
)

// JSON roundtrip tests for the Phase 0 admin schema. Goal: detect json tag
// drift early (per BUILD_PLAN §1.2 "every struct has Go test JSON roundtrip")
// so the TypeScript client generated from these structs cannot get out of
// sync silently.

func boolp(b bool) *bool       { return &b }
func intp(n int) *int          { return &n }
func strp(s string) *string    { return &s }
func timep(t time.Time) *time.Time { return &t }

func roundtripJSON[T any](t *testing.T, in T) T {
	t.Helper()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v\ndata: %s", err, data)
	}
	redata, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(data) != string(redata) {
		t.Errorf("roundtrip mismatch:\nin : %s\nout: %s", data, redata)
	}
	return out
}

var refTime = time.Date(2026, 5, 24, 10, 30, 0, 0, time.UTC)

func TestJSONRoundtripPhaseEntry(t *testing.T) {
	cases := map[string]PhaseEntry{
		"ok_sparse": {
			N: 1, Name: "sparse", OK: boolp(true),
			DurationMs: 3120, Subjects: intp(5120), Objects: intp(28700),
		},
		"skipped_dense": {
			N: 2, Name: "dense", OK: nil, Skipped: strp("dense_strategy=byoe"),
			Items: intp(0), Subjects: intp(0),
		},
		"failed_trending": {
			N: 3, Name: "trending", OK: boolp(false),
			Items: intp(0), Error: strp("redis unavailable"),
		},
		"ok_trending_no_subjects_objects": {
			N: 3, Name: "trending", OK: boolp(true),
			DurationMs: 980, Items: intp(4500),
		},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			roundtripJSON(t, in)
		})
	}
}

func TestJSONRoundtripBatchRunSummary(t *testing.T) {
	completedAt := refTime.Add(15 * time.Second)
	dur := 15000
	in := BatchRunSummary{
		ID:                1234,
		Namespace:         "prod",
		Kind:              "cf",
		TriggerSource:     "cron",
		StartedAt:         refTime,
		CompletedAt:       &completedAt,
		DurationMs:        &dur,
		Success:           true,
		CancelRequested:   false,
		EntitiesProcessed: 5120,
		PhaseStatus:       [3]*string{strp("ok"), strp("skipped"), strp("ok")},
		ErrorMessage:      nil,
	}
	roundtripJSON(t, in)
}

func TestJSONRoundtripBatchRunDetail(t *testing.T) {
	completedAt := refTime.Add(15 * time.Second)
	dur := 15000
	in := BatchRunDetail{
		BatchRunSummary: BatchRunSummary{
			ID:                1234,
			Namespace:         "prod",
			Kind:              "reembed",
			TriggerSource:     "admin_reembed",
			StartedAt:         refTime,
			CompletedAt:       &completedAt,
			DurationMs:        &dur,
			Success:           true,
			EntitiesProcessed: 28700,
			PhaseStatus:       [3]*string{strp("ok"), strp("ok"), strp("ok")},
		},
		Phases: []PhaseEntry{
			{N: 1, Name: "sparse", OK: boolp(true), DurationMs: 3120, Subjects: intp(5120), Objects: intp(28700)},
			{N: 2, Name: "dense", OK: boolp(true), DurationMs: 6220, Items: intp(28700), Subjects: intp(5120)},
			{N: 3, Name: "trending", OK: boolp(true), DurationMs: 980, Items: intp(4500)},
		},
		LogLines: []LogEntry{
			{Ts: refTime.Format(time.RFC3339Nano), Level: "info", Msg: "phase 1 started"},
		},
		TargetStrategy: &TargetStrategy{ID: "bge", Version: "v2"},
	}
	roundtripJSON(t, in)
}

func TestJSONRoundtripOverviewResponse(t *testing.T) {
	in := OverviewResponse{
		GeneratedAt: refTime,
		Health:      HealthResponse{Postgres: "ok", Redis: "ok", Qdrant: "ok", Status: "ok"},
		CronHeartbeat: CronHeartbeat{
			LastRunAt:  timep(refTime.Add(-2 * time.Minute)),
			LagSeconds: 42,
			OK:         true,
		},
		EmbedderHeartbeat: EmbedderHeartbeat{
			LastSeenAt: timep(refTime.Add(-10 * time.Second)),
			OK:         true,
		},
		Alerts: []Alert{
			{Level: "warn", Namespace: "prod", Kind: "dead_letter_growth", Message: "5 new dead letters in 5m"},
		},
		Namespaces: []NamespaceOverview{
			{
				Namespace: "prod",
				Status:    NSStatusActive,
				LastRun: &NamespaceOverviewRun{
					ID:          1234,
					StartedAt:   refTime.Add(-5 * time.Minute),
					Success:     true,
					PhaseStatus: [3]*string{strp("ok"), strp("ok"), strp("ok")},
				},
				Events24h:       192034,
				EventsPerMinNow: 142.7,
				Catalog:         NamespaceOverviewCatalog{Enabled: true, Pending: 12, DeadLetter: 0},
				Qdrant:          NamespaceOverviewQdrant{Subjects: 5120, Objects: 28700},
			},
		},
	}
	roundtripJSON(t, in)
}

func TestJSONRoundtripNamespaceDashboardResponse(t *testing.T) {
	dur := 8000
	in := NamespaceDashboardResponse{
		Namespace:   "prod",
		GeneratedAt: refTime,
		Config: NamespaceConfig{
			Namespace:     "prod",
			ActionWeights: map[string]float64{"view": 0.1, "like": 1.0},
			Lambda:        0.05,
			Gamma:         0.5,
			Alpha:         0.7,
			MaxResults:    100,
			SeenItemsDays: 7,
			DenseStrategy: "byoe",
			EmbeddingDim:  768,
			DenseDistance: "cosine",
			HasAPIKey:     true,
			UpdatedAt:     refTime,
		},
		LastRuns: []BatchRunSummary{
			{ID: 100, Namespace: "prod", Kind: "cf", TriggerSource: "cron",
				StartedAt: refTime, DurationMs: &dur, Success: true,
				PhaseStatus: [3]*string{strp("ok"), strp("ok"), strp("ok")}},
		},
		Catalog:         CatalogBacklog{Pending: 12, InFlight: 1, Embedded: 28699, Failed: 0, DeadLetter: 0, StreamLen: 13},
		Events24h:       192034,
		EventsPerMinNow: 142.7,
		Qdrant: QdrantInspectResponse{
			Subjects:      QdrantCollection{Exists: true, PointsCount: 5120},
			Objects:       QdrantCollection{Exists: true, PointsCount: 28700},
			SubjectsDense: QdrantCollection{Exists: true, PointsCount: 5120},
			ObjectsDense:  QdrantCollection{Exists: true, PointsCount: 28700},
		},
		TrendingTTLSec: 300,
	}
	roundtripJSON(t, in)
}

func TestJSONRoundtripAlert(t *testing.T) {
	cases := []Alert{
		{Level: "warn", Namespace: "prod", Kind: "dead_letter_growth", Message: "5 in 5m"},
		{Level: "error", Kind: "health_degraded", Message: "qdrant unreachable"}, // no namespace
	}
	for i, in := range cases {
		t.Run([]string{"warn", "error-no-ns"}[i], func(t *testing.T) {
			roundtripJSON(t, in)
		})
	}
}
