package embedder

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

// --- fakes ----------------------------------------------------------------

type fakeStreamClient struct {
	mu         sync.Mutex
	groupErr   error
	readFn     func(ctx context.Context, a *redis.XReadGroupArgs) ([]redis.XStream, error)
	autoFn     func(ctx context.Context, a *redis.XAutoClaimArgs) ([]redis.XMessage, string, error)
	ackErr     error
	ackedIDs   []string
	ackCalls   int
	groupCalls int
}

func (f *fakeStreamClient) XGroupCreateMkStream(ctx context.Context, stream, group, start string) *redis.StatusCmd {
	f.mu.Lock()
	f.groupCalls++
	f.mu.Unlock()
	cmd := redis.NewStatusCmd(ctx, "XGROUP", "CREATE", stream, group, start, "MKSTREAM")
	if f.groupErr != nil {
		cmd.SetErr(f.groupErr)
	} else {
		cmd.SetVal("OK")
	}
	return cmd
}

func (f *fakeStreamClient) XReadGroup(ctx context.Context, a *redis.XReadGroupArgs) *redis.XStreamSliceCmd {
	cmd := redis.NewXStreamSliceCmd(ctx, "XREADGROUP")
	if f.readFn != nil {
		out, err := f.readFn(ctx, a)
		if err != nil {
			cmd.SetErr(err)
		} else {
			cmd.SetVal(out)
		}
		return cmd
	}
	cmd.SetErr(redis.Nil)
	return cmd
}

func (f *fakeStreamClient) XAutoClaim(ctx context.Context, a *redis.XAutoClaimArgs) *redis.XAutoClaimCmd {
	cmd := redis.NewXAutoClaimCmd(ctx, "XAUTOCLAIM")
	if f.autoFn != nil {
		msgs, next, err := f.autoFn(ctx, a)
		if err != nil {
			cmd.SetErr(err)
		} else {
			cmd.SetVal(msgs, next)
		}
		return cmd
	}
	cmd.SetVal(nil, "0-0")
	return cmd
}

func (f *fakeStreamClient) XAck(ctx context.Context, stream, group string, ids ...string) *redis.IntCmd {
	f.mu.Lock()
	f.ackCalls++
	f.ackedIDs = append(f.ackedIDs, ids...)
	f.mu.Unlock()
	cmd := redis.NewIntCmd(ctx, "XACK", stream, group)
	if f.ackErr != nil {
		cmd.SetErr(f.ackErr)
	} else {
		cmd.SetVal(int64(len(ids)))
	}
	return cmd
}

func (f *fakeStreamClient) acked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.ackedIDs))
	copy(out, f.ackedIDs)
	return out
}

type fakeProcessor struct {
	out       ProcessOutcome
	err       error
	calls     int32
	lastID    int64
	processFn func(ctx context.Context, id int64) (ProcessOutcome, error)
}

func (f *fakeProcessor) ProcessItem(ctx context.Context, id int64) (ProcessOutcome, error) {
	atomic.AddInt32(&f.calls, 1)
	atomic.StoreInt64(&f.lastID, id)
	if f.processFn != nil {
		return f.processFn(ctx, id)
	}
	return f.out, f.err
}

type fakeNSLister struct {
	cfgs []*namespace.Config
	err  error
}

func (f *fakeNSLister) ListCatalogEnabled(_ context.Context) ([]*namespace.Config, error) {
	return f.cfgs, f.err
}

// --- helpers --------------------------------------------------------------

func validEntry(id string, catalogItemID int64, ns string) redis.XMessage {
	return redis.XMessage{
		ID: id,
		Values: map[string]any{
			"catalog_item_id":  toStringInt(catalogItemID),
			"namespace":        ns,
			"object_id":        "obj1",
			"strategy_id":      "internal-hashing-ngrams",
			"strategy_version": "v1",
			"enqueued_at":      "2026-05-09T00:00:00Z",
		},
	}
}

