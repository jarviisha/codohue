package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
}

func (f *fakeRepo) ListNamespaces(_ context.Context) ([]NamespaceConfig, error) {
	return f.namespaces, f.nsListErr
}

func (f *fakeRepo) GetNamespace(_ context.Context, _ string) (*NamespaceConfig, error) {
	return f.namespace, f.nsGetErr
}

func (f *fakeRepo) GetBatchRunLogs(_ context.Context, _ string, _ int) ([]BatchRunLog, error) {
	return f.batchRuns, f.batchRunsErr
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

// ─── helpers ──────────────────────────────────────────────────────────────────

func newTestService(repo adminRepo, apiURL, apiKey string) *Service {
	return NewService(repo, apiURL, apiKey, nil, nil)
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

func TestUpsertNamespace_Proxy(t *testing.T) {
	apiKey := "test-key-proxy"
	var receivedAuth string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(NamespaceUpsertResponse{ //nolint:errcheck // test helper; encoding errors are not meaningful here
			Namespace: "new_ns",
			UpdatedAt: time.Now(),
		})
	}))
	defer fake.Close()

	svc := newTestService(&fakeRepo{}, fake.URL, apiKey)
	body := strings.NewReader(`{"lambda":0.05}`)
	_, _, err := svc.UpsertNamespace(context.Background(), "new_ns", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedAuth != "Bearer "+apiKey {
		t.Errorf("expected Authorization header %q, got %q", "Bearer "+apiKey, receivedAuth)
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
	resp, _, err := svc.DebugRecommend(context.Background(), &RecommendDebugRequest{
		Namespace: "ns1",
		SubjectID: "user-1",
		Limit:     10,
	})
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

func TestDebugRecommend_404Passthrough(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":"not_found","message":"namespace not found"}}`, http.StatusNotFound)
	}))
	defer fake.Close()

	svc := newTestService(&fakeRepo{}, fake.URL, "test-key")
	_, statusCode, err := svc.DebugRecommend(context.Background(), &RecommendDebugRequest{
		Namespace: "unknown",
		SubjectID: "user-1",
	})
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
