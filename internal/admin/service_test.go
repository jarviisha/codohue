package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ─── fake repo ────────────────────────────────────────────────────────────────

type fakeRepo struct {
	namespaces           []NamespaceConfig
	nsListErr            error
	namespace            *NamespaceConfig
	nsGetErr             error
	batchRuns            []BatchRunLog
	batchRunsErr         error
	lastBatchRuns        map[string]BatchRunLog
	lastBatchRunsErr     error
	recentEventCounts    map[string]int
	recentEventCountsErr error
	subjectStats         *SubjectStats
	subjectStatsErr      error
	events               []EventSummary
	eventsTotal          int
	eventsErr            error
	seededEvents         []demoEvent
	seededNamespace      string
	seedErr              error
	clearNamespace       string
	clearDeleted         int
	clearErr             error
}

func (f *fakeRepo) ListNamespaces(_ context.Context) ([]NamespaceConfig, error) {
	return f.namespaces, f.nsListErr
}

func (f *fakeRepo) GetNamespace(_ context.Context, _ string) (*NamespaceConfig, error) {
	return f.namespace, f.nsGetErr
}

func (f *fakeRepo) GetBatchRunLogs(_ context.Context, _, _ string, _, _ int) ([]BatchRunLog, int, BatchRunStats, error) {
	return f.batchRuns, len(f.batchRuns), BatchRunStats{Total: len(f.batchRuns)}, f.batchRunsErr
}

func (f *fakeRepo) GetLastBatchRunPerNamespace(_ context.Context) (map[string]BatchRunLog, error) {
	if f.lastBatchRuns == nil {
		return map[string]BatchRunLog{}, f.lastBatchRunsErr
	}
	return f.lastBatchRuns, f.lastBatchRunsErr
}

func (f *fakeRepo) GetRecentEventCounts(_ context.Context, _ int) (map[string]int, error) {
	if f.recentEventCounts == nil {
		return map[string]int{}, f.recentEventCountsErr
	}
	return f.recentEventCounts, f.recentEventCountsErr
}

func (f *fakeRepo) GetSubjectStats(_ context.Context, _, _ string, _ int) (*SubjectStats, error) {
	return f.subjectStats, f.subjectStatsErr
}

func (f *fakeRepo) GetRecentEvents(_ context.Context, _ string, _, _ int, _ string) ([]EventSummary, int, error) {
	return f.events, f.eventsTotal, f.eventsErr
}

func (f *fakeRepo) SeedDemoEvents(_ context.Context, namespace string, events []demoEvent, _ time.Time) (int, error) {
	f.seededNamespace = namespace
	f.seededEvents = events
	return len(events), f.seedErr
}

func (f *fakeRepo) ClearNamespaceData(_ context.Context, namespace string) (int, error) {
	f.clearNamespace = namespace
	return f.clearDeleted, f.clearErr
}

// ─── fake nsconfig upserter ──────────────────────────────────────────────────

type fakeNSConfig struct {
	gotNamespace string
	gotReq       *NamespaceUpsertRequest
	resp         *NamespaceUpsertResponse
	err          error
}

func (f *fakeNSConfig) Upsert(_ context.Context, namespace string, req *NamespaceUpsertRequest) (*NamespaceUpsertResponse, error) {
	f.gotNamespace = namespace
	f.gotReq = req
	if f.resp != nil {
		return f.resp, f.err
	}
	return &NamespaceUpsertResponse{Namespace: namespace, UpdatedAt: time.Now()}, f.err
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newTestService(repo adminRepo, apiURL, apiKey string) *Service {
	return NewService(repo, apiURL, apiKey, nil, nil, nil, &fakeNSConfig{})
}

func newTestServiceWithNS(repo adminRepo, apiURL, apiKey string, ns nsConfigUpserter) *Service {
	return NewService(repo, apiURL, apiKey, nil, nil, nil, ns)
}

// ─── tests ────────────────────────────────────────────────────────────────────

func TestListNamespaces_Service(t *testing.T) {
	repo := &fakeRepo{namespaces: []NamespaceConfig{{Namespace: "ns1"}, {Namespace: "ns2"}}}
	svc := newTestService(repo, "", "")
	list, err := svc.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2 namespaces, got %d", len(list))
	}
}

func TestGetHealth_Service_OK(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(HealthResponse{ //nolint:errcheck // test helper; encoding errors are not meaningful here
			Postgres: "ok", Redis: "ok", Qdrant: "ok", Status: "ok",
		})
	}))
	defer fake.Close()

	svc := newTestService(&fakeRepo{}, fake.URL, "test-key")
	resp, code, err := svc.GetHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("expected 200, got %d", code)
	}
	if resp.Status != "ok" {
		t.Errorf("expected status=ok, got %q", resp.Status)
	}
}

