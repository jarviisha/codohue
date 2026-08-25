package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
)

// fakeProcessor implements eventProcessor for testing.
type fakeProcessor struct {
	processErr    error
	processCalled bool
	lastPayload   *EventPayload
}

type fakeLifecycleEvaluator struct {
	disposition nslifecycle.EnvelopeDisposition
	err         error
	calls       int
}

func (f *fakeLifecycleEvaluator) EvaluateEnvelope(context.Context, string, *int64) (nslifecycle.EnvelopeDisposition, error) {
	f.calls++
	return f.disposition, f.err
}

func (f *fakeProcessor) Process(_ context.Context, p *EventPayload) (int64, error) {
	f.processCalled = true
	f.lastPayload = p
	return 0, f.processErr
}

// ackRecorder returns an ackFn capturing acked ids into the given slice.
func ackRecorder(acked *[]string) func(context.Context, string, string, ...string) error {
	return func(_ context.Context, _, _ string, ids ...string) error {
		*acked = append(*acked, ids...)
		return nil
	}
}

func eventMessage(t *testing.T, id string) redis.XMessage {
	t.Helper()
	payload, err := json.Marshal(EventPayload{Namespace: "ns", SubjectID: "u1", ObjectID: "o1", Action: ActionView})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return redis.XMessage{ID: id, Values: map[string]any{"payload": string(payload)}}
}

func TestWorkerHandleMessage_MissingPayloadAckedAndDropped(t *testing.T) {
	acked := []string{}
	w := &Worker{ackFn: ackRecorder(&acked)}

	w.handleMessage(context.Background(), redis.XMessage{ID: "1-0", Values: map[string]any{}})

	if len(acked) != 1 || acked[0] != "1-0" {
		t.Fatalf("expected malformed entry to be acked, got %v", acked)
	}
}

func TestWorkerHandleMessage_InvalidJSONAckedAndDropped(t *testing.T) {
	acked := []string{}
	w := &Worker{ackFn: ackRecorder(&acked)}

	w.handleMessage(context.Background(), redis.XMessage{ID: "1-0", Values: map[string]any{"payload": "not-valid-json"}})

	if len(acked) != 1 {
		t.Fatalf("expected invalid JSON entry to be acked, got %v", acked)
	}
}

func TestWorkerHandleMessage_ValidPayloadProcessedAndAcked(t *testing.T) {
	proc := &fakeProcessor{}
	acked := []string{}
	w := &Worker{service: proc, ackFn: ackRecorder(&acked)}

	w.handleMessage(context.Background(), eventMessage(t, "1-0"))

	if !proc.processCalled {
		t.Fatal("expected service.Process to be called")
	}
	if proc.lastPayload.Namespace != "ns" || proc.lastPayload.Action != ActionView {
		t.Errorf("unexpected payload: %+v", proc.lastPayload)
	}
	if len(acked) != 1 || acked[0] != "1-0" {
		t.Fatalf("expected processed entry to be acked, got %v", acked)
	}
}

func TestWorkerHandleMessage_TransientErrorLeavesEntryPending(t *testing.T) {
	proc := &fakeProcessor{processErr: errors.New("db down")}
	acked := []string{}
	w := &Worker{service: proc, ackFn: ackRecorder(&acked)}

	w.handleMessage(context.Background(), eventMessage(t, "1-0"))

	if len(acked) != 0 {
		t.Fatalf("transient failure must not ack, got %v", acked)
	}
}

func TestWorkerLifecycleStaleACKsAndStoreFailureRetries(t *testing.T) {
	for name, tc := range map[string]struct {
		evaluator   *fakeLifecycleEvaluator
		wantACK     bool
		wantProcess bool
	}{
		"stale":         {&fakeLifecycleEvaluator{disposition: nslifecycle.EnvelopeStale}, true, false},
		"store failure": {&fakeLifecycleEvaluator{err: errors.New("postgres down")}, false, false},
		"accepted":      {&fakeLifecycleEvaluator{disposition: nslifecycle.EnvelopeProcess}, true, true},
	} {
		t.Run(name, func(t *testing.T) {
			acked := []string{}
			processor := &fakeProcessor{}
			worker := &Worker{service: processor, lifecycle: tc.evaluator, ackFn: ackRecorder(&acked)}
			worker.handleMessage(context.Background(), eventMessage(t, "1-0"))
			if (len(acked) == 1) != tc.wantACK || processor.processCalled != tc.wantProcess {
				t.Fatalf("acked=%v processed=%v", acked, processor.processCalled)
			}
		})
	}
}

func TestWorkerHandleMessage_PermanentErrorAckedAndDropped(t *testing.T) {
	for _, sentinel := range []error{ErrInvalidPayload, ErrUnknownAction, ErrNamespaceNotFound} {
		proc := &fakeProcessor{processErr: fmt.Errorf("wrapped: %w", sentinel)}
		acked := []string{}
		w := &Worker{service: proc, ackFn: ackRecorder(&acked)}

		w.handleMessage(context.Background(), eventMessage(t, "1-0"))

		if len(acked) != 1 {
			t.Fatalf("%v: expected poison entry to be acked, got %v", sentinel, acked)
		}
	}
}

