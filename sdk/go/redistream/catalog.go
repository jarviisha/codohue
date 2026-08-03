package redistream

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// CatalogProducer publishes catalog content to the durable catalog stream,
// symmetric with Producer for behavioural events. Content published while
// Codohue is unreachable sits in the stream until the ingest worker consumes
// it — no producer retry, no repair pass.
type CatalogProducer struct {
	rdb    XAdder
	stream string
}

// CatalogOption configures a CatalogProducer.
type CatalogOption func(*CatalogProducer)

// WithCatalogStream overrides the stream name. Defaults to
// codohuetypes.CatalogStreamName.
func WithCatalogStream(name string) CatalogOption {
	return func(p *CatalogProducer) {
		if name != "" {
			p.stream = name
		}
	}
}

// NewCatalogProducer returns a CatalogProducer publishing to the default
// Codohue catalog stream using rdb.
func NewCatalogProducer(rdb XAdder, opts ...CatalogOption) *CatalogProducer {
	p := &CatalogProducer{
		rdb:    rdb,
		stream: codohuetypes.CatalogStreamName,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Publish XADDs a single catalog item and returns the assigned stream ID.
func (p *CatalogProducer) Publish(ctx context.Context, item codohuetypes.CatalogStreamItem) (string, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("codohue/redistream: marshal catalog item: %w", err)
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

// PublishBatch publishes items sequentially and returns the IDs published so
// far; on failure callers can resume from the last successful item.
func (p *CatalogProducer) PublishBatch(ctx context.Context, items []codohuetypes.CatalogStreamItem) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(items))
	for i := range items {
		id, err := p.Publish(ctx, items[i])
		if err != nil {
			return ids, fmt.Errorf("publish catalog item %d: %w", i, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
