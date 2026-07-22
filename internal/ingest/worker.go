package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	streamName    = "codohue:events"
	consumerGroup = "codohue-ingest"

	// defaultConsumerName is the consumer name used when no replica name is
	// configured and the hostname cannot be resolved.
	defaultConsumerName = "worker-1"

	// reapInterval is how often the pending-entries list is scanned for
	// entries whose consumer died or whose processing failed; minIdleReap is
	// how long an entry must sit unacknowledged before it is reclaimed.
	reapInterval  = time.Minute
	minIdleReap   = time.Minute
	reapBatchSize = 100

	readErrBackoffMin = time.Second
	readErrBackoffMax = 30 * time.Second
)

type eventProcessor interface {
	Process(ctx context.Context, payload *EventPayload) (int64, error)
}

// Worker consumes events from Redis Streams and forwards them to the Service for processing.
type Worker struct {
	redis         *redis.Client
	service       eventProcessor
	consumer      string
	createGroupFn func(ctx context.Context, stream, group, start string) error
	readGroupFn   func(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XStream, error)
	autoClaimFn   func(ctx context.Context, args *redis.XAutoClaimArgs) ([]redis.XMessage, string, error)
	ackFn         func(ctx context.Context, stream, group string, ids ...string) error
}

// NewWorker creates a new Worker with the given Redis client and ingest
// service. consumer is the name this replica joins the consumer group with;
// empty falls back to defaultConsumerName.
func NewWorker(redisClient *redis.Client, service *Service, consumer string) *Worker {
	if consumer == "" {
		consumer = defaultConsumerName
	}
	return &Worker{
		redis:    redisClient,
		service:  service,
		consumer: consumer,
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
func (w *Worker) Init(ctx context.Context) error {
	err := w.createGroupFn(ctx, streamName, consumerGroup, "0")
	if err != nil && !isBusyGroupErr(err) {
		return fmt.Errorf("create consumer group: %w", err)
	}
	return nil
}

// Run starts consuming events from Redis Streams (blocking until ctx is
// cancelled). A reaper goroutine periodically reclaims pending entries left
// behind by crashed replicas or failed processing attempts, so delivery is
// at-least-once: an entry is only ACKed after the event row is persisted (or
// when it is permanently unprocessable).
func (w *Worker) Run(ctx context.Context) {
	slog.Info("ingest worker started", "stream", streamName, "consumer", w.consumer)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.reapPending(ctx)
	}()
	defer func() {
		wg.Wait()
		slog.Info("ingest worker stopped")
	}()

	backoff := readErrBackoffMin
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := w.readGroupFn(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: w.consumer,
			Streams:  []string{streamName, ">"},
			Count:    10,
			Block:    5 * time.Second,
		})
		if errors.Is(err, redis.Nil) {
			continue // no new messages within the block window
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if isNoGroupErr(err) {
				// The stream or group vanished (e.g. Redis restarted without
				// persistence). Recreate instead of spinning on NOGROUP.
				if createErr := w.createGroupFn(ctx, streamName, consumerGroup, "0"); createErr != nil && !isBusyGroupErr(createErr) {
					slog.Warn("ingest recreate consumer group failed", "error", createErr)
				}
			}
			slog.Warn("ingest xreadgroup failed", "error", err)
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

// reapPending periodically reclaims entries idle in the PEL — left there by a
// crashed replica or by a processing failure — and re-processes them.
func (w *Worker) reapPending(ctx context.Context) {
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

// reapOnce runs a single XAUTOCLAIM pass and re-processes what it claimed.
func (w *Worker) reapOnce(ctx context.Context) {
	msgs, _, err := w.autoClaimFn(ctx, &redis.XAutoClaimArgs{
		Stream:   streamName,
		Group:    consumerGroup,
		Consumer: w.consumer,
		MinIdle:  minIdleReap,
		Start:    "0",
		Count:    reapBatchSize,
	})
	if err != nil {
		// NOGROUP here just means nothing has been ingested since the
		// stream vanished; the read loop recreates the group.
		if ctx.Err() == nil && !isNoGroupErr(err) {
			slog.Warn("ingest xautoclaim failed", "error", err)
		}
		return
	}
	for _, msg := range msgs {
		w.handleMessage(ctx, msg)
	}
}

// handleMessage decodes and processes one stream entry. Permanently
// unprocessable entries (malformed JSON, failed validation) are ACKed and
// dropped — they can never succeed and would otherwise clog the PEL. Other
// failures leave the entry pending so the reaper redelivers it.
func (w *Worker) handleMessage(ctx context.Context, msg redis.XMessage) {
	payload, err := decodeEventMessage(msg)
	if err != nil {
		slog.Warn("ingest dropping malformed stream entry", "entry_id", msg.ID, "error", err)
		w.ack(ctx, msg.ID)
		return
	}

	if _, err := w.service.Process(ctx, payload); err != nil {
		if errors.Is(err, ErrInvalidPayload) || errors.Is(err, ErrUnknownAction) {
			slog.Warn("ingest dropping unprocessable event", "entry_id", msg.ID, "error", err)
			w.ack(ctx, msg.ID)
			return
		}
		slog.Warn("ingest process event failed; leaving entry pending", "entry_id", msg.ID, "error", err)
		return
	}
	w.ack(ctx, msg.ID)
}

func (w *Worker) ack(ctx context.Context, id string) {
	if err := w.ackFn(ctx, streamName, consumerGroup, id); err != nil {
		slog.Warn("ingest xack failed", "entry_id", id, "error", err)
	}
}

func decodeEventMessage(msg redis.XMessage) (*EventPayload, error) {
	raw, ok := msg.Values["payload"]
	if !ok {
		return nil, fmt.Errorf("missing payload field")
	}
	var payload EventPayload
	if err := json.Unmarshal(fmt.Append(nil, raw), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}
	return &payload, nil
}

func isBusyGroupErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "BUSYGROUP")
}

func isNoGroupErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "NOGROUP")
}

// sleepCtx sleeps for d or until ctx is done; it reports false when ctx ended.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}