func TestNewWorker(t *testing.T) {
	svc := &Service{}
	w := NewWorker(nil, svc, "replica-a")
	if w == nil || w.service != svc {
		t.Fatal("expected worker to wire dependencies")
	}
	if w.consumer != "replica-a" {
		t.Fatalf("consumer: got %q want %q", w.consumer, "replica-a")
	}
}

func TestNewWorker_EmptyConsumerFallsBack(t *testing.T) {
	w := NewWorker(nil, &Service{}, "")
	if w.consumer != defaultConsumerName {
		t.Fatalf("consumer: got %q want %q", w.consumer, defaultConsumerName)
	}
}

func TestWorkerInit_AllowsBusyGroup(t *testing.T) {
	w := &Worker{
		createGroupFn: func(_ context.Context, _, _, _ string) error {
			return errors.New("BUSYGROUP Consumer Group name already exists")
		},
	}
	if err := w.Init(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWorkerInit_ReturnsCreateGroupError(t *testing.T) {
	w := &Worker{
		createGroupFn: func(_ context.Context, _, _, _ string) error {
			return errors.New("redis down")
		},
	}
	if err := w.Init(context.Background()); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestWorkerRun_StopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	w := &Worker{
		readGroupFn: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
			t.Fatal("readGroupFn should not be called after cancellation")
			return nil, nil
		},
	}

	w.Run(ctx)
}

func TestWorkerRun_ContinuesOnReadErrorAndAcksProcessedMessages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proc := &fakeProcessor{}
	readCalls := 0
	acked := []string{}
	w := &Worker{
		service: proc,
		readGroupFn: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
			readCalls++
			switch readCalls {
			case 1:
				return nil, redis.Nil // empty block window must not back off
			case 2:
				cancel()
				return []redis.XStream{{
					Stream:   streamName,
					Messages: []redis.XMessage{eventMessage(t, "1-0")},
				}}, nil
			default:
				return nil, context.Canceled
			}
		},
		ackFn: ackRecorder(&acked),
	}

	w.Run(ctx)

	if readCalls < 2 {
		t.Fatalf("expected at least 2 read attempts, got %d", readCalls)
	}
	if !proc.processCalled {
		t.Fatal("expected payload to be processed")
	}
	if len(acked) != 1 || acked[0] != "1-0" {
		t.Fatalf("unexpected acked ids: %v", acked)
	}
}