func toStringInt(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

func newTestWorker(client *fakeStreamClient, proc *fakeProcessor, lister *fakeNSLister) *Worker {
	return newWorkerWithDeps(client, proc, lister, WorkerConfig{
		ConsumerName:  "test-consumer",
		PollInterval:  50 * time.Millisecond,
		ReapInterval:  50 * time.Millisecond,
		MinIdleReap:   time.Second,
		ReadBlockTime: 10 * time.Millisecond,
		ReadBatchSize: 8,
		ReapBatchSize: 8,
	})
}

// --- handleMessage tests --------------------------------------------------

func TestWorker_HandleMessage_EmbeddedOutcome_ACKs(t *testing.T) {
	client := &fakeStreamClient{}
	proc := &fakeProcessor{out: OutcomeEmbedded}
	w := newTestWorker(client, proc, &fakeNSLister{})

	w.handleMessage(context.Background(), "ns", "catalog:embed:ns", "embedder", validEntry("1-0", 7, "ns"))

	if got := client.acked(); len(got) != 1 || got[0] != "1-0" {
		t.Errorf("expected ACK of 1-0, got %v", got)
	}
	if proc.calls != 1 || proc.lastID != 7 {
		t.Errorf("processor calls=%d lastID=%d", proc.calls, proc.lastID)
	}
}

func TestWorker_HandleMessage_DeadLetterOutcome_ACKs(t *testing.T) {
	client := &fakeStreamClient{}
	proc := &fakeProcessor{out: OutcomeDeadLetter}
	w := newTestWorker(client, proc, &fakeNSLister{})

	w.handleMessage(context.Background(), "ns", "catalog:embed:ns", "embedder", validEntry("1-0", 7, "ns"))
	if len(client.acked()) != 1 {
		t.Errorf("DeadLetter must ACK, got %v", client.acked())
	}
}

func TestWorker_HandleMessage_SkippedOutcome_ACKs(t *testing.T) {
	client := &fakeStreamClient{}
	proc := &fakeProcessor{out: OutcomeSkipped}
	w := newTestWorker(client, proc, &fakeNSLister{})

	w.handleMessage(context.Background(), "ns", "catalog:embed:ns", "embedder", validEntry("1-0", 7, "ns"))
	if len(client.acked()) != 1 {
		t.Errorf("Skipped must ACK, got %v", client.acked())
	}
}

func TestWorker_HandleMessage_FailedOutcome_DoesNotACK(t *testing.T) {
	client := &fakeStreamClient{}
	proc := &fakeProcessor{out: OutcomeFailed, err: errors.New("transient")}
	w := newTestWorker(client, proc, &fakeNSLister{})

	w.handleMessage(context.Background(), "ns", "catalog:embed:ns", "embedder", validEntry("1-0", 7, "ns"))

	if len(client.acked()) != 0 {
		t.Errorf("Failed must NOT ACK; got %v", client.acked())
	}
	// processor still called
	if proc.calls != 1 {
		t.Errorf("processor calls=%d", proc.calls)
	}
}

func TestWorker_HandleMessage_MalformedEntry_ACKsToDrop(t *testing.T) {
	client := &fakeStreamClient{}
	proc := &fakeProcessor{out: OutcomeEmbedded}
	w := newTestWorker(client, proc, &fakeNSLister{})

	bad := redis.XMessage{ID: "1-0", Values: map[string]any{"namespace": "ns"}} // missing catalog_item_id
	w.handleMessage(context.Background(), "ns", "catalog:embed:ns", "embedder", bad)

	if len(client.acked()) != 1 {
		t.Errorf("malformed entry should be ACKed to drop; got %v", client.acked())
	}
	if proc.calls != 0 {
		t.Errorf("processor must NOT be called on malformed entry, got calls=%d", proc.calls)
	}
}

// --- ensureGroup tests ----------------------------------------------------

func TestWorker_EnsureGroup_BusyGroupTreatedAsSuccess(t *testing.T) {
	client := &fakeStreamClient{groupErr: errors.New("BUSYGROUP Consumer Group name already exists")}
	w := newTestWorker(client, &fakeProcessor{}, &fakeNSLister{})
	if err := w.ensureGroup(context.Background(), "catalog:embed:ns", "embedder"); err != nil {
		t.Errorf("BUSYGROUP should be treated as success, got %v", err)
	}
}

func TestWorker_EnsureGroup_RealErrorPropagates(t *testing.T) {
	client := &fakeStreamClient{groupErr: errors.New("some other failure")}
	w := newTestWorker(client, &fakeProcessor{}, &fakeNSLister{})
	if err := w.ensureGroup(context.Background(), "catalog:embed:ns", "embedder"); err == nil {
		t.Error("expected non-BUSYGROUP error to propagate")
	}
}

// --- refreshNamespaces tests ----------------------------------------------

func TestWorker_RefreshNamespaces_StartsConsumersForEnabled(t *testing.T) {
	client := &fakeStreamClient{}
	// The consume goroutine will block on ctx; we cancel quickly to exit.
	client.readFn = func(ctx context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	lister := &fakeNSLister{cfgs: []*namespace.Config{
		{Namespace: "ns-a", CatalogEnabled: true},
		{Namespace: "ns-b", CatalogEnabled: true},
	}}
	w := newTestWorker(client, &fakeProcessor{}, lister)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.refreshNamespaces(ctx); err != nil {
		t.Fatalf("refreshNamespaces: %v", err)
	}

	w.mu.Lock()
	if len(w.cancels) != 2 {
		t.Errorf("expected 2 namespaces tracked, got %d", len(w.cancels))
	}
	w.mu.Unlock()

	// Cancel triggers all per-namespace contexts to cancel; goroutines drain.
	cancel()
	w.stopAllNamespaces()
	w.wg.Wait()
}

func TestWorker_RefreshNamespaces_StopsDisabledConsumers(t *testing.T) {
	client := &fakeStreamClient{}
	client.readFn = func(ctx context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	lister := &fakeNSLister{cfgs: []*namespace.Config{{Namespace: "ns-a", CatalogEnabled: true}}}
	w := newTestWorker(client, &fakeProcessor{}, lister)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := w.refreshNamespaces(ctx); err != nil {
		t.Fatalf("first refresh: %v", err)
	}
	w.mu.Lock()
	if len(w.cancels) != 1 {
		t.Fatalf("expected 1 namespace, got %d", len(w.cancels))
	}
	w.mu.Unlock()

	// Now ns-a is no longer enabled; consumer should be stopped.
	lister.cfgs = nil
	if err := w.refreshNamespaces(ctx); err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	w.mu.Lock()
	if len(w.cancels) != 0 {
		t.Errorf("expected consumer for ns-a to be stopped, still tracking %v", w.cancels)
	}
	w.mu.Unlock()

	// Cleanup any in-flight goroutines.
	cancel()
	w.stopAllNamespaces()
	w.wg.Wait()
}

func TestWorker_RefreshNamespaces_DoubleStartIsNoOp(t *testing.T) {
	client := &fakeStreamClient{}
	client.readFn = func(ctx context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	lister := &fakeNSLister{cfgs: []*namespace.Config{{Namespace: "ns-a", CatalogEnabled: true}}}
	w := newTestWorker(client, &fakeProcessor{}, lister)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = w.refreshNamespaces(ctx)
	_ = w.refreshNamespaces(ctx) // second call: ns-a already running

	w.mu.Lock()
	if len(w.cancels) != 1 {
		t.Errorf("expected 1 namespace tracked even after duplicate refresh, got %d", len(w.cancels))
	}
	w.mu.Unlock()

	cancel()
	w.stopAllNamespaces()
	w.wg.Wait()
}

func TestWorker_RefreshNamespaces_ListerErrorPropagates(t *testing.T) {
	w := newTestWorker(&fakeStreamClient{}, &fakeProcessor{}, &fakeNSLister{err: errors.New("db down")})
	if err := w.refreshNamespaces(context.Background()); err == nil {
		t.Error("expected error from lister")
	}
}

// --- Run smoke test -------------------------------------------------------

func TestWorker_Run_ShutsDownOnContextCancel(t *testing.T) {
	client := &fakeStreamClient{}
	client.readFn = func(ctx context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	lister := &fakeNSLister{cfgs: []*namespace.Config{{Namespace: "ns-a", CatalogEnabled: true}}}
	w := newTestWorker(client, &fakeProcessor{}, lister)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := w.Run(ctx)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Errorf("expected ctx error from Run, got %v", err)
	}

	// All goroutines must have drained by the time Run returned.
	w.mu.Lock()
	if w.cancels != nil {
		t.Errorf("expected cancels map cleared after shutdown, got %v", w.cancels)
	}
	w.mu.Unlock()
}

// --- consumeStream end-to-end smoke --------------------------------------

func TestWorker_ConsumeStream_DispatchesAndACKs(t *testing.T) {
	processed := make(chan int64, 4)
	proc := &fakeProcessor{processFn: func(_ context.Context, id int64) (ProcessOutcome, error) {
		processed <- id
		return OutcomeEmbedded, nil
	}}

	delivered := false
	client := &fakeStreamClient{}
	client.readFn = func(ctx context.Context, _ *redis.XReadGroupArgs) ([]redis.XStream, error) {
		// Deliver one batch then block until ctx cancel.
		if delivered {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		delivered = true
		return []redis.XStream{{
			Stream:   "catalog:embed:ns",
			Messages: []redis.XMessage{validEntry("1-0", 42, "ns"), validEntry("2-0", 43, "ns")},
		}}, nil
	}

	w := newTestWorker(client, proc, &fakeNSLister{})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() { w.consumeStream(ctx, "ns"); close(done) }()

	gotIDs := []int64{}
	for i := 0; i < 2; i++ {
		select {
		case id := <-processed:
			gotIDs = append(gotIDs, id)
		case <-time.After(time.Second):
			t.Fatal("processor never received entries")
		}
	}

	cancel()
	<-done

	if (gotIDs[0] != 42 || gotIDs[1] != 43) && (gotIDs[0] != 43 || gotIDs[1] != 42) {
		t.Errorf("expected processed ids to include 42 and 43, got %v", gotIDs)
	}
	if len(client.acked()) != 2 {
		t.Errorf("expected 2 ACKs, got %v", client.acked())
	}
}

func TestStreamName(t *testing.T) {
	if got := streamName("foo"); got != "catalog:embed:foo" {
		t.Errorf("streamName(foo) = %q", got)
	}
}
