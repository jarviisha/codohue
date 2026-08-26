package redis

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type fakeRetentionBackend struct {
	groups     []retentionGroup
	groupsErr  error
	pending    map[string]retentionPending
	pendingErr map[string]error
	length     int64
	lengthErr  error
	trimmed    int64
	trimErr    error
	trimCalls  []string
}

func (f *fakeRetentionBackend) Groups(context.Context, string) ([]retentionGroup, error) {
	return f.groups, f.groupsErr
}

func (f *fakeRetentionBackend) Pending(_ context.Context, _, group string) (retentionPending, error) {
	return f.pending[group], f.pendingErr[group]
}

func (f *fakeRetentionBackend) Length(context.Context, string) (int64, error) {
	return f.length, f.lengthErr
}

func (f *fakeRetentionBackend) TrimMinID(_ context.Context, _, frontier string) (int64, error) {
	f.trimCalls = append(f.trimCalls, frontier)
	return f.trimmed, f.trimErr
}

func TestRetentionUsesMinimumMultiGroupSafeFrontier(t *testing.T) {
	backend := &fakeRetentionBackend{
		groups: []retentionGroup{
			{Name: "primary", Pending: 0, LastDeliveredID: "50-0", Lag: 3},
			{Name: "analytics", Pending: 2, LastDeliveredID: "70-0", Lag: 1},
			{Name: "unexpected", Pending: 0, LastDeliveredID: "40-0"},
		},
		pending: map[string]retentionPending{"analytics": {Count: 2, OldestID: "45-0"}},
		length:  100,
		trimmed: 39,
	}
	r := newRetentionWithBackend(backend, false)

	result, err := r.RunOnce(context.Background(), StreamSpec{
		Name: "codohue:events", Kind: "events", ExpectedGroups: []string{"primary", "analytics"},
	})
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.SafeFrontier != "40-0" {
		t.Fatalf("SafeFrontier = %q, want 40-0", result.SafeFrontier)
	}
	if result.Pending != 2 || result.Undelivered != 3 || result.UnexpectedGroups != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(backend.trimCalls) != 1 || backend.trimCalls[0] != "40-0" {
		t.Fatalf("exact trim calls = %v, want [40-0]", backend.trimCalls)
	}
	if result.Trimmed != 39 || result.Length != 100 || result.DryRun {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRetentionDryRunComputesWithoutTrimming(t *testing.T) {
	backend := &fakeRetentionBackend{
		groups: []retentionGroup{{Name: "primary", LastDeliveredID: "9-1"}},
		length: 10,
	}
	r := newRetentionWithBackend(backend, true)
	result, err := r.RunOnce(context.Background(), StreamSpec{
		Name: "codohue:catalog", Kind: "catalog", ExpectedGroups: []string{"primary"},
	})
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if !result.DryRun || result.SafeFrontier != "9-1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(backend.trimCalls) != 0 {
		t.Fatalf("dry run trimmed at %v", backend.trimCalls)
	}
}

func TestRetentionFailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		backend *fakeRetentionBackend
		stage   string
	}{
		{name: "group inspection", backend: &fakeRetentionBackend{groupsErr: errors.New("redis down")}, stage: "groups"},
		{name: "no groups", backend: &fakeRetentionBackend{}, stage: "groups"},
		{name: "missing expected group", backend: &fakeRetentionBackend{groups: []retentionGroup{{Name: "other", LastDeliveredID: "1-0"}}}, stage: "groups"},
		{name: "pending inspection", backend: &fakeRetentionBackend{
			groups:     []retentionGroup{{Name: "primary", Pending: 1, LastDeliveredID: "2-0"}},
			pendingErr: map[string]error{"primary": errors.New("pel unavailable")},
		}, stage: "pel"},
		{name: "contradictory pending", backend: &fakeRetentionBackend{
			groups:  []retentionGroup{{Name: "primary", Pending: 1, LastDeliveredID: "2-0"}},
			pending: map[string]retentionPending{"primary": {}},
		}, stage: "frontier"},
		{name: "malformed frontier", backend: &fakeRetentionBackend{
			groups: []retentionGroup{{Name: "primary", LastDeliveredID: "bad"}},
		}, stage: "frontier"},
		{name: "length", backend: &fakeRetentionBackend{
			groups: []retentionGroup{{Name: "primary", LastDeliveredID: "1-0"}}, lengthErr: errors.New("xlen failed"),
		}, stage: "length"},
		{name: "trim", backend: &fakeRetentionBackend{
			groups: []retentionGroup{{Name: "primary", LastDeliveredID: "1-0"}}, trimErr: errors.New("trim failed"),
		}, stage: "trim"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRetentionWithBackend(tt.backend, false)
			_, err := r.RunOnce(context.Background(), StreamSpec{
				Name: "stream", Kind: "events", ExpectedGroups: []string{"primary"},
			})
			var retentionErr *RetentionError
			if !errors.As(err, &retentionErr) || retentionErr.Stage != tt.stage {
				t.Fatalf("error = %v, want RetentionError stage %q", err, tt.stage)
			}
			if tt.stage != "trim" && len(tt.backend.trimCalls) != 0 {
				t.Fatalf("fail-closed path attempted trim: %v", tt.backend.trimCalls)
			}
		})
	}
}

