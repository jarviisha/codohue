package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerGetBatchRunDetailNotFound(t *testing.T) {
	h := newTestHandler(&fakeSvc{batchRunDetail: nil})

	req := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs/42",
		map[string]string{"id": "42"}, "")
	rec := httptest.NewRecorder()
	h.GetBatchRunDetail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestHandlerGetBatchRunDetailOK(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		batchRunDetail: &BatchRunDetail{
			BatchRunSummary: BatchRunSummary{ID: 7, Namespace: "prod", Kind: "cf"},
		},
	})

	req := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs/7",
		map[string]string{"id": "7"}, "")
	rec := httptest.NewRecorder()
	h.GetBatchRunDetail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"id":7`) {
		t.Fatalf("body missing id: %s", rec.Body.String())
	}
}

func TestHandlerParseRunIDRejectsBadInput(t *testing.T) {
	h := newTestHandler(&fakeSvc{})

	for _, bad := range []string{"", "abc", "-1", "0"} {
		req := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs/x",
			map[string]string{"id": bad}, "")
		rec := httptest.NewRecorder()
		h.GetBatchRunDetail(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("id=%q: status=%d, want 400", bad, rec.Code)
		}
	}
}

func TestHandlerCancelBatchRunStatusMapping(t *testing.T) {
	cases := []struct {
		svcStatus  int
		wantStatus int
	}{
		{http.StatusOK, http.StatusOK},
		{http.StatusNotFound, http.StatusNotFound},
		{http.StatusConflict, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.wantStatus), func(t *testing.T) {
			svc := &fakeSvc{cancelStatus: tc.svcStatus}
			if tc.svcStatus == http.StatusOK {
				svc.cancelSummary = &BatchRunSummary{ID: 1}
			}
			h := newTestHandler(svc)
			req := newChiRequest(http.MethodPost, "/api/admin/v1/batch-runs/1/cancel",
				map[string]string{"id": "1"}, "")
			rec := httptest.NewRecorder()
			h.CancelBatchRun(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestHandlerRetryBatchRunStatusMapping(t *testing.T) {
	cases := []struct {
		svcStatus  int
		wantStatus int
	}{
		{http.StatusAccepted, http.StatusAccepted},
		{http.StatusNotFound, http.StatusNotFound},
		{http.StatusUnprocessableEntity, http.StatusUnprocessableEntity},
		{http.StatusConflict, http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.wantStatus), func(t *testing.T) {
			svc := &fakeSvc{retryStatus: tc.svcStatus}
			if tc.svcStatus == http.StatusAccepted {
				svc.retryCreate = &BatchRunCreateResponse{ID: 99}
			}
			h := newTestHandler(svc)
			req := newChiRequest(http.MethodPost, "/api/admin/v1/batch-runs/1/retry",
				map[string]string{"id": "1"}, "")
			rec := httptest.NewRecorder()
			h.RetryBatchRun(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status=%d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestHandlerRetryBatchRunSetsLocationOnAccepted(t *testing.T) {
	svc := &fakeSvc{
		retryStatus: http.StatusAccepted,
		retryCreate: &BatchRunCreateResponse{ID: 99, Namespace: "prod"},
	}
	h := newTestHandler(svc)
	req := newChiRequest(http.MethodPost, "/api/admin/v1/batch-runs/1/retry",
		map[string]string{"id": "1"}, "")
	rec := httptest.NewRecorder()
	h.RetryBatchRun(rec, req)

	if loc := rec.Header().Get("Location"); loc != "/api/admin/v1/batch-runs/99" {
		t.Fatalf("Location=%q", loc)
	}
}

func TestHandlerGetBatchRunStatsDefaults(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		statsBuckets: []BatchRunStatsBucket{{OK: 5, Failed: 1}},
	})
	req := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs/stats", nil, "")
	rec := httptest.NewRecorder()
	h.GetBatchRunStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`"window_seconds":86400`, `"bucket_seconds":3600`, `"series":`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}
}

func TestHandlerGetBatchRunStatsRejectsBadDuration(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	req := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs/stats?window=notaduration",
		nil, "")
	rec := httptest.NewRecorder()
	h.GetBatchRunStats(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", rec.Code)
	}
}

func TestHandlerGetOverviewReturnsAggregate(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		overviewResp: &OverviewResponse{Namespaces: []NamespaceOverview{{Namespace: "prod"}}},
	})
	req := newChiRequest(http.MethodGet, "/api/admin/v1/overview", nil, "")
	rec := httptest.NewRecorder()
	h.GetOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"namespace":"prod"`) {
		t.Fatalf("body missing ns: %s", rec.Body.String())
	}
}

func TestHandlerGetNamespaceDashboardNotFound(t *testing.T) {
	h := newTestHandler(&fakeSvc{nsDashboardResp: nil})
	req := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/missing/dashboard",
		map[string]string{"ns": "missing"}, "")
	rec := httptest.NewRecorder()
	h.GetNamespaceDashboard(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}
