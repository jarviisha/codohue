//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	goredis "github.com/redis/go-redis/v9"
)

func TestStreamRetentionAcrossTenWindowsAndCapacityBackpressure(t *testing.T) {
	ctx := context.Background()
	suffix := strconv.FormatInt(testingSeed(), 10)
	specs := []infraredis.StreamSpec{
		{Name: "e2e:retention:events:" + suffix, Kind: "events", ExpectedGroups: []string{"codohue-ingest", "audit"}},
		{Name: "e2e:retention:catalog:" + suffix, Kind: "catalog", ExpectedGroups: []string{"codohue-catalog-ingest", "audit"}},
		{Name: "catalog:embed:e2e_retention_" + suffix, Kind: "embed", Namespace: "e2e_retention_" + suffix, ExpectedGroups: []string{"embedder", "audit"}},
	}
	for _, spec := range specs {
		spec := spec
		t.Cleanup(func() { testRedis.Del(ctx, spec.Name) }) //nolint:errcheck
		for _, group := range spec.ExpectedGroups {
			ensureRedisGroup(t, spec.Name, group, "0")
		}
	}

	retention := infraredis.NewRetention(testRedis, false)
	for window := 0; window < 10; window++ {
		for _, spec := range specs {
			for item := 0; item < 5; item++ {
				if err := testRedis.XAdd(ctx, &goredis.XAddArgs{
					Stream: spec.Name,
					Values: map[string]any{"window": window, "item": item},
				}).Err(); err != nil {
					t.Fatalf("xadd %q: %v", spec.Name, err)
				}
			}
			for _, group := range spec.ExpectedGroups {
				messages, err := testRedis.XReadGroup(ctx, &goredis.XReadGroupArgs{
					Group: group, Consumer: "e2e", Streams: []string{spec.Name, ">"}, Count: 5,
				}).Result()
				if err != nil {
					t.Fatalf("xreadgroup %q/%q: %v", spec.Name, group, err)
				}
				var ids []string
				for _, stream := range messages {
					for _, message := range stream.Messages {
						ids = append(ids, message.ID)
					}
				}
				if len(ids) != 5 {
					t.Fatalf("window %d %q/%q read %d entries, want 5", window, spec.Name, group, len(ids))
				}
				if err := testRedis.XAck(ctx, spec.Name, group, ids...).Err(); err != nil {
					t.Fatalf("xack %q/%q: %v", spec.Name, group, err)
				}
			}
			result, err := retention.RunOnce(ctx, spec)
			if err != nil {
				t.Fatalf("retention window %d %q: %v", window, spec.Name, err)
			}
			assertNoStreamEntryBelow(t, spec.Name, result.SafeFrontier)
		}
	}

	// Leave one event pending in the primary group while the audit group ACKs
	// it. The multi-group frontier must preserve the only durable copy.
	protectedID, err := testRedis.XAdd(ctx, &goredis.XAddArgs{
		Stream: specs[0].Name, Values: map[string]any{"protected": true},
	}).Result()
	if err != nil {
		t.Fatalf("publish protected entry: %v", err)
	}
	for _, group := range specs[0].ExpectedGroups {
		messages, err := testRedis.XReadGroup(ctx, &goredis.XReadGroupArgs{
			Group: group, Consumer: "e2e", Streams: []string{specs[0].Name, ">"}, Count: 1,
		}).Result()
		if err != nil || len(messages) != 1 || len(messages[0].Messages) != 1 {
			t.Fatalf("read protected entry for %q: messages=%v err=%v", group, messages, err)
		}
		if group == "audit" {
			if err := testRedis.XAck(ctx, specs[0].Name, group, protectedID).Err(); err != nil {
				t.Fatalf("ack audit protected entry: %v", err)
			}
		}
	}
	if _, err := retention.RunOnce(ctx, specs[0]); err != nil {
		t.Fatalf("retention with pending entry: %v", err)
	}
	if got, err := testRedis.XRangeN(ctx, specs[0].Name, protectedID, protectedID, 1).Result(); err != nil || len(got) != 1 {
		t.Fatalf("unprocessed entry %q was not recoverable: got=%v err=%v", protectedID, got, err)
	}

	assertRedisNoEvictionRejectsWithoutDeleting(t, specs[0].Name, protectedID)
}

func assertRedisNoEvictionRejectsWithoutDeleting(t testing.TB, stream, protectedID string) {
	t.Helper()
	ctx := context.Background()
	oldPolicy, err := testRedis.ConfigGet(ctx, "maxmemory-policy").Result()
	if err != nil {
		t.Fatalf("get maxmemory policy: %v", err)
	}
	oldMax, err := testRedis.ConfigGet(ctx, "maxmemory").Result()
	if err != nil {
		t.Fatalf("get maxmemory: %v", err)
	}
	t.Cleanup(func() {
		testRedis.ConfigSet(ctx, "maxmemory", oldMax["maxmemory"])                  //nolint:errcheck
		testRedis.ConfigSet(ctx, "maxmemory-policy", oldPolicy["maxmemory-policy"]) //nolint:errcheck
	})

	info, err := testRedis.Info(ctx, "memory").Result()
	if err != nil {
		t.Fatalf("redis memory info: %v", err)
	}
	used, err := redisInfoInt(info, "used_memory")
	if err != nil {
		t.Fatal(err)
	}
	if err := testRedis.ConfigSet(ctx, "maxmemory-policy", "noeviction").Err(); err != nil {
		t.Fatalf("set noeviction: %v", err)
	}
	if err := testRedis.ConfigSet(ctx, "maxmemory", strconv.FormatInt(used+1024, 10)).Err(); err != nil {
		t.Fatalf("set constrained maxmemory: %v", err)
	}
	err = testRedis.XAdd(ctx, &goredis.XAddArgs{
		Stream: stream, Values: map[string]any{"payload": strings.Repeat("x", 1<<20)},
	}).Err()
	if err == nil {
		t.Fatal("expected explicit Redis capacity error, got nil")
	}
	if got, rangeErr := testRedis.XRangeN(ctx, stream, protectedID, protectedID, 1).Result(); rangeErr != nil || len(got) != 1 {
		t.Fatalf("capacity rejection removed protected entry %q: got=%v err=%v", protectedID, got, rangeErr)
	}
}

func redisInfoInt(info, key string) (int64, error) {
	for _, line := range strings.Split(info, "\n") {
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && name == key {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return 0, fmt.Errorf("parse Redis %s: %w", key, err)
			}
			return parsed, nil
		}
	}
	return 0, fmt.Errorf("Redis INFO omitted %s", key)
}

func testingSeed() int64 {
	return time.Now().UnixNano()
}
