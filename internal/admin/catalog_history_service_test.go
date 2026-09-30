package admin

import (
	"context"
	"testing"
	"time"
)

func TestServiceGetCatalogBacklogHistory_PassesBucketSeconds(t *testing.T) {
	repo := &fakeRepo{}
	resp, err := newTestService(repo, "", "").GetCatalogBacklogHistory(context.Background(), "ns1", 168*time.Hour, 30*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.backlogHistoryBucket != 1800 {
		t.Errorf("repo bucketSeconds = %d, want 1800", repo.backlogHistoryBucket)
	}
	if resp.WindowSeconds != 604800 || resp.BucketSeconds != 1800 {
		t.Errorf("resp window/bucket = %d/%d, want 604800/1800", resp.WindowSeconds, resp.BucketSeconds)
	}
	if resp.Samples == nil {
		t.Error("samples must be [] not nil so the JSON body is an array")
	}
}
