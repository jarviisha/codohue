//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// Bucketing groups samples by the floor of their epoch second. A sample in
// the last half-second of a bucket must stay in that bucket (a rounding cast
// would push it into the next one), and each series reports its MAX so short
// backlog peaks survive downsampling.
func TestAdmin_CatalogBacklogHistoryBucketsByFloorAndMax(t *testing.T) {
	ns, _ := createIsolatedNamespace(t, "backlog", defaultNamespaceConfig())
	cookie := adminLogin(t)

	start := time.Now().Add(-10 * time.Minute).Truncate(time.Minute)
	for _, s := range []struct {
		offset  time.Duration
		pending int
	}{
		{200 * time.Millisecond, 1},
		{59700 * time.Millisecond, 9},
		{60100 * time.Millisecond, 2},
	} {
		if _, err := testDB.Exec(context.Background(), `
			INSERT INTO catalog_backlog_samples
			  (namespace, sampled_at, pending, in_flight, failed, dead_letter, stream_len)
			VALUES ($1, $2, $3, 0, 0, 0, 0)`, ns, start.Add(s.offset), s.pending); err != nil {
			t.Fatalf("seed sample: %v", err)
		}
	}

	var got struct {
		BucketSeconds int `json:"bucket_seconds"`
		Samples       []struct {
			SampledAt time.Time `json:"sampled_at"`
			Pending   int       `json:"pending"`
		} `json:"samples"`
	}
	resp := adminRequest(t, http.MethodGet,
		"/api/admin/v1/namespaces/"+ns+"/catalog/backlog-history?window=1h&bucket=1m", cookie, nil)
	decodeJSON(t, resp, &got)

	if got.BucketSeconds != 60 || len(got.Samples) != 2 {
		t.Fatalf("want 2 one-minute buckets, got bucket=%d samples=%+v", got.BucketSeconds, got.Samples)
	}
	if !got.Samples[0].SampledAt.Equal(start) || got.Samples[0].Pending != 9 {
		t.Errorf("first bucket = %+v, want %s with MAX pending 9", got.Samples[0], start)
	}
	if !got.Samples[1].SampledAt.Equal(start.Add(time.Minute)) || got.Samples[1].Pending != 2 {
		t.Errorf("second bucket = %+v, want %s with pending 2", got.Samples[1], start.Add(time.Minute))
	}
}
