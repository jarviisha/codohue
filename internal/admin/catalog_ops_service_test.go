package admin

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// withCatalogPlumbing wires the most common test rig: fakeRepo + fake stream
// publisher + fake strategy picker + fake qdrant deleter, all returned for
// assertions. Tests pass enabled=false to exercise 404 paths.
func withCatalogPlumbing(t *testing.T, repo *fakeRepo, picker *fakeStrategyPicker) (*Service, *fakeStreamPublisher, *fakeQdrantDeleter) {
	t.Helper()
	svc := newTestService(repo, "", "")
	pub := &fakeStreamPublisher{}
	del := &fakeQdrantDeleter{}
	svc.SetStreamPublisher(pub)
	svc.SetQdrantPointDeleter(del)
	if picker != nil {
		svc.SetCatalogStrategyPicker(picker)
	}
	svc.SetNowFn(func() time.Time { return time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC) })
	return svc, pub, del
}

// ─── GetCatalogConfig liveness signals ────────────────────────────────────────

func TestGetCatalogConfig_LivenessSignals(t *testing.T) {
	completedAt := time.Date(2026, 5, 10, 11, 45, 0, 0, time.UTC)
	startedAt := time.Date(2026, 5, 10, 11, 30, 0, 0, time.UTC)
	embedAt := time.Date(2026, 5, 10, 11, 59, 0, 0, time.UTC)
	target := "reembed:strat-x/v2"
	dur := 900_000 // 15 minutes

	cases := []struct {
		name           string
		run            *BatchRunLog
		wantStatus     string
		wantErrMessage string
	}{
		{
			name: "running",
			run: &BatchRunLog{
				ID:        7,
				StartedAt: startedAt,
				// CompletedAt nil → running
				ErrorMessage:      &target,
				SubjectsProcessed: 0,
			},
			wantStatus: "running",
		},
		{
			name: "success",
			run: &BatchRunLog{
				ID:                8,
				StartedAt:         startedAt,
				CompletedAt:       &completedAt,
				DurationMs:        &dur,
				Success:           true,
				SubjectsProcessed: 42,
			},
			wantStatus: "success",
		},
		{
			name: "failed",
			run: &BatchRunLog{
				ID:                9,
				StartedAt:         startedAt,
				CompletedAt:       &completedAt,
				Success:           false,
				ErrorMessage:      ptrStr("strategy not registered"),
				SubjectsProcessed: 10,
			},
			wantStatus:     "failed",
			wantErrMessage: "strategy not registered",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{
				lastEmbeddedAt: &embedAt,
				latestReembed:  tc.run,
			}
			svc := newTestService(repo, "", "")
			svc.SetCatalogConfigurator(&fakeCatalogConfig{getResp: &NamespaceCatalogConfig{Namespace: "ns"}})

			resp, err := svc.GetCatalogConfig(context.Background(), "ns")
			if err != nil {
				t.Fatalf("GetCatalogConfig returned error: %v", err)
			}
			if resp == nil {
				t.Fatal("expected non-nil response")
			}
			if resp.LastEmbeddedAt == nil || !resp.LastEmbeddedAt.Equal(embedAt) {
				t.Errorf("LastEmbeddedAt = %v, want %v", resp.LastEmbeddedAt, embedAt)
			}
			if resp.LastReEmbed == nil {
				t.Fatal("LastReEmbed nil, want populated")
			}
			if resp.LastReEmbed.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", resp.LastReEmbed.Status, tc.wantStatus)
			}
			if resp.LastReEmbed.ErrorMessage != tc.wantErrMessage {
				t.Errorf("ErrorMessage = %q, want %q", resp.LastReEmbed.ErrorMessage, tc.wantErrMessage)
			}
		})
	}
}

