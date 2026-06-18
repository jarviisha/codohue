package embedder

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestCatalogEventChannel(t *testing.T) {
	if got := CatalogEventChannel("prod"); got != "codohue:catalog-events:prod" {
		t.Errorf("CatalogEventChannel: got %q", got)
	}
	if CatalogEventChannelPattern != "codohue:catalog-events:*" {
		t.Errorf("pattern: got %q", CatalogEventChannelPattern)
	}
}

// TestRedisCatalogEventPublisher_PublishAll exercises every Publish* method
// plus the shared publish() helper. The Redis client points at a refused port
// so Publish().Err() fails — publish() swallows that, so the methods still run
// to completion. Each method is called twice: once with zero-value Kind/At to
// hit the defaulting branches, once with both set to skip them.
func TestRedisCatalogEventPublisher_PublishAll(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = rdb.Close() })
	pub := NewRedisCatalogEventPublisher(rdb)
	ctx := context.Background()
	preset := time.Now().UTC()

	pub.PublishItemStateChanged(ctx, CatalogItemStateChangedEvent{Namespace: "ns", ItemID: 1, To: "embedded"})
	pub.PublishItemStateChanged(ctx, CatalogItemStateChangedEvent{Kind: "item_state_changed", Namespace: "ns", To: "failed", At: preset})

	pub.PublishBacklogSnapshot(ctx, CatalogBacklogSnapshotEvent{Namespace: "ns"})
	pub.PublishBacklogSnapshot(ctx, CatalogBacklogSnapshotEvent{Kind: "backlog_snapshot", Namespace: "ns", At: preset})

	pub.PublishDeadLetterGrew(ctx, CatalogDeadLetterGrewEvent{Namespace: "ns", Delta: 2})
	pub.PublishDeadLetterGrew(ctx, CatalogDeadLetterGrewEvent{Kind: "dead_letter_grew", Namespace: "ns", At: preset})

	pub.PublishReembedProgress(ctx, CatalogReembedProgressEvent{Namespace: "ns", BatchRunID: 7})
	pub.PublishReembedProgress(ctx, CatalogReembedProgressEvent{Kind: "reembed_progress", Namespace: "ns", At: preset})
}
