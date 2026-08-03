package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

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

func TestCatalogWorkerInit_AllowsBusyGroup(t *testing.T) {
	w := &CatalogWorker{createGroupFn: func(_ context.Context, stream, group, _ string) error {
		if stream != codohuetypes.CatalogStreamName || group != catalogConsumerGroup {
			t.Errorf("unexpected group args: %s/%s", stream, group)
		}
		return errors.New("BUSYGROUP Consumer Group name already exists")
	}}
	if err := w.Init(context.Background()); err != nil {
		t.Fatalf("BUSYGROUP must be tolerated: %v", err)
	}
}

func TestCatalogWorkerInit_ReturnsCreateGroupError(t *testing.T) {
	w := &CatalogWorker{createGroupFn: func(_ context.Context, _, _, _ string) error {
		return errors.New("redis down")
	}}
	if err := w.Init(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestCatalogWorkerRun_StopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := &CatalogWorker{readGroupFn: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
		t.Fatal("readGroupFn should not be called after cancellation")
		return nil, nil
	}}
	w.Run(ctx)
}

func TestCatalogWorkerRun_ContinuesOnReadErrorAndAcksProcessed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := &fakeCatalogIngestor{}
	readCalls := 0
	acked := []string{}
	w := &CatalogWorker{
		service: svc,
		readGroupFn: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
			readCalls++
			switch readCalls {
			case 1:
				return nil, redis.Nil
			case 2:
				cancel()
				return []redis.XStream{{
					Stream:   codohuetypes.CatalogStreamName,
					Messages: []redis.XMessage{catalogMessage(t, "1-0", codohuetypes.CatalogStreamItem{Namespace: "ns", ObjectID: "o1", Content: "hi"})},
				}}, nil
			default:
				return nil, context.Canceled
			}
		},
		ackFn: ackRecorder(&acked),
	}

	w.Run(ctx)

	if svc.called != 1 || len(acked) != 1 || acked[0] != "1-0" {
		t.Fatalf("expected one ingested + acked entry: called=%d acked=%v", svc.called, acked)
	}
}

func TestCatalogWorkerRun_RecreatesGroupOnNoGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	groupCreated := false
	w := &CatalogWorker{
		readGroupFn: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
			return nil, errors.New("NOGROUP No such key 'codohue:catalog' or consumer group")
		},
		createGroupFn: func(_ context.Context, stream, group, _ string) error {
			groupCreated = true
			if stream != codohuetypes.CatalogStreamName || group != catalogConsumerGroup {
				t.Errorf("unexpected group recreate args: %s/%s", stream, group)
			}
			cancel()
			return nil
		},
	}

	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not stop")
	}
	if !groupCreated {
		t.Fatal("expected consumer group to be recreated after NOGROUP")
	}
}

func TestCatalogWorkerReapOnce_ReprocessesClaimedEntries(t *testing.T) {
	svc := &fakeCatalogIngestor{}
	acked := []string{}
	w := &CatalogWorker{
		service: svc,
		autoClaimFn: func(_ context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			if args.Stream != codohuetypes.CatalogStreamName || args.Group != catalogConsumerGroup {
				t.Errorf("unexpected autoclaim args: %s/%s", args.Stream, args.Group)
			}
			return []redis.XMessage{catalogMessage(t, "9-0", codohuetypes.CatalogStreamItem{Namespace: "ns", ObjectID: "o1", Content: "hi"})}, "0-0", nil
		},
		ackFn: ackRecorder(&acked),
	}

	w.reapOnce(context.Background())

	if svc.called != 1 || len(acked) != 1 || acked[0] != "9-0" {
		t.Fatalf("expected claimed entry reprocessed + acked: called=%d acked=%v", svc.called, acked)
	}
}

func TestCatalogWorkerReapOnce_ToleratesAutoClaimError(t *testing.T) {
	w := &CatalogWorker{autoClaimFn: func(_ context.Context, _ *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
		return nil, "", errors.New("redis down")
	}}
	w.reapOnce(context.Background())
}

func TestCatalogWorkerAck_LogsAckFailure(t *testing.T) {
	w := &CatalogWorker{ackFn: func(_ context.Context, _, _ string, _ ...string) error {
		return errors.New("ack failed")
	}}
	w.ack(context.Background(), "1-0") // must not panic
}
