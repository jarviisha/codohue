package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetCatalogBacklogHistory_WindowAndBucket(t *testing.T) {
	cases := []struct {
		query      string
		wantStatus int
		wantWindow time.Duration
		wantBucket time.Duration
	}{
		{"", http.StatusOK, time.Hour, 0},
		{"?window=24h&bucket=5m", http.StatusOK, 24 * time.Hour, 5 * time.Minute},
		{"?window=168h&bucket=30m", http.StatusOK, 168 * time.Hour, 30 * time.Minute},
		{"?window=7d", http.StatusBadRequest, 0, 0},
		{"?bucket=-5m", http.StatusBadRequest, 0, 0},
		{"?bucket=abc", http.StatusBadRequest, 0, 0},
		{"?window=0s", http.StatusBadRequest, 0, 0},
		{"?window=-1h", http.StatusBadRequest, 0, 0},
		// Sub-second buckets would truncate to 0 and silently mean "raw".
		{"?bucket=500ms", http.StatusBadRequest, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			svc := &fakeSvc{backlogHistoryResp: &CatalogBacklogHistoryResponse{Namespace: "ns1"}}
			rec := httptest.NewRecorder()
			r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns1/catalog/backlog-history"+tc.query,
				map[string]string{"ns": "ns1"}, "")
			newTestHandler(svc).GetCatalogBacklogHistory(rec, r)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if svc.backlogHistoryWindow != tc.wantWindow || svc.backlogHistoryBucket != tc.wantBucket {
				t.Errorf("svc got window=%v bucket=%v, want %v / %v",
					svc.backlogHistoryWindow, svc.backlogHistoryBucket, tc.wantWindow, tc.wantBucket)
			}
		})
	}
}

func TestGetCatalogFailuresSummary_RejectsNonPositiveWindow(t *testing.T) {
	for _, q := range []string{"?window=0s", "?window=-24h"} {
		rec := httptest.NewRecorder()
		r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns1/catalog/failures-summary"+q,
			map[string]string{"ns": "ns1"}, "")
		newTestHandler(&fakeSvc{}).GetCatalogFailuresSummary(rec, r)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", q, rec.Code)
		}
	}
}
