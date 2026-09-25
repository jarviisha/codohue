package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/jarviisha/codohue/internal/infra/metrics"
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

// fakeStreamClient fakes the catalogStreamClient collaborator; ops a test
// leaves unset panic when called, failing the test just as the old nil
// function fields did.
type fakeStreamClient struct {
	createGroup func(ctx context.Context, stream, group, start string) error
	readGroup   func(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XStream, error)
	autoClaim   func(ctx context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error)
	ack         func(ctx context.Context, stream, group string, ids ...string) error
}

func (f *fakeStreamClient) XGroupCreateMkStream(ctx context.Context, stream, group, start string) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(ctx)
	cmd.SetErr(f.createGroup(ctx, stream, group, start))
	return cmd
}

func (f *fakeStreamClient) XReadGroup(ctx context.Context, args *redis.XReadGroupArgs) *redis.XStreamSliceCmd {
	cmd := redis.NewXStreamSliceCmd(ctx)
	streams, err := f.readGroup(ctx, args)
	cmd.SetVal(streams)
	cmd.SetErr(err)
	return cmd
}

func (f *fakeStreamClient) XAutoClaim(ctx context.Context, args *redis.XAutoClaimArgs) *redis.XAutoClaimCmd {
	cmd := redis.NewXAutoClaimCmd(ctx)
	msgs, next, err := f.autoClaim(ctx, args)
	cmd.SetVal(msgs, next)
	cmd.SetErr(err)
	return cmd
}

func (f *fakeStreamClient) XAck(ctx context.Context, stream, group string, ids ...string) *redis.IntCmd {
	cmd := redis.NewIntCmd(ctx)
	cmd.SetErr(f.ack(ctx, stream, group, ids...))
	return cmd
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
	w := NewCatalogWorker(&fakeStreamClient{ack: ackRecorder(&acked)}, svc, "")

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
	w := NewCatalogWorker(&fakeStreamClient{ack: ackRecorder(&acked)}, svc, "")

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
	w := NewCatalogWorker(&fakeStreamClient{ack: ackRecorder(&acked)}, svc, "")

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
	w := NewCatalogWorker(&fakeStreamClient{ack: ackRecorder(&acked)}, svc, "")

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
	w := NewCatalogWorker(&fakeStreamClient{ack: ackRecorder(&acked)}, svc, "")

	w.handleMessage(context.Background(), catalogMessage(t, "1-0", codohuetypes.CatalogStreamItem{
		Namespace: "ns", ObjectID: "post-1", Content: "hello",
	}))

	if len(acked) != 0 {
		t.Fatalf("transient failure must leave the entry pending for the reaper, got %v", acked)
	}
}

func TestCatalogWorkerLifecycleStaleACKsAndStoreFailureRetries(t *testing.T) {
	for name, tc := range map[string]struct {
		evaluator *fakeLifecycleEvaluator
		wantACK   bool
		wantCall  bool
	}{
		"stale":         {&fakeLifecycleEvaluator{disposition: nslifecycle.EnvelopeStale}, true, false},
		"store failure": {&fakeLifecycleEvaluator{err: errors.New("postgres down")}, false, false},
		"accepted":      {&fakeLifecycleEvaluator{disposition: nslifecycle.EnvelopeProcess}, true, true},
	} {
		t.Run(name, func(t *testing.T) {
			acked := []string{}
			svc := &fakeCatalogIngestor{}
			worker := NewCatalogWorker(&fakeStreamClient{ack: ackRecorder(&acked)}, svc, "")
			worker.SetLifecycleEvaluator(tc.evaluator)
			worker.handleMessage(context.Background(), catalogMessage(t, "1-0", codohuetypes.CatalogStreamItem{Namespace: "ns", NamespaceGeneration: 2}))
			if (len(acked) == 1) != tc.wantACK || (svc.called == 1) != tc.wantCall {
				t.Fatalf("acked=%v calls=%d", acked, svc.called)
			}
		})
	}
}

func TestCatalogWorkerHandleMessage_RedeliveryIsIdempotent(t *testing.T) {
	// At-least-once delivery redelivers after a crash between ingest and ack.
	// The second delivery must ingest again (the content-hash short-circuit
	// makes it a no-op upsert) and ack — never error, never duplicate work
	// visible to the caller.
	acked := []string{}
	svc := &fakeCatalogIngestor{}
	w := NewCatalogWorker(&fakeStreamClient{ack: ackRecorder(&acked)}, svc, "")

	msg := catalogMessage(t, "1-0", codohuetypes.CatalogStreamItem{Namespace: "ns", ObjectID: "post-1", Content: "hello"})
	w.handleMessage(context.Background(), msg)
	w.handleMessage(context.Background(), msg)

	if svc.called != 2 || len(acked) != 2 {
		t.Fatalf("redelivery must re-ingest and re-ack: called=%d acked=%v", svc.called, acked)
	}
}

