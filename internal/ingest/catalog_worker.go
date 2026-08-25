package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

const catalogConsumerGroup = "codohue-catalog-ingest"

// ErrCatalogItemRejected marks a stream-delivered catalog item as permanently
// unprocessable: it failed the same validation the HTTP ingest path applies
// (empty content, over the size cap, namespace missing or not in catalog
// mode). The worker acks such entries off the stream and records the
// rejection; any other error is treated as transient and left pending for
// redelivery. The cmd/api adapter wraps catalog-domain validation errors with
// this sentinel because the import rule keeps this package blind to them.
var ErrCatalogItemRejected = errors.New("ingest: catalog item rejected")

// catalogIngestor is the seam to the catalog domain. Satisfied by an adapter
// in cmd/api around catalog.Service — internal/ingest must not import the
// peer domain directly.
type catalogIngestor interface {
	IngestStreamItem(ctx context.Context, item *codohuetypes.CatalogStreamItem) error
}

// CatalogWorker consumes catalog content from the durable client-facing
// stream (codohuetypes.CatalogStreamName) and hands it to the catalog domain,
// which persists catalog_items and feeds the embedder pipeline exactly as the
// HTTP path does. Delivery is at-least-once: entries are acked only after the
// row is durable (or the item is permanently rejected), and redelivery is
// idempotent — re-ingesting unchanged content is a no-op upsert behind the
// content-hash short-circuit.
type CatalogWorker struct {
	service       catalogIngestor
	lifecycle     lifecycleEvaluator
	consumer      string
	createGroupFn func(ctx context.Context, stream, group, start string) error
	readGroupFn   func(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XStream, error)
	autoClaimFn   func(ctx context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error)
	ackFn         func(ctx context.Context, stream, group string, ids ...string) error
	reapCursor    string
}

// SetLifecycleEvaluator enables durable generation enforcement.
func (w *CatalogWorker) SetLifecycleEvaluator(evaluator lifecycleEvaluator) { w.lifecycle = evaluator }

// NewCatalogWorker creates a CatalogWorker consuming as the given consumer
// name (empty falls back to the same default as the event worker).
func NewCatalogWorker(redisClient *redis.Client, service catalogIngestor, consumer string) *CatalogWorker {
	if consumer == "" {
		consumer = defaultConsumerName
	}
	return &CatalogWorker{
		service:    service,
		consumer:   consumer,
		reapCursor: "0-0",
		createGroupFn: func(ctx context.Context, stream, group, start string) error {
			return redisClient.XGroupCreateMkStream(ctx, stream, group, start).Err()
		},
		readGroupFn: func(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XStream, error) {
			return redisClient.XReadGroup(ctx, args).Result()
		},
		autoClaimFn: func(ctx context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error) {
			msgs, next, err := redisClient.XAutoClaim(ctx, args).Result()
			if err != nil {
				return nil, "", fmt.Errorf("xautoclaim: %w", err)
			}
			return msgs, next, nil
		},
		ackFn: func(ctx context.Context, stream, group string, ids ...string) error {
			return redisClient.XAck(ctx, stream, group, ids...).Err()
		},
	}
}

// Init creates the consumer group if it does not already exist.
func (w *CatalogWorker) Init(ctx context.Context) error {
	err := w.createGroupFn(ctx, codohuetypes.CatalogStreamName, catalogConsumerGroup, "0")
	if err != nil && !isBusyGroupErr(err) {
		return fmt.Errorf("create catalog consumer group: %w", err)
	}
	return nil
}

// Run consumes the catalog stream until ctx is cancelled, with the same
// reap/backoff mechanics as the event worker.
func (w *CatalogWorker) Run(ctx context.Context) {
	slog.Info("catalog ingest worker started", "stream", codohuetypes.CatalogStreamName, "consumer", w.consumer)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.reapPending(ctx)
	}()
	defer func() {
		wg.Wait()
		slog.Info("catalog ingest worker stopped")
	}()

	backoff := readErrBackoffMin
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := w.readGroupFn(ctx, &redis.XReadGroupArgs{
			Group:    catalogConsumerGroup,
			Consumer: w.consumer,
			Streams:  []string{codohuetypes.CatalogStreamName, ">"},
			Count:    10,
			Block:    5 * time.Second,
		})
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if isNoGroupErr(err) {
				if createErr := w.createGroupFn(ctx, codohuetypes.CatalogStreamName, catalogConsumerGroup, "0"); createErr != nil && !isBusyGroupErr(createErr) {
					slog.Warn("catalog ingest recreate consumer group failed", "error", createErr)
				}
			}
			slog.Warn("catalog ingest xreadgroup failed", "error", err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, readErrBackoffMax)
			continue
		}
		backoff = readErrBackoffMin

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				w.handleMessage(ctx, msg)
			}
		}
	}
}