func TestGetHealth_Service_Degraded(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(HealthResponse{ //nolint:errcheck // test helper; encoding errors are not meaningful here
			Postgres: "ok", Redis: "degraded", Qdrant: "ok", Status: "degraded",
		})
	}))
	defer fake.Close()

	svc := newTestService(&fakeRepo{}, fake.URL, "test-key")
	resp, code, err := svc.GetHealth(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", code)
	}
	if resp.Status != "degraded" {
		t.Errorf("expected status=degraded, got %q", resp.Status)
	}
}

func TestUpsertNamespace_DelegatesToNSConfig(t *testing.T) {
	lambda := 0.05
	apiKey := "plaintext-once"
	fakeNS := &fakeNSConfig{
		resp: &NamespaceUpsertResponse{
			Namespace: "new_ns",
			UpdatedAt: time.Now(),
			APIKey:    &apiKey,
		},
	}
	svc := newTestServiceWithNS(&fakeRepo{}, "", "", fakeNS)

	req := &NamespaceUpsertRequest{Lambda: &lambda}
	resp, status, err := svc.UpsertNamespace(context.Background(), "new_ns", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fakeNS.gotNamespace != "new_ns" {
		t.Errorf("expected nsconfig to receive namespace %q, got %q", "new_ns", fakeNS.gotNamespace)
	}
	if fakeNS.gotReq == nil || fakeNS.gotReq.Lambda == nil || *fakeNS.gotReq.Lambda != lambda {
		t.Errorf("nsconfig did not receive the same upsert request payload")
	}
	if status != http.StatusCreated {
		t.Errorf("expected 201 (first-time create with api_key), got %d", status)
	}
	if resp == nil || resp.APIKey == nil || *resp.APIKey != apiKey {
		t.Errorf("expected APIKey %q to be propagated, got %+v", apiKey, resp)
	}
}

func TestUpsertNamespace_UpdateReturns200(t *testing.T) {
	fakeNS := &fakeNSConfig{
		resp: &NamespaceUpsertResponse{
			Namespace: "existing_ns",
			UpdatedAt: time.Now(),
			// APIKey nil → existing namespace, no plaintext key returned
		},
	}
	svc := newTestServiceWithNS(&fakeRepo{}, "", "", fakeNS)

	_, status, err := svc.UpsertNamespace(context.Background(), "existing_ns", &NamespaceUpsertRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("expected 200 (update), got %d", status)
	}
}

func TestUpsertNamespace_PropagatesError(t *testing.T) {
	fakeNS := &fakeNSConfig{err: errors.New("db down")}
	svc := newTestServiceWithNS(&fakeRepo{}, "", "", fakeNS)

	_, status, err := svc.UpsertNamespace(context.Background(), "any", &NamespaceUpsertRequest{})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if status != http.StatusInternalServerError {
		t.Errorf("expected 500 on upstream error, got %d", status)
	}
}

func TestDebugRecommend_Proxy(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck // test helper; encoding errors are not meaningful here
			"subject_id":   "user-1",
			"namespace":    "ns1",
			"items":        []map[string]any{{"object_id": "post_1", "score": 0.9, "rank": 1}},
			"source":       "cf",
			"limit":        10,
			"offset":       0,
			"total":        1,
			"generated_at": time.Now(),
		})
	}))
	defer fake.Close()

	svc := newTestService(&fakeRepo{}, fake.URL, "test-key")
	resp, _, err := svc.GetSubjectRecommendations(context.Background(), "ns1", "user-1", 10, 0, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].ObjectID != "post_1" {
		t.Errorf("expected object_id=post_1, got %q", resp.Items[0].ObjectID)
	}
}

