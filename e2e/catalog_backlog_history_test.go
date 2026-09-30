//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// Bucketing groups samples by the floor of their epoch second and keeps, per
// bucket, the one sample with the largest backlog total. A sample in the last
// half-second of a bucket must stay in that bucket (a rounding cast would push
// it into the next one). Taking MAX per series instead would mix rows: the
// stacked chart would show pending 9 + in-flight 12 = 21, a total that never
// existed when items merely moved from pending to in-flight.
func TestAdmin_CatalogBacklogHistoryBucketsKeepTheLargestSample(t *testing.T) {
	ns, _ := createIsolatedNamespace(t, "backlog", defaultNamespaceConfig())
	cookie := adminLogin(t)

	start := time.Now().Add(-10 * time.Minute).Truncate(time.Minute)
	for _, s := range []struct {
		offset            time.Duration
		pending, inFlight int
	}{
		{200 * time.Millisecond, 1, 0},
		{30 * time.Second, 9, 0},
		{59700 * time.Millisecond, 0, 12},
		{60100 * time.Millisecond, 2, 0},
	} {
		if _, err := testDB.Exec(context.Background(), `
			INSERT INTO catalog_backlog_samples
			  (namespace, sampled_at, pending, in_flight, failed, dead_letter, stream_len)
			VALUES ($1, $2, $3, $4, 0, 0, 0)`, ns, start.Add(s.offset), s.pending, s.inFlight); err != nil {
			t.Fatalf("seed sample: %v", err)
		}
	}

	type sample struct {
		SampledAt time.Time `json:"sampled_at"`
		Pending   int       `json:"pending"`
		InFlight  int       `json:"in_flight"`
	}
	var got struct {
		BucketSeconds int      `json:"bucket_seconds"`
		Samples       []sample `json:"samples"`
	}
	resp := adminRequest(t, http.MethodGet,
		"/api/admin/v1/namespaces/"+ns+"/catalog/backlog-history?window=1h&bucket=1m", cookie, nil)
	decodeJSON(t, resp, &got)

	want := []sample{
		{SampledAt: start, Pending: 0, InFlight: 12},
		{SampledAt: start.Add(time.Minute), Pending: 2, InFlight: 0},
	}
	if got.BucketSeconds != 60 || len(got.Samples) != len(want) {
		t.Fatalf("want 2 one-minute buckets, got bucket=%d samples=%+v", got.BucketSeconds, got.Samples)
	}
	for i, w := range want {
		g := got.Samples[i]
		if !g.SampledAt.Equal(w.SampledAt) || g.Pending != w.Pending || g.InFlight != w.InFlight {
			t.Errorf("bucket %d = %+v, want %+v", i, g, w)
		}
	}
}
