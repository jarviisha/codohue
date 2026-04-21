package redistream

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// XAdder is the minimal surface of a Redis client needed to publish stream
// entries. *redis.Client satisfies this interface. Exposing it as an interface
// keeps the producer easy to fake in tests.
type XAdder interface {
	XAdd(ctx context.Context, a *redis.XAddArgs) *redis.StringCmd
}

// Producer publishes Codohue behavioral events to a Redis stream. The default
// stream name matches what the Codohue ingest worker consumes; override only
// if you are running a custom deployment.
type Producer struct {
	rdb    XAdder
	stream string
}

// Option configures a Producer.
type Option func(*Producer)

// WithStream overrides the stream name. Defaults to codohuetypes.StreamName.
func WithStream(name string) Option {
	return func(p *Producer) {
		if name != "" {
			p.stream = name
		}
	}
}

// NewProducer returns a Producer that publishes to the default Codohue ingest
// stream using rdb. Pass a *redis.Client or any other XAdder implementation.
func NewProducer(rdb XAdder, opts ...Option) *Producer {
	p := &Producer{
		rdb:    rdb,
		stream: codohuetypes.StreamName,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish XADDs a single event and returns the assigned Redis stream ID.
func (p *Producer) Publish(ctx context.Context, event codohuetypes.EventPayload) (string, error) {
	raw, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("codohue/redistream: marshal event: %w", err)
	}
	id, err := p.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: map[string]any{codohuetypes.PayloadField: string(raw)},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("codohue/redistream: xadd: %w", err)
	}
	return id, nil
}

// PublishBatch publishes a slice of events sequentially and returns the IDs of
// events that were successfully published. If one XADD fails, PublishBatch
// returns the IDs accumulated so far along with the error — callers can use
// this to resume from the last successfully published event.
func (p *Producer) PublishBatch(ctx context.Context, events []codohuetypes.EventPayload) ([]string, error) {
	if len(events) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(events))
	for i := range events {
		id, err := p.Publish(ctx, events[i])
		if err != nil {
			return ids, fmt.Errorf("publish event %d: %w", i, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
