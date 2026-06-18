package eventbus

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishFanOutToAllSubscribers(t *testing.T) {
	b := NewBus()
	defer b.Close()

	const n = 3
	chans := make([]<-chan Event, n)
	cancels := make([]func(), n)
	for i := 0; i < n; i++ {
		chans[i], cancels[i] = b.Subscribe(Filter{})
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	b.Publish(context.Background(), Event{Kind: "test", Payload: "hi"})

	for i, ch := range chans {
		select {
		case e := <-ch:
			if e.Kind != "test" {
				t.Fatalf("sub %d: got kind %q, want test", i, e.Kind)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d: timeout waiting for event", i)
		}
	}
}

func TestPublishStampsAtIfZero(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ch, cancel := b.Subscribe(Filter{})
	defer cancel()

	before := time.Now()
	b.Publish(context.Background(), Event{Kind: "test"})
	after := time.Now()

	select {
	case e := <-ch:
		if e.At.Before(before) || e.At.After(after) {
			t.Fatalf("At=%v not between %v and %v", e.At, before, after)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestPublishPreservesExplicitAt(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ch, cancel := b.Subscribe(Filter{})
	defer cancel()

	want := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	b.Publish(context.Background(), Event{Kind: "test", At: want})

	select {
	case e := <-ch:
		if !e.At.Equal(want) {
			t.Fatalf("At=%v want %v", e.At, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestFilterByNamespace(t *testing.T) {
	b := NewBus()
	defer b.Close()

	prodCh, cancelProd := b.Subscribe(Filter{Namespace: "prod"})
	defer cancelProd()
	stagingCh, cancelStaging := b.Subscribe(Filter{Namespace: "staging"})
	defer cancelStaging()

	b.Publish(context.Background(), Event{Kind: "test", Namespace: "prod"})

	select {
	case e := <-prodCh:
		if e.Namespace != "prod" {
			t.Fatalf("got namespace %q", e.Namespace)
		}
	case <-time.After(time.Second):
		t.Fatal("prod sub timeout")
	}

	select {
	case e := <-stagingCh:
		t.Fatalf("staging sub got unexpected event: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// expected — staging filter must not match prod event
	}
}

func TestFilterByKinds(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ch, cancel := b.Subscribe(Filter{Kinds: []string{"batch_run.started", "batch_run.completed"}})
	defer cancel()

	b.Publish(context.Background(), Event{Kind: "batch_run.started"})
	b.Publish(context.Background(), Event{Kind: "catalog.item_state_changed"})
	b.Publish(context.Background(), Event{Kind: "batch_run.completed"})

	got := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		select {
		case e := <-ch:
			got = append(got, e.Kind)
		case <-time.After(time.Second):
			t.Fatalf("timeout after %d events; got=%v", i, got)
		}
	}
	if got[0] != "batch_run.started" || got[1] != "batch_run.completed" {
		t.Fatalf("got=%v", got)
	}
	select {
	case e := <-ch:
		t.Fatalf("unexpected extra event %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestFilterByEntityID(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ch, cancel := b.Subscribe(Filter{EntityID: "run-42"})
	defer cancel()

	b.Publish(context.Background(), Event{Kind: "x", EntityID: "run-42"})
	b.Publish(context.Background(), Event{Kind: "x", EntityID: "run-43"})

	select {
	case e := <-ch:
		if e.EntityID != "run-42" {
			t.Fatalf("got entity %q", e.EntityID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
	select {
	case e := <-ch:
		t.Fatalf("unexpected extra event %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCancelClosesChannelAndIsIdempotent(t *testing.T) {
	b := NewBus()
	defer b.Close()

	ch, cancel := b.Subscribe(Filter{})
	cancel()
	cancel() // idempotent

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed after cancel")
	}

	// Publish after cancel must not panic.
	b.Publish(context.Background(), Event{Kind: "after_cancel"})
}

func TestCloseIsIdempotent(t *testing.T) {
	b := NewBus()
	b.Close()
	b.Close()
}

func TestPublishAfterCloseIsNoop(t *testing.T) {
	b := NewBus()
	b.Close()
	b.Publish(context.Background(), Event{Kind: "test"}) // must not panic
}

func TestSubscribeAfterCloseReturnsClosedChannel(t *testing.T) {
	b := NewBus()
	b.Close()

	ch, cancel := b.Subscribe(Filter{})
	defer cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected closed channel")
		}
	case <-time.After(time.Second):
		t.Fatal("channel not closed")
	}
}

func TestDropOnSlowSubscriberFiresCallback(t *testing.T) {
	var dropped atomic.Int64
	b := NewBus(
		WithBufferSize(2),
		WithDropCallback(func(Event) { dropped.Add(1) }),
	)
	defer b.Close()

	ch, cancel := b.Subscribe(Filter{})
	defer cancel()

	// Publish 5 without draining; buffer is 2, so at least 3 must drop.
	for i := 0; i < 5; i++ {
		b.Publish(context.Background(), Event{Kind: "x"})
	}

	if got := dropped.Load(); got < 3 {
		t.Fatalf("dropped=%d, want >= 3", got)
	}
	// Drain to confirm channel still functions.
	deadline := time.After(200 * time.Millisecond)
	drained := 0
loop:
	for {
		select {
		case <-ch:
			drained++
		case <-deadline:
			break loop
		}
	}
	if drained == 0 {
		t.Fatal("channel had no readable events after drops")
	}
}

func TestPublishCallbackFiresPerEvent(t *testing.T) {
	var publishedKinds []string
	var mu sync.Mutex
	b := NewBus(WithPublishCallback(func(kind string) {
		mu.Lock()
		defer mu.Unlock()
		publishedKinds = append(publishedKinds, kind)
	}))
	defer b.Close()

	b.Publish(context.Background(), Event{Kind: "a"})
	b.Publish(context.Background(), Event{Kind: "b"})
	b.Publish(context.Background(), Event{Kind: "a"})

	mu.Lock()
	defer mu.Unlock()
	if got, want := publishedKinds, []string{"a", "b", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("publishedKinds = %v, want %v", got, want)
	}
}

func TestPublishAfterCloseDoesNotFirePublishCallback(t *testing.T) {
	var fires atomic.Int64
	b := NewBus(WithPublishCallback(func(string) { fires.Add(1) }))
	b.Close()
	b.Publish(context.Background(), Event{Kind: "x"})
	if got := fires.Load(); got != 0 {
		t.Fatalf("fires=%d, want 0 after Close", got)
	}
}

func TestSubscribeAndUnsubscribeCallbacksTrackGauge(t *testing.T) {
	var gauge atomic.Int64
	b := NewBus(
		WithSubscribeCallback(func() { gauge.Add(1) }),
		WithUnsubscribeCallback(func() { gauge.Add(-1) }),
	)
	defer b.Close()

	_, cancel1 := b.Subscribe(Filter{})
	_, cancel2 := b.Subscribe(Filter{})
	if got := gauge.Load(); got != 2 {
		t.Fatalf("after 2 subscribes gauge=%d, want 2", got)
	}

	cancel1()
	cancel1() // idempotent — must not double-decrement
	if got := gauge.Load(); got != 1 {
		t.Fatalf("after cancel1 (twice) gauge=%d, want 1", got)
	}

	cancel2()
	if got := gauge.Load(); got != 0 {
		t.Fatalf("after cancel2 gauge=%d, want 0", got)
	}
}

func TestCloseFiresUnsubscribeForEverySubscriber(t *testing.T) {
	var unsubs atomic.Int64
	b := NewBus(WithUnsubscribeCallback(func() { unsubs.Add(1) }))

	const n = 5
	for i := 0; i < n; i++ {
		_, _ = b.Subscribe(Filter{})
	}
	b.Close()

	if got := unsubs.Load(); got != n {
		t.Fatalf("unsubs=%d, want %d", got, n)
	}
}

// TestConcurrentFanOut covers the BUILD_PLAN §8 requirement: 100 subscribers,
// 10 000 events, no drops with adequate buffer, events ordered per topic.
func TestConcurrentFanOut(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	const (
		nSubs   = 100
		nEvents = 10_000
	)
	b := NewBus(WithBufferSize(16_384))
	defer b.Close()

	var wg sync.WaitGroup
	counts := make([]int64, nSubs)
	orderErrors := make([]int64, nSubs)

	for i := 0; i < nSubs; i++ {
		ch, cancel := b.Subscribe(Filter{})
		wg.Add(1)
		go func(idx int, ch <-chan Event, cancel func()) {
			defer wg.Done()
			defer cancel()
			var last int
			for e := range ch {
				seq, _ := e.Payload.(int)
				if counts[idx] > 0 && seq != last+1 {
					orderErrors[idx]++
				}
				last = seq
				counts[idx]++
				if counts[idx] >= nEvents {
					return
				}
			}
		}(i, ch, cancel)
	}

	// Allow subscribers to register before publishing.
	time.Sleep(50 * time.Millisecond)

	go func() {
		for i := 0; i < nEvents; i++ {
			b.Publish(context.Background(), Event{Kind: "load", Payload: i + 1})
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("subscribers did not drain in 15s")
	}

	for i := 0; i < nSubs; i++ {
		if counts[i] != nEvents {
			t.Errorf("sub %d: got %d events, want %d", i, counts[i], nEvents)
		}
		if orderErrors[i] != 0 {
			t.Errorf("sub %d: %d ordering violations", i, orderErrors[i])
		}
	}
}
