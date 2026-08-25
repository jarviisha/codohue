package embedder

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
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

// The generation field is additive: it must vanish from the wire for
// generation-1 namespaces, and a payload written before the field existed must
// still decode.
func TestCatalogItemStateChangedEvent_GenerationWireCompatibility(t *testing.T) {
	legacy, err := json.Marshal(CatalogItemStateChangedEvent{
		Kind: "item_state_changed", Namespace: "ns", ItemID: 1, To: "embedded",
	})
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if strings.Contains(string(legacy), "namespace_generation") {
		t.Errorf("generation 0 must not reach the wire: %s", legacy)
	}

	qualified, err := json.Marshal(CatalogItemStateChangedEvent{
		Kind: "item_state_changed", Namespace: "ns", NamespaceGeneration: 3, ItemID: 1, To: "embedded",
	})
	if err != nil {
		t.Fatalf("marshal qualified: %v", err)
	}
	if !strings.Contains(string(qualified), `"namespace_generation":3`) {
		t.Errorf("generation 3 missing from wire: %s", qualified)
	}

	var decoded CatalogItemStateChangedEvent
	if err := json.Unmarshal([]byte(`{"kind":"item_state_changed","namespace":"ns","item_id":9,"to":"failed"}`), &decoded); err != nil {
		t.Fatalf("decode pre-generation payload: %v", err)
	}
	if decoded.NamespaceGeneration != 0 || decoded.ItemID != 9 {
		t.Errorf("pre-generation payload decoded as %+v", decoded)
	}
}

type recordingEventPublisher struct {
	itemStates []CatalogItemStateChangedEvent
}

func (p *recordingEventPublisher) PublishItemStateChanged(_ context.Context, ev CatalogItemStateChangedEvent) {
	p.itemStates = append(p.itemStates, ev)
}
func (p *recordingEventPublisher) PublishBacklogSnapshot(context.Context, CatalogBacklogSnapshotEvent) {}
func (p *recordingEventPublisher) PublishDeadLetterGrew(context.Context, CatalogDeadLetterGrewEvent)   {}
func (p *recordingEventPublisher) PublishReembedProgress(context.Context, CatalogReembedProgressEvent) {}

// State-change events are stamped from the lifecycle lease held by the
// mutation, never from an argument a caller could get wrong.
func TestServicePublishItemStateChanged_StampsLeaseGeneration(t *testing.T) {
	for _, tc := range []struct {
		name       string
		ctx        context.Context
		wantStamp  int64
		wantStream string
	}{
		{
			name:      "no lease leaves the field unset",
			ctx:       context.Background(),
			wantStamp: 0,
		},
		{
			name:      "generation 1 stays legacy-shaped",
			ctx:       nslifecycle.ContextWithLease(context.Background(), "ns", 1, nslifecycle.LockShared),
			wantStamp: 0,
		},
		{
			name:      "recreated namespace is disambiguated",
			ctx:       nslifecycle.ContextWithLease(context.Background(), "ns", 4, nslifecycle.LockShared),
			wantStamp: 4,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _, _, _, _ := newSvc(t)
			pub := &recordingEventPublisher{}
			svc.SetEventPublisher(pub)

			svc.publishItemStateChanged(tc.ctx, &PendingItem{ID: 7, Namespace: "ns", ObjectID: "obj1"}, "in_flight", "embedded")

			if len(pub.itemStates) != 1 {
				t.Fatalf("expected 1 event, got %d", len(pub.itemStates))
			}
			if got := pub.itemStates[0].NamespaceGeneration; got != tc.wantStamp {
				t.Errorf("namespace_generation: got %d, want %d", got, tc.wantStamp)
			}
		})
	}
}