func TestGetCatalogConfig_LivenessSignals_RepoErrorsNonFatal(t *testing.T) {
	// Repo failures for liveness signals must not break the response — the
	// status panel still renders with whatever it had.
	repo := &fakeRepo{
		lastEmbeddedAtErr: errors.New("db transient"),
		latestReembedErr:  errors.New("db transient"),
	}
	svc := newTestService(repo, "", "")
	svc.SetCatalogConfigurator(&fakeCatalogConfig{getResp: &NamespaceCatalogConfig{Namespace: "ns"}})

	resp, err := svc.GetCatalogConfig(context.Background(), "ns")
	if err != nil {
		t.Fatalf("GetCatalogConfig returned error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.LastEmbeddedAt != nil {
		t.Errorf("LastEmbeddedAt = %v, want nil on repo error", resp.LastEmbeddedAt)
	}
	if resp.LastReEmbed != nil {
		t.Errorf("LastReEmbed = %+v, want nil on repo error", resp.LastReEmbed)
	}
}

func ptrStr(s string) *string { return &s }

// ─── TriggerReEmbed ───────────────────────────────────────────────────────────

func TestTriggerReEmbed_Service_HappyPath(t *testing.T) {
	repo := &fakeRepo{
		insertReembedID:   42,
		staleResetTargets: []CatalogReembedTarget{{ID: 1, ObjectID: "o1"}, {ID: 2, ObjectID: "o2"}},
	}
	picker := &fakeStrategyPicker{id: "internal-hashing-ngrams", version: "v1", enabled: true}
	svc, pub, _ := withCatalogPlumbing(t, repo, picker)

	resp, err := svc.TriggerReEmbed(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.BatchRunID != 42 {
		t.Errorf("expected batch_run_id=42, got %d", resp.BatchRunID)
	}
	if resp.StaleItems != 2 {
		t.Errorf("expected stale_items=2, got %d", resp.StaleItems)
	}
	if len(pub.calls) != 2 {
		t.Errorf("expected 2 XADD calls, got %d", len(pub.calls))
	}
	if pub.calls[0].Stream != "catalog:embed:ns" {
		t.Errorf("expected stream=catalog:embed:ns, got %q", pub.calls[0].Stream)
	}
	values := pub.calls[0].Values.(map[string]any)
	if values["strategy_version"] != "v1" {
		t.Errorf("expected strategy_version=v1 in payload, got %v", values["strategy_version"])
	}
	if repo.insertedReembed.namespace != "ns" || repo.insertedReembed.strategyVersion != "v1" {
		t.Errorf("repo insert not called with right args: %+v", repo.insertedReembed)
	}
}

func TestTriggerReEmbed_Service_NotEnabled_ReturnsNil(t *testing.T) {
	repo := &fakeRepo{}
	picker := &fakeStrategyPicker{enabled: false}
	svc, _, _ := withCatalogPlumbing(t, repo, picker)

	resp, err := svc.TriggerReEmbed(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response when catalog disabled, got %+v", resp)
	}
	if repo.insertedReembed.namespace != "" {
		t.Errorf("expected no DB insert when disabled, got %+v", repo.insertedReembed)
	}
}

func TestTriggerReEmbed_Service_RunningInDB_409(t *testing.T) {
	repo := &fakeRepo{runningReembed: &BatchRunLog{ID: 99, Namespace: "ns"}}
	picker := &fakeStrategyPicker{id: "x", version: "v1", enabled: true}
	svc, _, _ := withCatalogPlumbing(t, repo, picker)

	_, err := svc.TriggerReEmbed(context.Background(), "ns")
	if !errors.Is(err, ErrReembedAlreadyRunning) {
		t.Fatalf("expected ErrReembedAlreadyRunning, got %v", err)
	}
}

func TestTriggerReEmbed_Service_PickerUnavailable_503(t *testing.T) {
	svc := newTestService(&fakeRepo{}, "", "")
	// no SetCatalogStrategyPicker call

	_, err := svc.TriggerReEmbed(context.Background(), "ns")
	if !errors.Is(err, ErrCatalogStrategyPickerUnavailable) {
		t.Fatalf("expected ErrCatalogStrategyPickerUnavailable, got %v", err)
	}
}

func TestTriggerReEmbed_Service_PublishFailureIsBestEffort(t *testing.T) {
	repo := &fakeRepo{
		insertReembedID:   1,
		staleResetTargets: []CatalogReembedTarget{{ID: 1, ObjectID: "o1"}},
	}
	picker := &fakeStrategyPicker{id: "x", version: "v1", enabled: true}
	svc, pub, _ := withCatalogPlumbing(t, repo, picker)
	pub.err = errors.New("redis down")

	resp, err := svc.TriggerReEmbed(context.Background(), "ns")
	if err != nil {
		t.Fatalf("publish failure must NOT bubble up: %v", err)
	}
	if resp.StaleItems != 1 {
		t.Errorf("DB reset should still report 1 stale item, got %d", resp.StaleItems)
	}
}

// ─── ListCatalogItems ─────────────────────────────────────────────────────────

func TestListCatalogItems_Service_DefaultsAndLimitClamp(t *testing.T) {
	repo := &fakeRepo{
		listItemsResp:  []CatalogItemSummary{{ID: 1}},
		listItemsTotal: 1,
	}
	svc := newTestService(repo, "", "")

	resp, err := svc.ListCatalogItems(context.Background(), "ns", "", 9999, -1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Limit != 500 {
		t.Errorf("expected limit clamp to 500, got %d", resp.Limit)
	}
	if repo.listItemsCalled.limit != 500 {
		t.Errorf("expected repo limit=500, got %d", repo.listItemsCalled.limit)
	}
}

func TestListCatalogItems_Service_NilItemsBecomesEmpty(t *testing.T) {
	repo := &fakeRepo{listItemsResp: nil, listItemsTotal: 0}
	svc := newTestService(repo, "", "")

	resp, err := svc.ListCatalogItems(context.Background(), "ns", "all", 50, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Items == nil {
		t.Errorf("expected non-nil empty slice, got nil")
	}
}

// ─── RedriveCatalogItem ───────────────────────────────────────────────────────

func TestRedriveCatalogItem_Service_HappyPath(t *testing.T) {
	repo := &fakeRepo{
		redriveItem: &CatalogItemDetail{
			CatalogItemSummary: CatalogItemSummary{ID: 5, ObjectID: "o5", State: "pending"},
		},
	}
	picker := &fakeStrategyPicker{id: "x", version: "v1", enabled: true}
	svc, pub, _ := withCatalogPlumbing(t, repo, picker)

	resp, err := svc.RedriveCatalogItem(context.Background(), "ns", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != 5 || resp.ObjectID != "o5" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(pub.calls) != 1 {
		t.Fatalf("expected exactly 1 XADD call, got %d", len(pub.calls))
	}
	values := pub.calls[0].Values.(map[string]any)
	if values["catalog_item_id"] != int64(5) {
		t.Errorf("expected catalog_item_id=5, got %v", values["catalog_item_id"])
	}
}

func TestRedriveCatalogItem_Service_NotFoundReturnsNil(t *testing.T) {
	repo := &fakeRepo{redriveItem: nil}
	picker := &fakeStrategyPicker{id: "x", version: "v1", enabled: true}
	svc, pub, _ := withCatalogPlumbing(t, repo, picker)

	resp, err := svc.RedriveCatalogItem(context.Background(), "ns", 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response for not-found, got %+v", resp)
	}
	if len(pub.calls) != 0 {
		t.Errorf("expected no XADD when row missing, got %d", len(pub.calls))
	}
}

func TestRedriveCatalogItem_Service_CatalogDisabled(t *testing.T) {
	repo := &fakeRepo{}
	picker := &fakeStrategyPicker{enabled: false}
	svc, _, _ := withCatalogPlumbing(t, repo, picker)

	resp, err := svc.RedriveCatalogItem(context.Background(), "ns", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response when disabled, got %+v", resp)
	}
}

// ─── BulkRedriveDeadletter ────────────────────────────────────────────────────

func TestBulkRedriveDeadletter_Service_HappyPath(t *testing.T) {
	repo := &fakeRepo{
		bulkRedriveTargets: []CatalogReembedTarget{
			{ID: 1, ObjectID: "o1"},
			{ID: 2, ObjectID: "o2"},
			{ID: 3, ObjectID: "o3"},
		},
	}
	picker := &fakeStrategyPicker{id: "x", version: "v1", enabled: true}
	svc, pub, _ := withCatalogPlumbing(t, repo, picker)

	resp, err := svc.BulkRedriveDeadletter(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Redriven != 3 {
		t.Errorf("expected redriven=3, got %d", resp.Redriven)
	}
	if len(pub.calls) != 3 {
		t.Errorf("expected 3 XADD calls, got %d", len(pub.calls))
	}
}

func TestBulkRedriveDeadletter_Service_EmptyOK(t *testing.T) {
	repo := &fakeRepo{bulkRedriveTargets: nil}
	picker := &fakeStrategyPicker{id: "x", version: "v1", enabled: true}
	svc, pub, _ := withCatalogPlumbing(t, repo, picker)

	resp, err := svc.BulkRedriveDeadletter(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Redriven != 0 {
		t.Errorf("expected redriven=0, got %d", resp.Redriven)
	}
	if len(pub.calls) != 0 {
		t.Errorf("expected no XADD on empty list, got %d", len(pub.calls))
	}
}

// ─── DeleteCatalogItem ────────────────────────────────────────────────────────

func TestDeleteCatalogItem_Service_HappyPath(t *testing.T) {
	repo := &fakeRepo{
		deleteCatalogItemFound:  true,
		deleteCatalogItemObject: "o7",
		numericObjectID:         123,
		numericObjectFound:      true,
	}
	svc, _, del := withCatalogPlumbing(t, repo, nil)

	if err := svc.DeleteCatalogItem(context.Background(), "ns", 7); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.deleteCatalogItemCalled != 7 {
		t.Errorf("expected repo delete called with id=7, got %d", repo.deleteCatalogItemCalled)
	}
	if len(del.calls) != 1 {
		t.Fatalf("expected 1 qdrant delete call, got %d", len(del.calls))
	}
	if del.calls[0].collection != "ns_objects_dense" || del.calls[0].id != 123 {
		t.Errorf("unexpected qdrant call: %+v", del.calls[0])
	}
}

func TestDeleteCatalogItem_Service_PostgresMissIsIdempotent(t *testing.T) {
	repo := &fakeRepo{deleteCatalogItemFound: false}
	svc, _, del := withCatalogPlumbing(t, repo, nil)

	if err := svc.DeleteCatalogItem(context.Background(), "ns", 999); err != nil {
		t.Fatalf("expected idempotent success, got error: %v", err)
	}
	if len(del.calls) != 0 {
		t.Errorf("expected no qdrant delete when row was already gone, got %d", len(del.calls))
	}
}

func TestDeleteCatalogItem_Service_PostgresErrorBubbles(t *testing.T) {
	repo := &fakeRepo{deleteCatalogItemErr: fmt.Errorf("db down")}
	svc, _, _ := withCatalogPlumbing(t, repo, nil)

	if err := svc.DeleteCatalogItem(context.Background(), "ns", 1); err == nil {
		t.Fatal("expected error from postgres delete failure")
	}
}

func TestDeleteCatalogItem_Service_QdrantMissIsBestEffort(t *testing.T) {
	repo := &fakeRepo{
		deleteCatalogItemFound:  true,
		deleteCatalogItemObject: "o7",
		numericObjectID:         99,
		numericObjectFound:      true,
	}
	svc, _, del := withCatalogPlumbing(t, repo, nil)
	del.err = errors.New("qdrant down")

	if err := svc.DeleteCatalogItem(context.Background(), "ns", 7); err != nil {
		t.Fatalf("qdrant failure must NOT bubble up: %v", err)
	}
}