func TestCatalogWorkerInit_AllowsBusyGroup(t *testing.T) {
	w := NewCatalogWorker(&fakeStreamClient{createGroup: func(_ context.Context, stream, group, _ string) error {
		if stream != codohuetypes.CatalogStreamName || group != catalogConsumerGroup {
			t.Errorf("unexpected group args: %s/%s", stream, group)
		}
		return errors.New("BUSYGROUP Consumer Group name already exists")
	}}, nil, "")
	if err := w.Init(context.Background()); err != nil {
		t.Fatalf("BUSYGROUP must be tolerated: %v", err)
	}
}

func TestCatalogWorkerInit_ReturnsCreateGroupError(t *testing.T) {
	w := NewCatalogWorker(&fakeStreamClient{createGroup: func(_ context.Context, _, _, _ string) error {
		return errors.New("redis down")
	}}, nil, "")
	if err := w.Init(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestCatalogWorkerRun_StopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := NewCatalogWorker(&fakeStreamClient{readGroup: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
		t.Fatal("readGroup should not be called after cancellation")
		return nil, nil
	}}, nil, "")
	w.Run(ctx)
}

func TestCatalogWorkerRun_ContinuesOnReadErrorAndAcksProcessed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc := &fakeCatalogIngestor{}
	readCalls := 0
	acked := []string{}
	w := NewCatalogWorker(&fakeStreamClient{
		readGroup: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
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
		ack: ackRecorder(&acked),
	}, svc, "")

	w.Run(ctx)

	if svc.called != 1 || len(acked) != 1 || acked[0] != "1-0" {
		t.Fatalf("expected one ingested + acked entry: called=%d acked=%v", svc.called, acked)
	}
}

func TestCatalogWorkerRun_RecreatesGroupOnNoGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	groupCreated := false
	w := NewCatalogWorker(&fakeStreamClient{
		readGroup: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
			return nil, errors.New("NOGROUP No such key 'codohue:catalog' or consumer group")
		},
		createGroup: func(_ context.Context, stream, group, _ string) error {
			groupCreated = true
			if stream != codohuetypes.CatalogStreamName || group != catalogConsumerGroup {
				t.Errorf("unexpected group recreate args: %s/%s", stream, group)
			}
			cancel()
			return nil
		},
	}, nil, "")

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
	w := NewCatalogWorker(&fakeStreamClient{
		autoClaim: func(_ context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			if args.Stream != codohuetypes.CatalogStreamName || args.Group != catalogConsumerGroup {
				t.Errorf("unexpected autoclaim args: %s/%s", args.Stream, args.Group)
			}
			return []redis.XMessage{catalogMessage(t, "9-0", codohuetypes.CatalogStreamItem{Namespace: "ns", ObjectID: "o1", Content: "hi"})}, "0-0", nil
		},
		ack: ackRecorder(&acked),
	}, svc, "")

	w.reapOnce(context.Background())

	if svc.called != 1 || len(acked) != 1 || acked[0] != "9-0" {
		t.Fatalf("expected claimed entry reprocessed + acked: called=%d acked=%v", svc.called, acked)
	}
}

func TestCatalogWorkerReapOnce_ToleratesAutoClaimError(t *testing.T) {
	w := NewCatalogWorker(&fakeStreamClient{autoClaim: func(_ context.Context, _ *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
		return nil, "", errors.New("redis down")
	}}, nil, "")
	w.reapOnce(context.Background())
}

func TestCatalogWorkerAck_LogsAckFailure(t *testing.T) {
	w := NewCatalogWorker(&fakeStreamClient{ack: func(_ context.Context, _, _ string, _ ...string) error {
		return errors.New("ack failed")
	}}, nil, "")
	w.ack(context.Background(), "1-0") // must not panic
}