func TestWorkerRun_RecreatesGroupOnNoGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	groupCreated := false
	w := &Worker{
		readGroupFn: func(_ context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
			return nil, errors.New("NOGROUP No such key 'codohue:events' or consumer group")
		},
		createGroupFn: func(_ context.Context, stream, group, _ string) error {
			groupCreated = true
			if stream != streamName || group != consumerGroup {
				t.Errorf("unexpected group recreate args: %s/%s", stream, group)
			}
			cancel() // stop the run loop during the post-error backoff
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

func TestWorkerReapOnce_ReprocessesClaimedEntries(t *testing.T) {
	proc := &fakeProcessor{}
	acked := []string{}
	w := &Worker{
		service: proc,
		autoClaimFn: func(_ context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			if args.Stream != streamName || args.Group != consumerGroup {
				t.Errorf("unexpected autoclaim args: %s/%s", args.Stream, args.Group)
			}
			return []redis.XMessage{eventMessage(t, "9-0")}, "0-0", nil
		},
		ackFn: ackRecorder(&acked),
	}

	w.reapOnce(context.Background())

	if !proc.processCalled {
		t.Fatal("expected claimed entry to be processed")
	}
	if len(acked) != 1 || acked[0] != "9-0" {
		t.Fatalf("unexpected acked ids: %v", acked)
	}
}

func TestWorkerReapOnce_ToleratesAutoClaimError(t *testing.T) {
	w := &Worker{
		autoClaimFn: func(_ context.Context, _ *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			return nil, "", errors.New("redis down")
		},
	}
	w.reapOnce(context.Background()) // must not panic or ack anything
}

// ─── reclaim cursor fairness ─────────────────────────────────────────────────

// XAUTOCLAIM returns a cursor, and the next call must start from it. Restarting
// from "0-0" every tick re-examines the same head of the PEL forever: an entry
// that keeps failing at the front starves every entry behind it, which no
// amount of waiting fixes.
func TestWorkerReapOnce_ContinuesFromTheReturnedCursor(t *testing.T) {
	var starts []string
	w := &Worker{
		service: &fakeProcessor{},
		autoClaimFn: func(_ context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			starts = append(starts, args.Start)
			switch len(starts) {
			case 1:
				return nil, "100-0", nil
			case 2:
				return nil, "200-0", nil
			default:
				return nil, "0-0", nil
			}
		},
		ackFn: func(context.Context, string, string, ...string) error { return nil },
	}

	w.reapOnce(context.Background())

	if len(starts) != 3 {
		t.Fatalf("expected 3 pages before the terminal cursor, got %v", starts)
	}
	if starts[0] != "0-0" || starts[1] != "100-0" || starts[2] != "200-0" {
		t.Errorf("cursor not carried across pages: %v", starts)
	}
	if w.reapCursor != "0-0" {
		t.Errorf("terminal cursor must reset to 0-0, got %q", w.reapCursor)
	}
}

// A page with nothing eligible still advances: stopping on an empty page would
// pin the cursor in front of entries that are not idle *yet*.
func TestWorkerReapOnce_EmptyPageStillAdvances(t *testing.T) {
	var starts []string
	w := &Worker{
		service: &fakeProcessor{},
		autoClaimFn: func(_ context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			starts = append(starts, args.Start)
			if len(starts) == 1 {
				return nil, "500-0", nil // empty page, non-terminal cursor
			}
			return nil, "0-0", nil
		},
		ackFn: func(context.Context, string, string, ...string) error { return nil },
	}

	w.reapOnce(context.Background())

	if len(starts) != 2 || starts[1] != "500-0" {
		t.Errorf("an empty page must not stop the scan: %v", starts)
	}
}

// One tick may not scan the whole PEL — that would let a large backlog block
// the reaper indefinitely. The cursor survives the tick, so the next one picks
// up exactly where this one stopped.
func TestWorkerReapOnce_StopsAtThePageBudgetAndResumesNextTick(t *testing.T) {
	calls := 0
	w := &Worker{
		service: &fakeProcessor{},
		autoClaimFn: func(_ context.Context, _ *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			calls++
			return nil, fmt.Sprintf("%d-0", calls), nil // never terminal
		},
		ackFn: func(context.Context, string, string, ...string) error { return nil },
	}

	w.reapOnce(context.Background())

	if calls != reapPageBudget {
		t.Fatalf("scanned %d pages, want the %d-page budget", calls, reapPageBudget)
	}
	carried := w.reapCursor
	if carried == "0-0" || carried == "" {
		t.Fatalf("cursor must survive the tick, got %q", carried)
	}

	var resumedFrom string
	w.autoClaimFn = func(_ context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
		resumedFrom = args.Start
		return nil, "0-0", nil
	}
	w.reapOnce(context.Background())

	if resumedFrom != carried {
		t.Errorf("next tick resumed from %q, want %q", resumedFrom, carried)
	}
}

// An error says nothing about where the PEL scan had got to, so the cursor is
// kept: resetting would re-walk ground already covered, and advancing would
// skip entries that were never examined.
func TestWorkerReapOnce_ErrorRetainsTheCursor(t *testing.T) {
	w := &Worker{
		service:    &fakeProcessor{},
		reapCursor: "700-0",
		autoClaimFn: func(_ context.Context, _ *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			return nil, "", errors.New("redis down")
		},
	}

	w.reapOnce(context.Background())

	if w.reapCursor != "700-0" {
		t.Errorf("cursor after an error = %q, want it retained at 700-0", w.reapCursor)
	}
}

// NOGROUP means the group was destroyed and recreated, so any cursor into the
// old PEL is meaningless — this is the one case that resets.
func TestWorkerReapOnce_NoGroupResetsTheCursor(t *testing.T) {
	w := &Worker{
		service:    &fakeProcessor{},
		reapCursor: "700-0",
		autoClaimFn: func(_ context.Context, _ *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			return nil, "", errors.New("NOGROUP No such consumer group")
		},
	}

	w.reapOnce(context.Background())

	if w.reapCursor != "0-0" {
		t.Errorf("cursor after NOGROUP = %q, want 0-0", w.reapCursor)
	}
}

// A permanently failing entry at the head of the PEL must not hold up the rest:
// the scan moves past it and later entries are still processed this tick.
func TestWorkerReapOnce_PermanentlyFailingHeadDoesNotStarveTheTail(t *testing.T) {
	proc := &fakeProcessor{processErr: errors.New("still failing")}
	var processed []string
	page := 0
	w := &Worker{
		service: proc,
		autoClaimFn: func(_ context.Context, _ *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			page++
			switch page {
			case 1:
				return []redis.XMessage{eventMessage(t, "1-0")}, "50-0", nil
			case 2:
				return []redis.XMessage{eventMessage(t, "2-0")}, "0-0", nil
			default:
				return nil, "0-0", nil
			}
		},
		ackFn: func(_ context.Context, _, _ string, ids ...string) error {
			processed = append(processed, ids...)
			return nil
		},
	}

	w.reapOnce(context.Background())

	if page < 2 {
		t.Fatalf("a failing entry stopped the scan after %d page(s)", page)
	}
	// Neither entry is acked (both still failing), but both were attempted.
	if len(processed) != 0 {
		t.Errorf("transiently failing entries must stay pending, acked %v", processed)
	}
}
