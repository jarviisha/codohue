package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// fakeCatalogIngestor implements catalogIngestor for testing.
type fakeCatalogIngestor struct {
	err      error
	called   int
	lastItem *codohuetypes.CatalogStreamItem
}

func (f *fakeCatalogIngestor) IngestStreamItem(_ context.Context, item *codohuetypes.CatalogStreamItem) error {
	f.called++
	f.lastItem = item
	return f.err
}

func catalogMessage(t *testing.T, id string, item codohuetypes.CatalogStreamItem) redis.XMessage {
	t.Helper()
	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return redis.XMessage{ID: id, Values: map[string]any{codohuetypes.PayloadField: string(payload)}}
}

func TestCatalogWorkerHandleMessage_ValidItemIngestedAndAcked(t *testing.T) {
	acked := []string{}
	svc := &fakeCatalogIngestor{}
	w := &CatalogWorker{service: svc, ackFn: ackRecorder(&acked)}

	w.handleMessage(context.Background(), catalogMessage(t, "1-0", codohuetypes.CatalogStreamItem{
		Namespace: "ns", ObjectID: "post-1", Content: "hello", AuthorSubjectID: "u-1",
	}))

	if svc.called != 1 || svc.lastItem.ObjectID != "post-1" || svc.lastItem.AuthorSubjectID != "u-1" {
		t.Fatalf("service not called with the item: %+v", svc.lastItem)
	}
	if len(acked) != 1 || acked[0] != "1-0" {
		t.Fatalf("expected ack after successful ingest, got %v", acked)
	}
}

func TestCatalogWorkerHandleMessage_MalformedPayloadAckedAndDropped(t *testing.T) {
	acked := []string{}
	svc := &fakeCatalogIngestor{}
	w := &CatalogWorker{service: svc, ackFn: ackRecorder(&acked)}

	w.handleMessage(context.Background(), redis.XMessage{ID: "1-0", Values: map[string]any{codohuetypes.PayloadField: "not-json"}})

	if svc.called != 0 {
		t.Fatal("service must not be called for a malformed entry")
	}
	if len(acked) != 1 {
		t.Fatalf("malformed entry must be acked off the stream, got %v", acked)
	}
}

func TestCatalogWorkerHandleMessage_MissingNamespaceAckedAndDropped(t *testing.T) {
	acked := []string{}
	svc := &fakeCatalogIngestor{}
	w := &CatalogWorker{service: svc, ackFn: ackRecorder(&acked)}

	w.handleMessage(context.Background(), catalogMessage(t, "1-0", codohuetypes.CatalogStreamItem{
		ObjectID: "post-1", Content: "hello",
	}))

	if svc.called != 0 || len(acked) != 1 {
		t.Fatalf("namespace-less entry must be dropped without an ingest call: called=%d acked=%v", svc.called, acked)
	}
}

func TestCatalogWorkerHandleMessage_RejectedItemAckedAndDropped(t *testing.T) {
	acked := []string{}
	svc := &fakeCatalogIngestor{err: fmt.Errorf("%w: content is empty", ErrCatalogItemRejected)}
	w := &CatalogWorker{service: svc, ackFn: ackRecorder(&acked)}

	w.handleMessage(context.Background(), catalogMessage(t, "1-0", codohuetypes.CatalogStreamItem{
		Namespace: "ns", ObjectID: "post-1", Content: "  ",
	}))

	if len(acked) != 1 {
		t.Fatalf("permanently rejected entry must be acked, got %v", acked)
	}
}

func TestCatalogWorkerHandleMessage_TransientErrorLeavesEntryPending(t *testing.T) {
	acked := []string{}
	svc := &fakeCatalogIngestor{err: errors.New("db down")}
	w := &CatalogWorker{service: svc, ackFn: ackRecorder(&acked)}

	w.handleMessage(context.Background(), catalogMessage(t, "1-0", codohuetypes.CatalogStreamItem{
		Namespace: "ns", ObjectID: "post-1", Content: "hello",
	}))

	if len(acked) != 0 {
		t.Fatalf("transient failure must leave the entry pending for the reaper, got %v", acked)
	}
}

func TestCatalogWorkerHandleMessage_RedeliveryIsIdempotent(t *testing.T) {
	// At-least-once delivery redelivers after a crash between ingest and ack.
	// The second delivery must ingest again (the content-hash short-circuit
	// makes it a no-op upsert) and ack — never error, never duplicate work
	// visible to the caller.
	acked := []string{}
	svc := &fakeCatalogIngestor{}
	w := &CatalogWorker{service: svc, ackFn: ackRecorder(&acked)}

	msg := catalogMessage(t, "1-0", codohuetypes.CatalogStreamItem{Namespace: "ns", ObjectID: "post-1", Content: "hello"})
	w.handleMessage(context.Background(), msg)
	w.handleMessage(context.Background(), msg)

	if svc.called != 2 || len(acked) != 2 {
		t.Fatalf("redelivery must re-ingest and re-ack: called=%d acked=%v", svc.called, acked)
	}
}
