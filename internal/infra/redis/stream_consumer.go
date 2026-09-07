package redis

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const initialReclaimCursor = "0-0"

// ReclaimOutcome describes why a bounded pending-entry scan stopped.
type ReclaimOutcome string

const (
	// ReclaimTerminal means Redis reported that the scan wrapped.
	ReclaimTerminal ReclaimOutcome = "terminal"
	// ReclaimNoGroup means the group vanished and invalidated its cursor.
	ReclaimNoGroup ReclaimOutcome = "nogroup"
	// ReclaimError means the scan stopped on a transient transport failure.
	ReclaimError ReclaimOutcome = "error"
	// ReclaimBudgetExhausted means the scan reached its per-tick page cap.
	ReclaimBudgetExhausted ReclaimOutcome = "budget_exhausted"
)

// StreamConsumerConfig contains transport mechanics shared by stream workers.
// Decode, processing, ACK policy, lifecycle checks, and metric labels remain in
// the domain adapter supplied through Handle and Observe.
type StreamConsumerConfig struct {
	Stream         string
	Group          string
	Consumer       string
	ReadCount      int64
	ReadBlock      time.Duration
	ReapInterval   time.Duration
	MinIdleReap    time.Duration
	ReapCount      int64
	ReapPageBudget int
	ReadBackoffMin time.Duration
	ReadBackoffMax time.Duration
}

// StreamConsumerFuncs is the Redis transport seam. The value-returning shape
// keeps the state machine independent of go-redis command types in tests.
type StreamConsumerFuncs struct {
	CreateGroup func(context.Context, string, string, string) error
	ReadGroup   func(context.Context, *goredis.XReadGroupArgs) ([]goredis.XStream, error)
	AutoClaim   func(context.Context, *goredis.XAutoClaimArgs) ([]goredis.XMessage, string, error)
}

// StreamConsumerObserver lets domain adapters attach their own logs and metric
// labels without teaching the transport state machine about domain names.
type StreamConsumerObserver struct {
	ReadError        func(context.Context, error)
	RecreateError    func(context.Context, error)
	ReclaimError     func(context.Context, error)
	Reclaimed        func(int)
	ReclaimCompleted func(ReclaimOutcome)
}

// StreamConsumer centralizes the consume/reclaim state machine used by event,
// catalog-ingest, and embedder workers.
type StreamConsumer struct {
	cfg     StreamConsumerConfig
	fns     StreamConsumerFuncs
	handle  func(context.Context, goredis.XMessage)
	observe StreamConsumerObserver
}

// NewStreamConsumer builds a consumer with explicit domain callbacks.
func NewStreamConsumer(
	cfg StreamConsumerConfig,
	fns StreamConsumerFuncs,
	handle func(context.Context, goredis.XMessage),
	observe StreamConsumerObserver,
) *StreamConsumer {
	return &StreamConsumer{cfg: cfg, fns: fns, handle: handle, observe: observe}
}

// Init creates the configured group and treats BUSYGROUP as success.
func (c *StreamConsumer) Init(ctx context.Context) error {
	err := c.fns.CreateGroup(ctx, c.cfg.Stream, c.cfg.Group, "0")
	if err != nil && !IsBusyGroupError(err) {
		return err
	}
	return nil
}

// Run consumes new entries and concurrently reclaims abandoned pending
// entries until ctx ends. All goroutines are joined before it returns.
func (c *StreamConsumer) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.reapLoop(ctx)
	}()
	c.consumeLoop(ctx)
	wg.Wait()
}

func (c *StreamConsumer) consumeLoop(ctx context.Context) {
	backoff := c.cfg.ReadBackoffMin
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := c.fns.ReadGroup(ctx, &goredis.XReadGroupArgs{
			Group: c.cfg.Group, Consumer: c.cfg.Consumer,
			Streams: []string{c.cfg.Stream, ">"}, Count: c.cfg.ReadCount, Block: c.cfg.ReadBlock,
		})
		if errors.Is(err, goredis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			if IsNoGroupError(err) {
				if createErr := c.Init(ctx); createErr != nil && c.observe.RecreateError != nil {
					c.observe.RecreateError(ctx, createErr)
				}
			}
			if c.observe.ReadError != nil {
				c.observe.ReadError(ctx, err)
			}
			if !sleepContext(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, c.cfg.ReadBackoffMax)
			continue
		}
		backoff = c.cfg.ReadBackoffMin

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				c.handle(ctx, msg)
			}
		}
	}
}

func (c *StreamConsumer) reapLoop(ctx context.Context) {
	cursor := initialReclaimCursor
	ticker := time.NewTicker(c.cfg.ReapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cursor = c.ReapOnce(ctx, cursor)
		}
	}
}

// ReapOnce scans at most ReapPageBudget pages and returns the cursor for the
// next tick. Empty pages advance; transient failures retain the cursor; a
// terminal cursor or NOGROUP resets it.
func (c *StreamConsumer) ReapOnce(ctx context.Context, cursor string) string {
	if cursor == "" {
		cursor = initialReclaimCursor
	}
	for page := 0; page < c.cfg.ReapPageBudget; page++ {
		msgs, next, err := c.fns.AutoClaim(ctx, &goredis.XAutoClaimArgs{
			Stream: c.cfg.Stream, Group: c.cfg.Group, Consumer: c.cfg.Consumer,
			MinIdle: c.cfg.MinIdleReap, Start: cursor, Count: c.cfg.ReapCount,
		})
		if err != nil {
			if IsNoGroupError(err) {
				c.completed(ReclaimNoGroup)
				return initialReclaimCursor
			}
			if ctx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) && c.observe.ReclaimError != nil {
				c.observe.ReclaimError(ctx, err)
			}
			c.completed(ReclaimError)
			return cursor
		}
		if len(msgs) > 0 && c.observe.Reclaimed != nil {
			c.observe.Reclaimed(len(msgs))
		}
		for _, msg := range msgs {
			c.handle(ctx, msg)
		}
		cursor = next
		if next == initialReclaimCursor || next == "" {
			c.completed(ReclaimTerminal)
			return initialReclaimCursor
		}
	}
	c.completed(ReclaimBudgetExhausted)
	return cursor
}

func (c *StreamConsumer) completed(outcome ReclaimOutcome) {
	if c.observe.ReclaimCompleted != nil {
		c.observe.ReclaimCompleted(outcome)
	}
}

// IsBusyGroupError reports Redis' idempotent group-create response.
func IsBusyGroupError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BUSYGROUP")
}

// IsNoGroupError reports that a stream consumer group no longer exists.
func IsNoGroupError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "NOGROUP")
}

func sleepContext(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