func TestGetSubjectRecommendations_404Passthrough(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"not_found","message":"namespace not found"}}`, http.StatusNotFound)
	}))
	defer fake.Close()

	svc := newTestService(&fakeRepo{}, fake.URL, "test-key")
	_, statusCode, err := svc.GetSubjectRecommendations(context.Background(), "unknown", "user-1", 0, 0, false)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
	if statusCode != http.StatusNotFound {
		t.Errorf("expected statusCode=404, got %d", statusCode)
	}
}

func TestGetSubjectProfile_NoQdrant(t *testing.T) {
	numID := uint64(42)
	repo := &fakeRepo{
		namespace: &NamespaceConfig{Namespace: "ns1", SeenItemsDays: 30},
		subjectStats: &SubjectStats{
			InteractionCount: 7,
			SeenItems:        []string{"post_1", "post_2"},
			NumericID:        &numID,
		},
	}
	svc := newTestService(repo, "", "")
	profile, err := svc.GetSubjectProfile(context.Background(), "ns1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.InteractionCount != 7 {
		t.Errorf("expected 7 interactions, got %d", profile.InteractionCount)
	}
	if len(profile.SeenItems) != 2 {
		t.Errorf("expected 2 seen items, got %d", len(profile.SeenItems))
	}
	// qdrantClient is nil → NNZ should be -1
	if profile.SparseVectorNNZ != -1 {
		t.Errorf("expected sparse_vector_nnz=-1 when qdrant unavailable, got %d", profile.SparseVectorNNZ)
	}
	if profile.SeenItemsDays != 30 {
		t.Errorf("expected seen_items_days=30, got %d", profile.SeenItemsDays)
	}
}

func TestGetTrending_WithTTL(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck // test helper; encoding errors are not meaningful here
			"namespace":    "ns1",
			"items":        []map[string]any{{"object_id": "post_1", "score": 100.0}},
			"window_hours": 24,
			"limit":        50,
			"offset":       0,
			"total":        1,
			"generated_at": time.Now(),
		})
	}))
	defer fake.Close()

	// redisClient is nil — TTL will be -2 (key missing)
	svc := newTestService(&fakeRepo{}, fake.URL, "test-key")
	resp, err := svc.GetTrending(context.Background(), "ns1", 50, 0, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(resp.Items))
	}
	// With nil redisClient, TTL should be -2 (key missing sentinel)
	if resp.CacheTTLSec != -2 {
		t.Errorf("expected cache_ttl_sec=-2 when redis unavailable, got %d", resp.CacheTTLSec)
	}
}

// ─── TriggerBatch tests ───────────────────────────────────────────────────────

func TestTriggerBatch_NamespaceNotFound(t *testing.T) {
	repo := &fakeRepo{namespace: nil}
	svc := newTestService(repo, "", "")
	resp, err := svc.CreateBatchRun(context.Background(), "missing")
	if err != nil {
		t.Fatalf("expected nil error for missing ns, got %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for missing namespace, got %+v", resp)
	}
}

func TestTriggerBatch_ConcurrentLock(t *testing.T) {
	repo := &fakeRepo{namespace: &NamespaceConfig{Namespace: "ns1"}}
	svc := newTestService(repo, "", "")

	// Pre-load the key to simulate a running batch.
	svc.runningBatch.Store("ns1", true)
	defer svc.runningBatch.Delete("ns1")

	_, err := svc.CreateBatchRun(context.Background(), "ns1")
	if err == nil {
		t.Fatal("expected errBatchRunning, got nil")
	}
	if !errors.Is(err, errBatchRunning) {
		t.Errorf("expected errBatchRunning, got %v", err)
	}
}

// ─── GetRecentEvents tests ────────────────────────────────────────────────────

func TestGetRecentEvents_LimitClamp_Zero(t *testing.T) {
	repo := &fakeRepo{events: []EventSummary{}, eventsTotal: 0}
	svc := newTestService(repo, "", "")
	resp, err := svc.GetRecentEvents(context.Background(), "ns1", 0, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Limit != 50 {
		t.Errorf("expected limit clamped to 50, got %d", resp.Limit)
	}
}

func TestGetRecentEvents_LimitClamp_Over200(t *testing.T) {
	repo := &fakeRepo{events: []EventSummary{}, eventsTotal: 0}
	svc := newTestService(repo, "", "")
	resp, err := svc.GetRecentEvents(context.Background(), "ns1", 300, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Limit != 200 {
		t.Errorf("expected limit clamped to 200, got %d", resp.Limit)
	}
}

// ─── InjectEvent tests ────────────────────────────────────────────────────────

func TestInjectEvent_Proxy202(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/namespaces/ns1/events" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"ok":true}`)) //nolint:errcheck // test helper; encoding errors are not meaningful here
	}))
	defer fake.Close()

	svc := newTestService(&fakeRepo{}, fake.URL, "test-key")
	err := svc.InjectEvent(context.Background(), "ns1", InjectEventRequest{
		SubjectID: "user-1",
		ObjectID:  "item-1",
		Action:    "VIEW",
	})
	if err != nil {
		t.Fatalf("expected nil error on 202, got %v", err)
	}
}

func TestInjectEvent_ProxyError(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"code":"invalid","message":"action VIEW not configured"}}`)) //nolint:errcheck // test helper; encoding errors are not meaningful here
	}))
	defer fake.Close()

	svc := newTestService(&fakeRepo{}, fake.URL, "test-key")
	err := svc.InjectEvent(context.Background(), "ns1", InjectEventRequest{
		SubjectID: "user-1",
		ObjectID:  "item-1",
		Action:    "VIEW",
	})
	if err == nil {
		t.Fatal("expected error on non-202, got nil")
	}
}