func TestCompareStreamIDs(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1-0", "2-0", -1},
		{"2-0", "2-0", 0},
		{"2-1", "2-0", 1},
		{"10-0", "2-99", 1},
	}
	for _, tt := range tests {
		got, err := compareStreamIDs(tt.a, tt.b)
		if err != nil {
			t.Fatalf("compare %q %q: %v", tt.a, tt.b, err)
		}
		if got != tt.want {
			t.Errorf("compare %q %q = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
	if _, err := compareStreamIDs("bad", "1-0"); err == nil {
		t.Fatal("expected malformed ID error")
	}
}

// The go-redis adapter. These are the translation layer between go-redis
// types and this package's own, exercised through the same function seams the
// rest of the package uses so no live server is needed.

func TestRedisBackendGroups_TranslatesEveryField(t *testing.T) {
	original := xInfoGroupsFn
	t.Cleanup(func() { xInfoGroupsFn = original })
	xInfoGroupsFn = func(_ context.Context, _ *goredis.Client, stream string) ([]goredis.XInfoGroup, error) {
		if stream != "codohue:events" {
			t.Errorf("stream = %q, want codohue:events", stream)
		}
		return []goredis.XInfoGroup{
			{Name: "ingest", Pending: 3, LastDeliveredID: "5-0", Lag: 7},
		}, nil
	}

	groups, err := (redisRetentionBackend{}).Groups(context.Background(), "codohue:events")
	if err != nil {
		t.Fatalf("Groups: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("got %d group(s), want 1", len(groups))
	}
	// Every field feeds the frontier decision: a dropped Pending would let the
	// trim run past a group's unacked entries.
	got := groups[0]
	if got.Name != "ingest" || got.Pending != 3 || got.LastDeliveredID != "5-0" || got.Lag != 7 {
		t.Errorf("group = %+v, want all four fields carried through", got)
	}
}

func TestRedisBackendGroups_ErrorNamesTheStream(t *testing.T) {
	original := xInfoGroupsFn
	t.Cleanup(func() { xInfoGroupsFn = original })
	xInfoGroupsFn = func(context.Context, *goredis.Client, string) ([]goredis.XInfoGroup, error) {
		return nil, errors.New("connection refused")
	}

	_, err := (redisRetentionBackend{}).Groups(context.Background(), "codohue:events")
	if err == nil || !strings.Contains(err.Error(), "codohue:events") {
		t.Fatalf("error = %v, want it to name the stream", err)
	}
}

func TestRedisBackendPending_CarriesTheOldestID(t *testing.T) {
	original := xPendingFn
	t.Cleanup(func() { xPendingFn = original })
	xPendingFn = func(_ context.Context, _ *goredis.Client, _, group string) (*goredis.XPending, error) {
		if group != "ingest" {
			t.Errorf("group = %q, want ingest", group)
		}
		return &goredis.XPending{Count: 4, Lower: "9-1", Higher: "20-0"}, nil
	}

	pending, err := (redisRetentionBackend{}).Pending(context.Background(), "s", "ingest")
	if err != nil {
		t.Fatalf("Pending: %v", err)
	}
	// Lower, not Higher: the oldest pending entry is the frontier, because
	// everything at or after it is still unacked by this group.
	if pending.Count != 4 || pending.OldestID != "9-1" {
		t.Errorf("pending = %+v, want count 4 and oldest 9-1", pending)
	}
}

func TestRedisBackendPending_Error(t *testing.T) {
	original := xPendingFn
	t.Cleanup(func() { xPendingFn = original })
	xPendingFn = func(context.Context, *goredis.Client, string, string) (*goredis.XPending, error) {
		return nil, errors.New("connection refused")
	}

	if _, err := (redisRetentionBackend{}).Pending(context.Background(), "s", "g"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRedisBackendLength(t *testing.T) {
	original := xLenFn
	t.Cleanup(func() { xLenFn = original })
	xLenFn = func(context.Context, *goredis.Client, string) (int64, error) { return 12, nil }

	n, err := (redisRetentionBackend{}).Length(context.Background(), "s")
	if err != nil || n != 12 {
		t.Fatalf("got (%d, %v), want (12, nil)", n, err)
	}

	xLenFn = func(context.Context, *goredis.Client, string) (int64, error) {
		return 0, errors.New("connection refused")
	}
	if _, err := (redisRetentionBackend{}).Length(context.Background(), "s"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRedisBackendTrimMinID(t *testing.T) {
	original := xTrimMinIDFn
	t.Cleanup(func() { xTrimMinIDFn = original })
	var gotFrontier string
	xTrimMinIDFn = func(_ context.Context, _ *goredis.Client, _, frontier string) (int64, error) {
		gotFrontier = frontier
		return 5, nil
	}

	trimmed, err := (redisRetentionBackend{}).TrimMinID(context.Background(), "s", "9-1")
	if err != nil || trimmed != 5 {
		t.Fatalf("got (%d, %v), want (5, nil)", trimmed, err)
	}
	if gotFrontier != "9-1" {
		t.Errorf("frontier = %q, want the value passed through unchanged", gotFrontier)
	}

	xTrimMinIDFn = func(context.Context, *goredis.Client, string, string) (int64, error) {
		return 0, errors.New("connection refused")
	}
	if _, err := (redisRetentionBackend{}).TrimMinID(context.Background(), "s", "9-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewRetention_WiresTheRedisBackend(t *testing.T) {
	r := NewRetention(nil, true)
	if r == nil || !r.dryRun {
		t.Fatal("dry-run flag was not carried through")
	}
	if _, ok := r.backend.(redisRetentionBackend); !ok {
		t.Errorf("backend = %T, want redisRetentionBackend", r.backend)
	}
}

// The stage is what tells an operator which fail-closed check refused, and the
// wrapped error must stay reachable through errors.Is.
func TestRetentionError_NamesTheStageAndUnwraps(t *testing.T) {
	cause := errors.New("connection refused")
	err := &RetentionError{Stage: "frontier", Err: cause}

	if !strings.Contains(err.Error(), "frontier") {
		t.Errorf("message %q does not name the stage", err)
	}
	if !errors.Is(err, cause) {
		t.Error("the wrapped cause is not reachable through errors.Is")
	}
}

// --- RunRetentionLoop ----------------------------------------------------

type countingRunner struct {
	mu    sync.Mutex
	specs []string
	err   error
}

func (r *countingRunner) RunOnce(_ context.Context, spec StreamSpec) (RetentionResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.specs = append(r.specs, spec.Name)
	return RetentionResult{}, r.err
}

func (r *countingRunner) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.specs...)
}

// The first pass runs immediately rather than waiting a full interval, so a
// restart does not leave streams untrimmed for the whole tick.
func TestRunRetentionLoop_RunsImmediatelyThenStopsOnCancel(t *testing.T) {
	runner := &countingRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	provider := func(context.Context) ([]StreamSpec, error) {
		return []StreamSpec{{Name: "codohue:events"}}, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		RunRetentionLoop(ctx, time.Hour, runner, provider) // an interval that never fires
	}()

	waitFor(t, func() bool { return len(runner.seen()) >= 1 })
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunRetentionLoop did not return after its context was cancelled")
	}
	if got := runner.seen(); len(got) != 1 || got[0] != "codohue:events" {
		t.Errorf("passes = %v, want exactly one immediate pass", got)
	}
}

// A discovery failure must not stop the loop: the next tick retries rather
// than leaving retention dead until the process restarts.
func TestRunRetentionLoop_ProviderFailureIsRetried(t *testing.T) {
	var calls atomic.Int64
	runner := &countingRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := func(context.Context) ([]StreamSpec, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("namespace list unavailable")
		}
		return []StreamSpec{{Name: "codohue:catalog"}}, nil
	}

	go RunRetentionLoop(ctx, time.Millisecond, runner, provider)

	waitFor(t, func() bool { return len(runner.seen()) >= 1 })
	if got := runner.seen()[0]; got != "codohue:catalog" {
		t.Errorf("first successful pass = %q, want codohue:catalog", got)
	}
}

// A stream that fails must not stop the pass: the remaining streams are still
// this tick's work.
func TestRunRetentionLoop_OneStreamFailureDoesNotSkipTheRest(t *testing.T) {
	runner := &countingRunner{err: errors.New("xinfo failed")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	provider := func(context.Context) ([]StreamSpec, error) {
		return []StreamSpec{{Name: "a"}, {Name: "b"}}, nil
	}

	go RunRetentionLoop(ctx, time.Hour, runner, provider)

	waitFor(t, func() bool { return len(runner.seen()) >= 2 })
	if got := runner.seen(); len(got) < 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("passes = %v, want both streams attempted", got)
	}
}

// A non-positive interval would make time.NewTicker panic and take the whole
// process with it, so the loop substitutes a default.
func TestRunRetentionLoop_NonPositiveIntervalDoesNotPanic(t *testing.T) {
	runner := &countingRunner{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		RunRetentionLoop(ctx, 0, runner, func(context.Context) ([]StreamSpec, error) {
			return []StreamSpec{{Name: "a"}}, nil
		})
	}()

	waitFor(t, func() bool { return len(runner.seen()) >= 1 })
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not return")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met within the deadline")
}

// Both halves of a stream id are required: a bare "12345" or a non-numeric
// sequence is not an entry id, and treating one as zero would produce a
// frontier that trims live entries.
func TestParseStreamID_RejectsMalformedIDs(t *testing.T) {
	for _, id := range []string{"", "12345", "-1", "5-", "abc-1", "5-abc"} {
		if _, _, err := parseStreamID(id); err == nil {
			t.Errorf("parseStreamID(%q) accepted a malformed id", id)
		}
	}
}

func TestCompareStreamIDs_MalformedOperandIsAnError(t *testing.T) {
	if _, err := compareStreamIDs("5-0", "not-an-id"); err == nil {
		t.Error("compareStreamIDs accepted a malformed second operand")
	}
	if _, err := compareStreamIDs("not-an-id", "5-0"); err == nil {
		t.Error("compareStreamIDs accepted a malformed first operand")
	}
}
