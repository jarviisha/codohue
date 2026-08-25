package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
)

type trendingPipeline interface {
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	ZAdd(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	Exec(ctx context.Context) ([]redis.Cmder, error)
}

var (
	newPipelineFn = func(rdb *redis.Client) trendingPipeline {
		return rdb.Pipeline()
	}
	zRevRangeWithScoresFn = func(ctx context.Context, rdb *redis.Client, key string, start, stop int64) ([]redis.Z, error) {
		return rdb.ZRevRangeWithScores(ctx, key, start, stop).Result()
	}
)

// TrendingEntry is a single item returned from the trending sorted set.
type TrendingEntry struct {
	ObjectID string
	Score    float64
}

// trendingKey returns the Redis key for a namespace's trending sorted set.
func trendingKey(namespace string) string {
	return trendingKeyForGeneration(namespace, 1)
}

func trendingKeyForGeneration(namespace string, generation int64) string {
	if generation < 1 {
		generation = 1
	}
	return nslifecycle.MustPhysicalName(nslifecycle.KindTrending, namespace, generation)
}

// StoreTrending atomically replaces the trending sorted set for a namespace with the
// given scores and sets a TTL. A DEL + ZADD + EXPIRE pipeline ensures a stale
// read window of at most one round-trip duration.
func StoreTrending(ctx context.Context, rdb *redis.Client, namespace string, scores map[string]float64, ttl time.Duration) error {
	return StoreTrendingForGeneration(ctx, rdb, namespace, 1, scores, ttl)
}

// StoreTrendingForGeneration replaces the current lifecycle's sorted set.
func StoreTrendingForGeneration(ctx context.Context, rdb *redis.Client, namespace string, generation int64, scores map[string]float64, ttl time.Duration) error {
	key := trendingKeyForGeneration(namespace, generation)
	members := make([]redis.Z, 0, len(scores))
	for id, score := range scores {
		members = append(members, redis.Z{Score: score, Member: id})
	}

	pipe := newPipelineFn(rdb)
	pipe.Del(ctx, key)
	if len(members) > 0 {
		pipe.ZAdd(ctx, key, members...)
		pipe.Expire(ctx, key, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("store trending %s: %w", namespace, err)
	}
	return nil
}

// GetTrending reads the top trending items from Redis, ordered by score descending.
// offset and limit implement pagination (0-based offset, 0 limit returns nothing).
func GetTrending(ctx context.Context, rdb *redis.Client, namespace string, offset, limit int) ([]TrendingEntry, error) {
	return GetTrendingForGeneration(ctx, rdb, namespace, 1, offset, limit)
}

// GetTrendingForGeneration reads the current lifecycle's trending set.
func GetTrendingForGeneration(ctx context.Context, rdb *redis.Client, namespace string, generation int64, offset, limit int) ([]TrendingEntry, error) {
	if limit <= 0 {
		return nil, nil
	}
	key := trendingKeyForGeneration(namespace, generation)
	results, err := zRevRangeWithScoresFn(ctx, rdb, key, int64(offset), int64(offset+limit-1))
	if err != nil {
		return nil, fmt.Errorf("get trending %s: %w", namespace, err)
	}

	entries := make([]TrendingEntry, 0, len(results))
	for _, r := range results {
		str, ok := r.Member.(string)
		if !ok {
			continue
		}
		entries = append(entries, TrendingEntry{ObjectID: str, Score: r.Score})
	}
	return entries, nil
}
