package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

var (
	parseURLFn   = redis.ParseURL
	newClientFn  = redis.NewClient
	pingClientFn = func(ctx context.Context, client *redis.Client) error {
		return client.Ping(ctx).Err()
	}
)

// NewClient creates a Redis client and verifies connectivity with PING.
func NewClient(redisURL string) (*redis.Client, error) {
	opts, err := parseURLFn(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	client := newClientFn(opts)

	if err := pingClientFn(context.Background(), client); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
