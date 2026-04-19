package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/redis/go-redis/v9"
)

const (
	streamName    = "codohue:events"
	consumerGroup = "codohue-ingest"
	consumerName  = "worker-1"
)

type eventProcessor interface {
	Process(ctx context.Context, payload *EventPayload) error
}

// Worker consumes events from Redis Streams and forwards them to the Service for processing.
type Worker struct {
	redis         *redis.Client
	service       eventProcessor
	createGroupFn func(ctx context.Context, stream, group, start string) error
	readGroupFn   func(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XStream, error)
	ackFn         func(ctx context.Context, stream, group string, ids ...string) error
}

// NewWorker creates a new Worker with the given Redis client and ingest service.
func NewWorker(redisClient *redis.Client, service *Service) *Worker {
	return &Worker{
		redis:   redisClient,
		service: service,
		createGroupFn: func(ctx context.Context, stream, group, start string) error {
			return redisClient.XGroupCreateMkStream(ctx, stream, group, start).Err()
		},
		readGroupFn: func(ctx context.Context, args *redis.XReadGroupArgs) ([]redis.XStream, error) {
			return redisClient.XReadGroup(ctx, args).Result()
		},
		ackFn: func(ctx context.Context, stream, group string, ids ...string) error {
			return redisClient.XAck(ctx, stream, group, ids...).Err()
		},
	}
}

// Init creates the consumer group if it does not already exist.
func (w *Worker) Init(ctx context.Context) error {
	err := w.createGroupFn(ctx, streamName, consumerGroup, "0")
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("create consumer group: %w", err)
	}
	return nil
}

// Run starts consuming events from Redis Streams (blocking).
func (w *Worker) Run(ctx context.Context) {
	log.Printf("[ingest] worker started, listening on stream %q", streamName)

	for {
		select {
		case <-ctx.Done():
			log.Println("[ingest] worker stopped")
			return
		default:
		}

		streams, err := w.readGroupFn(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumerName,
			Streams:  []string{streamName, ">"},
			Count:    10,
			Block:    0,
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("[ingest] xreadgroup error: %v", err)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				if err := w.handleMessage(ctx, msg); err != nil {
					log.Printf("[ingest] handle message %s error: %v", msg.ID, err)
					continue
				}
				w.ackFn(ctx, streamName, consumerGroup, msg.ID) //nolint:errcheck // ack is best-effort
			}
		}
	}
}

func (w *Worker) handleMessage(ctx context.Context, msg redis.XMessage) error {
	raw, ok := msg.Values["payload"]
	if !ok {
		return fmt.Errorf("missing payload field")
	}

	var payload EventPayload
	if err := json.Unmarshal(fmt.Append(nil, raw), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	if err := w.service.Process(ctx, &payload); err != nil {
		return fmt.Errorf("process event: %w", err)
	}
	return nil
}
