package redis

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"time"

	"github.com/jarviisha/codohue/internal/config"
	goredis "github.com/redis/go-redis/v9"
)

// runtimeKey holds every process report as one hash field per boot, so a reader
// costs a single HGETALL instead of a keyspace scan on shared Redis.
const runtimeKey = "admin:runtime:v1"

// StartRuntimeReporter publishes effective, allowlisted settings every 30 seconds.
// Each boot has its own expiring key, so replicas and rolling restarts are visible.
func StartRuntimeReporter(ctx context.Context, client *goredis.Client, process string, settings []config.RuntimeSetting) func() {
	ctx, cancel := context.WithCancel(ctx)
	if client == nil {
		return cancel
	}
	instance, _ := os.Hostname()
	snapshot := config.RuntimeSnapshot{Process: process, Instance: instance, StartedAt: time.Now().UTC(), Settings: settings}
	field := process + ":" + rand.Text()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			snapshot.ReportedAt = time.Now().UTC()
			body, err := json.Marshal(snapshot)
			if err == nil {
				writeCtx, done := context.WithTimeout(ctx, 3*time.Second)
				// Refreshing the whole-hash expiry each tick means the key vanishes
				// once every process stops reporting.
				if err = client.HSet(writeCtx, runtimeKey, field, body).Err(); err == nil {
					err = client.Expire(writeCtx, runtimeKey, config.RuntimeReportTTL).Err()
				}
				done()
			}
			if err != nil && ctx.Err() == nil {
				slog.Warn("runtime snapshot publish failed", "process", process)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return cancel
}

// RuntimeSnapshots lists only recent self-reported process snapshots.
func RuntimeSnapshots(ctx context.Context, client *goredis.Client) ([]config.RuntimeSnapshot, error) {
	if client == nil {
		return nil, fmt.Errorf("runtime reporting unavailable")
	}
	entries, err := client.HGetAll(ctx, runtimeKey).Result()
	if err != nil {
		return nil, err
	}
	snapshots := make([]config.RuntimeSnapshot, 0, len(entries))
	stale := make([]string, 0)
	for field, raw := range entries {
		var snapshot config.RuntimeSnapshot
		if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
			return nil, fmt.Errorf("invalid runtime snapshot")
		}
		// Hash fields carry no individual TTL, so a boot that stopped reporting is
		// dropped by age and pruned; the set is small and bounded by replica count.
		if time.Since(snapshot.ReportedAt) > config.RuntimeReportTTL {
			stale = append(stale, field)
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	if len(stale) > 0 {
		_ = client.HDel(ctx, runtimeKey, stale...).Err()
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Process == snapshots[j].Process {
			return snapshots[i].StartedAt.Before(snapshots[j].StartedAt)
		}
		return snapshots[i].Process < snapshots[j].Process
	})
	return snapshots, nil
}
