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

const runtimePrefix = "admin:runtime:v1:"

// StartRuntimeReporter publishes effective, allowlisted settings every 30 seconds.
// Each boot has its own expiring key, so replicas and rolling restarts are visible.
func StartRuntimeReporter(ctx context.Context, client *goredis.Client, process string, settings []config.RuntimeSetting) func() {
	ctx, cancel := context.WithCancel(ctx)
	if client == nil {
		return cancel
	}
	instance, _ := os.Hostname()
	snapshot := config.RuntimeSnapshot{Process: process, Instance: instance, StartedAt: time.Now().UTC(), Settings: settings}
	key := runtimePrefix + process + ":" + rand.Text()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			snapshot.ReportedAt = time.Now().UTC()
			body, err := json.Marshal(snapshot)
			if err == nil {
				writeCtx, done := context.WithTimeout(ctx, 3*time.Second)
				err = client.Set(writeCtx, key, body, config.RuntimeReportTTL).Err()
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
	snapshots := make([]config.RuntimeSnapshot, 0)
	iter := client.Scan(ctx, 0, runtimePrefix+"*", 100).Iterator()
	seen := map[string]bool{}
	for iter.Next(ctx) {
		key := iter.Val()
		if seen[key] {
			continue
		}
		seen[key] = true
		raw, err := client.Get(ctx, key).Bytes()
		if err == goredis.Nil {
			continue
		}
		if err != nil {
			return nil, err
		}
		var snapshot config.RuntimeSnapshot
		if err := json.Unmarshal(raw, &snapshot); err != nil {
			return nil, fmt.Errorf("invalid runtime snapshot")
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].Process == snapshots[j].Process {
			return snapshots[i].StartedAt.Before(snapshots[j].StartedAt)
		}
		return snapshots[i].Process < snapshots[j].Process
	})
	return snapshots, nil
}