// The catalog stream's PEL is scanned under the same fairness rules as the
// event stream: continue from the returned cursor, keep it across ticks, and
// reset only at a terminal cursor or after the group is recreated. Restarting
// at "0-0" every tick lets one permanently failing entry at the head starve
// every catalog item behind it.
func TestCatalogWorkerReapOnce_CursorFairness(t *testing.T) {
	t.Run("continues from the returned cursor and resets at terminal", func(t *testing.T) {
		var starts []string
		w := NewCatalogWorker(&fakeStreamClient{
			autoClaim: func(_ context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
				starts = append(starts, args.Start)
				if len(starts) == 1 {
					return nil, "300-0", nil
				}
				return nil, "0-0", nil
			},
			ack: func(context.Context, string, string, ...string) error { return nil },
		}, &fakeCatalogIngestor{}, "")

		w.reapOnce(context.Background())

		if len(starts) != 2 || starts[0] != "0-0" || starts[1] != "300-0" {
			t.Errorf("cursor not carried: %v", starts)
		}
		if w.reapCursor != "0-0" {
			t.Errorf("terminal cursor must reset, got %q", w.reapCursor)
		}
	})

	t.Run("stops at the page budget and keeps the cursor", func(t *testing.T) {
		calls := 0
		w := NewCatalogWorker(&fakeStreamClient{
			autoClaim: func(_ context.Context, _ *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
				calls++
				return nil, fmt.Sprintf("%d-0", calls), nil
			},
			ack: func(context.Context, string, string, ...string) error { return nil },
		}, &fakeCatalogIngestor{}, "")

		w.reapOnce(context.Background())

		if calls != reapPageBudget {
			t.Errorf("scanned %d pages, want %d", calls, reapPageBudget)
		}
		if w.reapCursor == "0-0" || w.reapCursor == "" {
			t.Errorf("cursor must survive the tick, got %q", w.reapCursor)
		}
	})

	t.Run("error retains, NOGROUP resets", func(t *testing.T) {
		client := &fakeStreamClient{
			autoClaim: func(_ context.Context, _ *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
				return nil, "", errors.New("redis down")
			},
		}
		w := NewCatalogWorker(client, &fakeCatalogIngestor{}, "")
		w.reapCursor = "800-0"
		w.reapOnce(context.Background())
		if w.reapCursor != "800-0" {
			t.Errorf("cursor after error = %q, want it retained", w.reapCursor)
		}

		client.autoClaim = func(_ context.Context, _ *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			return nil, "", errors.New("NOGROUP No such consumer group")
		}
		w.reapOnce(context.Background())
		if w.reapCursor != "0-0" {
			t.Errorf("cursor after NOGROUP = %q, want 0-0", w.reapCursor)
		}
	})
}

// TestCatalogWorkerReapOnce_RecordsTheTerminalOutcome pins the reclaim-cycle
// counter for the catalog stream. The metric was registered but never observed,
// so it read zero forever; without an assertion at the call site that regresses
// silently.
func TestCatalogWorkerReapOnce_RecordsTheTerminalOutcome(t *testing.T) {
	// Process-global counter, and the sibling reap tests bump it, so start from
	// a known zero rather than from whatever ran first.
	metrics.StreamReclaimCyclesTotal.Reset()
	t.Cleanup(metrics.StreamReclaimCyclesTotal.Reset)

	outcome := func(name string) float64 {
		return testutil.ToFloat64(metrics.StreamReclaimCyclesTotal.WithLabelValues("catalog", "", name))
	}

	client := &fakeStreamClient{
		autoClaim: func(context.Context, *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			return nil, "0-0", nil
		},
	}
	w := NewCatalogWorker(client, nil, "")
	w.reapOnce(context.Background())
	if got := outcome("terminal"); got != 1 {
		t.Errorf("terminal = %v, want 1", got)
	}

	client.autoClaim = func(context.Context, *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
		return nil, "", errors.New("redis down")
	}
	w.reapOnce(context.Background())
	if got := outcome("error"); got != 1 {
		t.Errorf("error = %v, want 1", got)
	}

	calls := 0
	w.reapCursor = "0-0"
	client.autoClaim = func(context.Context, *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
		calls++
		return nil, fmt.Sprintf("%d-0", calls), nil
	}
	w.reapOnce(context.Background())
	if got := outcome("budget_exhausted"); got != 1 {
		t.Errorf("budget_exhausted = %v, want 1", got)
	}
	// The catalog counter must not be attributed to the events stream.
	if got := testutil.ToFloat64(metrics.StreamReclaimCyclesTotal.WithLabelValues("events", "", "terminal")); got != 0 {
		t.Errorf("events/terminal = %v, want the catalog pass not to touch it", got)
	}
}

// TestCatalogWorkerReapOnce_CountsReclaimedEntries pins the companion counter,
// which reports how much the scan actually took over rather than that it ran.
func TestCatalogWorkerReapOnce_CountsReclaimedEntries(t *testing.T) {
	metrics.StreamReclaimedTotal.Reset()
	t.Cleanup(metrics.StreamReclaimedTotal.Reset)

	// Entries carry no payload field, so each is acked and dropped as malformed
	// — this test is about the count, not about what the entries mean.
	w := NewCatalogWorker(&fakeStreamClient{
		autoClaim: func(context.Context, *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			return []redis.XMessage{{ID: "1-0"}, {ID: "2-0"}}, "0-0", nil
		},
		ack: func(context.Context, string, string, ...string) error { return nil },
	}, nil, "")
	w.reapOnce(context.Background())

	if got := testutil.ToFloat64(metrics.StreamReclaimedTotal.WithLabelValues("catalog", "")); got != 2 {
		t.Errorf("catalog reclaimed = %v, want 2", got)
	}
	if got := testutil.ToFloat64(metrics.StreamReclaimedTotal.WithLabelValues("events", "")); got != 0 {
		t.Errorf("events reclaimed = %v, want the catalog pass not to touch it", got)
	}
}
