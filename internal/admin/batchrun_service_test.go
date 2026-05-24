package admin

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestGetBatchRunDetailNotFoundReturnsNil(t *testing.T) {
	repo := &fakeRepo{batchRunByID: nil}
	svc := newTestService(repo, "http://api", "k")

	d, err := svc.GetBatchRunDetail(context.Background(), 42)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if d != nil {
		t.Fatalf("got detail, want nil for 404")
	}
}

func TestGetBatchRunDetailMapsRowToDetail(t *testing.T) {
	repo := &fakeRepo{
		batchRunByID: &BatchRunLog{
			ID:            7,
			Namespace:     "prod",
			TriggerSource: "cron",
			Success:       true,
			Phase1OK:      ptrB(true),
		},
	}
	svc := newTestService(repo, "http://api", "k")

	d, err := svc.GetBatchRunDetail(context.Background(), 7)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if d == nil || d.ID != 7 {
		t.Fatalf("detail=%+v", d)
	}
	if d.Kind != "cf" {
		t.Errorf("kind=%q, want cf", d.Kind)
	}
	if len(d.Phases) != 3 {
		t.Errorf("phases=%d, want 3", len(d.Phases))
	}
}

func TestCancelBatchRunStatusMapping(t *testing.T) {
	cases := []struct {
		name       string
		repoResult RequestCancelResult
		repoErr    error
		row        *BatchRunLog
		wantStatus int
	}{
		{"ok", RequestCancelOK, nil, &BatchRunLog{ID: 1}, http.StatusOK},
		{"not_found", RequestCancelNotFound, nil, nil, http.StatusNotFound},
		{"already_terminal", RequestCancelAlreadyTerminal, nil, nil, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{
				requestCancelResult: tc.repoResult,
				batchRunByID:        tc.row,
			}
			svc := newTestService(repo, "http://api", "k")
			summary, status, err := svc.CancelBatchRun(context.Background(), 1)
			if err != nil {
				t.Fatalf("err=%v", err)
			}
			if status != tc.wantStatus {
				t.Fatalf("status=%d, want %d", status, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusOK {
				if summary == nil || summary.ID != 1 {
					t.Errorf("summary=%+v", summary)
				}
			} else {
				if summary != nil {
					t.Errorf("summary=%+v, want nil on non-200", summary)
				}
			}
		})
	}
}

func TestCancelBatchRunRepoErrorPropagates(t *testing.T) {
	wantErr := errors.New("boom")
	repo := &fakeRepo{requestCancelErr: wantErr}
	svc := newTestService(repo, "http://api", "k")

	_, status, err := svc.CancelBatchRun(context.Background(), 1)
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if status != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", status)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("err=%v, want wraps %v", err, wantErr)
	}
}

func TestRetryBatchRunRejectRules(t *testing.T) {
	completed := time.Now()
	cases := []struct {
		name       string
		row        *BatchRunLog
		ns         *NamespaceConfig
		wantStatus int
	}{
		{"not_found", nil, nil, http.StatusNotFound},
		{
			"reembed_unsupported",
			&BatchRunLog{ID: 1, Namespace: "prod", TriggerSource: "admin_reembed", CompletedAt: &completed},
			&NamespaceConfig{Namespace: "prod"},
			http.StatusUnprocessableEntity,
		},
		{
			"namespace_deleted",
			&BatchRunLog{ID: 1, Namespace: "prod", TriggerSource: "cron", CompletedAt: &completed},
			nil,
			http.StatusUnprocessableEntity,
		},
		{
			"original_running",
			&BatchRunLog{ID: 1, Namespace: "prod", TriggerSource: "cron", CompletedAt: nil},
			&NamespaceConfig{Namespace: "prod"},
			http.StatusConflict,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{
				batchRunByID: tc.row,
				namespace:    tc.ns,
			}
			svc := newTestService(repo, "http://api", "k")

			_, status, _ := svc.RetryBatchRun(context.Background(), 1)
			if status != tc.wantStatus {
				t.Fatalf("status=%d, want %d", status, tc.wantStatus)
			}
		})
	}
}

func TestGetBatchRunStatsArgValidation(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo, "http://api", "k")

	cases := []struct {
		name           string
		window, bucket time.Duration
	}{
		{"zero_window", 0, time.Minute},
		{"zero_bucket", time.Hour, 0},
		{"bucket_gt_window", time.Minute, time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.GetBatchRunStats(context.Background(), tc.window, tc.bucket)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestGetBatchRunStatsPassThrough(t *testing.T) {
	want := []BatchRunStatsBucket{{OK: 5, Failed: 1}}
	repo := &fakeRepo{batchRunStats: want}
	svc := newTestService(repo, "http://api", "k")

	got, err := svc.GetBatchRunStats(context.Background(), 24*time.Hour, time.Hour)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(got) != 1 || got[0].OK != 5 {
		t.Fatalf("got=%+v", got)
	}
}
