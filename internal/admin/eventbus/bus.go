package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Event is a single notification published on the bus.
type Event struct {
	Kind      string    // e.g. "batch_run.started", "catalog.dead_letter_grew"
	Namespace string    // optional filter target
	EntityID  string    // optional filter target (batch run id, item id, ...)
	Payload   any       // serialised by SSE handler; bus does not inspect
	At        time.Time // stamped by Publish if zero
}

// Filter narrows which events a subscriber receives. Empty fields match anything.
type Filter struct {
	Kinds     []string // OR-matched; empty = any kind
	Namespace string   // exact match; empty = any namespace
	EntityID  string   // exact match; empty = any entity
}

func (f Filter) matches(e Event) bool {
	if f.Namespace != "" && f.Namespace != e.Namespace {
		return false
	}
	if f.EntityID != "" && f.EntityID != e.EntityID {
		return false
	}
	if len(f.Kinds) == 0 {
		return true
	}
	for _, k := range f.Kinds {
		if k == e.Kind {
			return true
		}
	}
	return false
}

// Bus is the publish/subscribe primitive. Safe for concurrent use.
type Bus struct {
	mu            sync.RWMutex
	subscribers   map[*subscription]struct{}
	bufferSize    int
	onDrop        func(Event)
	onPublish     func(kind string)
	onSubscribe   func()
	onUnsubscribe func()
	closed        atomic.Bool
}

type subscription struct {
	filter Filter
	ch     chan Event
}

// Option configures a Bus.
type Option func(*Bus)

// WithBufferSize sets the per-subscriber channel buffer. Must be > 0; ignored otherwise.
// Default 1024.
func WithBufferSize(n int) Option {
	return func(b *Bus) {
		if n > 0 {
			b.bufferSize = n
		}
	}
}

// WithDropCallback registers a callback invoked when an event is dropped from
// a slow subscriber's buffer. Runs synchronously in the publish path — keep
// it cheap (e.g. increment a Prometheus counter).
func WithDropCallback(fn func(Event)) Option {
	return func(b *Bus) {
		b.onDrop = fn
	}
}

// WithPublishCallback registers a callback invoked once per Publish call,
// after the fan-out completes. Receives the event kind so a Prometheus
// counter can label by kind without rebuilding the event surface in the
// caller. Runs synchronously — keep it cheap.
func WithPublishCallback(fn func(kind string)) Option {
	return func(b *Bus) {
		b.onPublish = fn
	}
}

// WithSubscribeCallback registers a callback invoked every time a subscriber
// attaches. Pair with WithUnsubscribeCallback to maintain a live subscriber
// gauge.
func WithSubscribeCallback(fn func()) Option {
	return func(b *Bus) {
		b.onSubscribe = fn
	}
}

// WithUnsubscribeCallback registers a callback invoked every time a
// subscriber detaches (cancel func called OR Close fans out closures).
func WithUnsubscribeCallback(fn func()) Option {
	return func(b *Bus) {
		b.onUnsubscribe = fn
	}
}

// NewBus constructs a bus ready for Publish/Subscribe.
func NewBus(opts ...Option) *Bus {
	b := &Bus{
		subscribers: make(map[*subscription]struct{}),
		bufferSize:  1024,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Publish fans an event out to every subscriber whose filter matches. Slow
// subscribers drop their oldest event rather than block the publish path; the
// WithDropCallback hook fires once per drop.
//
// Publish holds an RLock for the duration of the fan-out so concurrent
// Subscribe/cancel/Close cannot close a channel while a send is in flight.
// All sends are non-blocking, so the lock window is microseconds even with
// many subscribers.
func (b *Bus) Publish(ctx context.Context, e Event) {
	if b.closed.Load() {
		return
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	b.mu.RLock()
	for s := range b.subscribers {
		if !s.filter.matches(e) {
			continue
		}
		select {
		case s.ch <- e:
		default:
			// Buffer full: drop oldest, then push the new event.
			select {
			case <-s.ch:
			default:
			}
			select {
			case s.ch <- e:
			default:
			}
			if b.onDrop != nil {
				b.onDrop(e)
			}
		}
	}
	b.mu.RUnlock()
	if b.onPublish != nil {
		b.onPublish(e.Kind)
	}
}

// Subscribe returns a channel that receives events matching filter and a
// cancel func that removes the subscription and closes the channel. cancel is
// idempotent.
func (b *Bus) Subscribe(filter Filter) (events <-chan Event, cancel func()) {
	s := &subscription{
		filter: filter,
		ch:     make(chan Event, b.bufferSize),
	}
	b.mu.Lock()
	if b.closed.Load() {
		b.mu.Unlock()
		close(s.ch)
		return s.ch, func() {}
	}
	b.subscribers[s] = struct{}{}
	b.mu.Unlock()
	if b.onSubscribe != nil {
		b.onSubscribe()
	}

	var once sync.Once
	cancel = func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subscribers, s)
			b.mu.Unlock()
			close(s.ch)
			if b.onUnsubscribe != nil {
				b.onUnsubscribe()
			}
		})
	}
	return s.ch, cancel
}

// Close stops the bus and closes every subscriber channel. Subsequent Publish
// and Subscribe calls are no-ops (Subscribe returns an already-closed channel).
func (b *Bus) Close() {
	if !b.closed.CompareAndSwap(false, true) {
		return
	}
	b.mu.Lock()
	count := len(b.subscribers)
	for s := range b.subscribers {
		close(s.ch)
	}
	b.subscribers = nil
	b.mu.Unlock()
	if b.onUnsubscribe != nil {
		for i := 0; i < count; i++ {
			b.onUnsubscribe()
		}
	}
}
