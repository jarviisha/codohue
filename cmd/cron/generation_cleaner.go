package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/qdrant/go-client/qdrant"
	"github.com/redis/go-redis/v9"
)

// generationRedisCleaner is the slice of Redis the cleaner needs, expressed in
// plain values so a test can drive it without a live server.
type generationRedisCleaner interface {
	ScanKeys(ctx context.Context, pattern string) ([]string, error)
	DeleteKeys(ctx context.Context, keys []string) error
}

// generationQdrantCleaner is the slice of Qdrant the cleaner needs.
// *qdrant.Client satisfies it directly.
type generationQdrantCleaner interface {
	CollectionExists(ctx context.Context, name string) (bool, error)
	DeleteCollection(ctx context.Context, name string) error
}

// redisClientCleaner adapts the go-redis command API to plain values.
type redisClientCleaner struct {
	client *redis.Client
}

// ScanKeys collects every key matching pattern. Scanning rather than KEYS
// because the recommendation cache holds one key per (subject, limit) pair.
func (c redisClientCleaner) ScanKeys(ctx context.Context, pattern string) ([]string, error) {
	var keys []string
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan %q: %w", pattern, err)
	}
	return keys, nil
}

func (c redisClientCleaner) DeleteKeys(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("delete %d key(s): %w", len(keys), err)
	}
	return nil
}

// storeGenerationCleaner implements nslifecycle.GenerationCleaner over the live
// stores. It lives here rather than in the core package because that package
// deliberately keeps no store dependencies.
type storeGenerationCleaner struct {
	redis  generationRedisCleaner
	qdrant generationQdrantCleaner
}

// newStoreGenerationCleaner tolerates a nil Redis client because cron already
// runs without Redis (phase 3 is skipped). Candidates are re-listed on every
// pass, so a generation whose Redis half is skipped now is reclaimed once
// Redis returns.
func newStoreGenerationCleaner(redisClient *redis.Client, qdrantClient *qdrant.Client) *storeGenerationCleaner {
	cleaner := &storeGenerationCleaner{}
	if redisClient != nil {
		cleaner.redis = redisClientCleaner{client: redisClient}
	}
	if qdrantClient != nil {
		cleaner.qdrant = qdrantClient
	}
	return cleaner
}

// DeleteRedisGeneration removes the trending ZSET, the embed stream, and every
// recommendation cache key belonging to one superseded generation. Names come
// from the lifecycle resolver so the cleaner deletes exactly what the writers
// created.
func (c *storeGenerationCleaner) DeleteRedisGeneration(ctx context.Context, candidate nslifecycle.CleanupCandidate) error {
	if c.redis == nil {
		return nil
	}
	keys := make([]string, 0, 2)
	for _, kind := range []nslifecycle.PhysicalKind{nslifecycle.KindTrending, nslifecycle.KindEmbedStream} {
		name, err := nslifecycle.PhysicalName(kind, candidate.Namespace, candidate.Generation)
		if err != nil {
			return err
		}
		keys = append(keys, name)
	}
	cachePrefix, err := nslifecycle.PhysicalName(nslifecycle.KindRecommendationCache, candidate.Namespace, candidate.Generation)
	if err != nil {
		return err
	}
	cached, err := c.redis.ScanKeys(ctx, cachePrefix+":*")
	if err != nil {
		return fmt.Errorf("scan recommendation cache: %w", err)
	}
	keys = append(keys, cached...)
	if err := c.redis.DeleteKeys(ctx, keys); err != nil {
		return fmt.Errorf("delete redis keys: %w", err)
	}
	return nil
}

// DeleteQdrantGeneration drops the four collections belonging to one superseded
// generation, skipping those already gone so a retry is a no-op.
func (c *storeGenerationCleaner) DeleteQdrantGeneration(ctx context.Context, candidate nslifecycle.CleanupCandidate) error {
	if c.qdrant == nil {
		return nil
	}
	for _, kind := range []nslifecycle.PhysicalKind{
		nslifecycle.KindSubjects,
		nslifecycle.KindObjects,
		nslifecycle.KindSubjectsDense,
		nslifecycle.KindObjectsDense,
	} {
		collection, err := nslifecycle.PhysicalName(kind, candidate.Namespace, candidate.Generation)
		if err != nil {
			return err
		}
		exists, err := c.qdrant.CollectionExists(ctx, collection)
		if err != nil {
			return fmt.Errorf("check collection %q: %w", collection, err)
		}
		if !exists {
			continue
		}
		if err := c.qdrant.DeleteCollection(ctx, collection); err != nil {
			return fmt.Errorf("delete collection %q: %w", collection, err)
		}
	}
	return nil
}

// generationCleanupRunner is the bounded reclaim pass the loop drives.
type generationCleanupRunner interface {
	RunOnce(ctx context.Context, limit int) (int, error)
}

// generationCleanupBatch bounds one pass so a large backlog is spread over
// several ticks instead of holding both stores for one long sweep.
const generationCleanupBatch = 50

// runGenerationCleanupLoop reclaims superseded generations until ctx is
// cancelled. An open legacy gate is the expected pre-closure state rather than
// a failure, so it is logged once per process instead of on every tick.
func runGenerationCleanupLoop(ctx context.Context, interval time.Duration, janitor generationCleanupRunner) {
	if interval <= 0 {
		interval = time.Hour
	}
	gateLogged := false
	runPass := func() {
		cleaned, err := janitor.RunOnce(ctx, generationCleanupBatch)
		switch {
		case errors.Is(err, nslifecycle.ErrLegacyEnvelopesOpen):
			if !gateLogged {
				slog.InfoContext(ctx, "deleted-generation cleanup idle until legacy envelopes are disabled")
				gateLogged = true
			}
			return
		case err != nil:
			slog.WarnContext(ctx, "deleted-generation cleanup failed", "cleaned", cleaned, "error", err)
			return
		}
		gateLogged = false
		if cleaned == 0 {
			return
		}
		slog.InfoContext(ctx, "reclaimed superseded generations", "count", cleaned)
		// The ledger re-lists the same bounded page every pass, so a saturated
		// batch is the one signal an operator has that candidates beyond it are
		// waiting.
		if cleaned == generationCleanupBatch {
			slog.InfoContext(ctx, "deleted-generation cleanup hit its per-pass bound", "limit", generationCleanupBatch)
		}
	}
	runPass()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runPass()
		}
	}
}