func (w *CatalogWorker) reapPending(ctx context.Context) {
	ticker := time.NewTicker(reapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		w.reapOnce(ctx)
	}
}

func (w *CatalogWorker) reapOnce(ctx context.Context) {
	if w.reapCursor == "" {
		w.reapCursor = "0-0"
	}
	for page := 0; page < reapPageBudget; page++ {
		start := w.reapCursor
		msgs, next, err := w.autoClaimFn(ctx, &redis.XAutoClaimArgs{
			Stream: codohuetypes.CatalogStreamName, Group: catalogConsumerGroup, Consumer: w.consumer,
			MinIdle: minIdleReap, Start: start, Count: reapBatchSize,
		})
		if err != nil {
			if isNoGroupErr(err) {
				w.reapCursor = "0-0"
			} else if ctx.Err() == nil {
				slog.Warn("catalog ingest xautoclaim failed", "error", err)
			}
			return
		}
		for _, msg := range msgs {
			w.handleMessage(ctx, msg)
		}
		w.reapCursor = next
		if next == "0-0" || next == "" {
			w.reapCursor = "0-0"
			return
		}
	}
}

// handleMessage decodes and ingests one stream entry. Permanent rejections
// are acked and recorded (log + metric — the observable path for items that
// never reach a catalog_items row); transient failures leave the entry
// pending for the reaper.
func (w *CatalogWorker) handleMessage(ctx context.Context, msg redis.XMessage) {
	item, err := decodeCatalogMessage(msg)
	if err != nil {
		slog.Warn("catalog ingest dropping malformed stream entry", "entry_id", msg.ID, "error", err)
		metrics.CatalogStreamRejectsTotal.WithLabelValues("", "malformed").Inc()
		w.ack(ctx, msg.ID)
		return
	}
	if w.lifecycle != nil {
		var generation *int64
		if item.NamespaceGeneration > 0 {
			generation = &item.NamespaceGeneration
		}
		disposition, lifecycleErr := w.lifecycle.EvaluateEnvelope(ctx, item.Namespace, generation)
		if lifecycleErr != nil {
			slog.Warn("catalog lifecycle check failed; leaving entry pending", "entry_id", msg.ID, "error", lifecycleErr)
			return
		}
		if disposition == nslifecycle.EnvelopeStale {
			metrics.StaleGenerationTotal.WithLabelValues("catalog", "stale").Inc()
			w.ack(ctx, msg.ID)
			return
		}
	}

	if err := w.service.IngestStreamItem(ctx, item); err != nil {
		if errors.Is(err, ErrCatalogItemRejected) {
			slog.Warn("catalog ingest rejecting stream item",
				"entry_id", msg.ID, "namespace", item.Namespace, "object_id", item.ObjectID, "error", err)
			metrics.CatalogStreamRejectsTotal.WithLabelValues(item.Namespace, "invalid").Inc()
			w.ack(ctx, msg.ID)
			return
		}
		slog.Warn("catalog ingest failed; leaving entry pending",
			"entry_id", msg.ID, "namespace", item.Namespace, "object_id", item.ObjectID, "error", err)
		return
	}
	w.ack(ctx, msg.ID)
}

func (w *CatalogWorker) ack(ctx context.Context, id string) {
	if err := w.ackFn(ctx, codohuetypes.CatalogStreamName, catalogConsumerGroup, id); err != nil {
		slog.Warn("catalog ingest xack failed", "entry_id", id, "error", err)
	}
}

func decodeCatalogMessage(msg redis.XMessage) (*codohuetypes.CatalogStreamItem, error) {
	raw, ok := msg.Values[codohuetypes.PayloadField]
	if !ok {
		return nil, fmt.Errorf("missing payload field")
	}
	var item codohuetypes.CatalogStreamItem
	if err := json.Unmarshal(fmt.Append(nil, raw), &item); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	if item.Namespace == "" {
		return nil, fmt.Errorf("missing namespace")
	}
	return &item, nil
}
