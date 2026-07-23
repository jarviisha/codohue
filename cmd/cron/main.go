package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jarviisha/codohue/internal/compute"
	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/idmap"
	infrapg "github.com/jarviisha/codohue/internal/infra/postgres"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/jarviisha/codohue/internal/nsconfig"
	"github.com/jarviisha/codohue/internal/retention"
)

var (
	loadConfigFn    = config.LoadCron
	newPoolFn       = infrapg.NewPool
	newQdrantFn     = infraqdrant.NewClient
	newRedisFn      = infraredis.NewClient
	newComputeJobFn = compute.NewJob
	signalNotifyFn  = signal.Notify
	closePoolFn     = func(db *pgxpool.Pool) { db.Close() }
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := loadConfigFn()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	initLogger(cfg.LogFormat)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := newPoolFn(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer closePoolFn(db)

	qdrantClient, err := newQdrantFn(cfg.QdrantHost, cfg.QdrantPort)
	if err != nil {
		return fmt.Errorf("connect qdrant: %w", err)
	}

	// Redis is used by Phase 3 (trending). Connect when available; Phase 3 is skipped if nil.
	redisClient, err := newRedisFn(cfg.RedisURL)
	if err != nil {
		slog.Warn("redis unavailable, trending phase will be skipped", "error", err)
		redisClient = nil
	}

	idmapRepo := idmap.NewRepository(db)
	idmapSvc := idmap.NewService(idmapRepo)

	nsConfigRepo := nsconfig.NewRepository(db)
	nsConfigSvc := nsconfig.NewService(nsConfigRepo)

	computeRepo := compute.NewRepository(db)
	computeSvc := compute.NewService(computeRepo, idmapSvc, qdrantClient)
	job := newComputeJobFn(computeSvc, nsConfigSvc, computeRepo, qdrantClient, idmapSvc, redisClient, cfg.BatchIntervalMinutes)
	// Publish run lifecycle events so cmd/admin can stream cron runs live —
	// its in-process observer never sees them (different process).
	if obs := compute.NewRedisBatchRunObserver(redisClient); obs != nil {
		job.SetObserver(obs)
	}

	jobDone := make(chan struct{})
	go func() {
		defer close(jobDone)
		job.Run(ctx)
	}()

	// Retention: prune batch_run_logs + catalog_backlog_samples on a
	// configurable interval. Both windows can be disabled by setting their
	// *_RETENTION_DAYS env var to 0; the job stays alive but only logs.
	retentionJob := retention.NewJob(retention.NewPgRepository(db), retention.Config{
		BatchRunRetentionDays:       cfg.BatchRunRetentionDays,
		BacklogSamplesRetentionDays: cfg.BacklogSamplesRetentionDays,
		Interval:                    cfg.RetentionInterval,
	})
	retentionDone := make(chan struct{})
	go func() {
		defer close(retentionDone)
		if err := retentionJob.Run(ctx); err != nil {
			slog.Error("retention job exited with error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signalNotifyFn(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Cancel first, then join both goroutines BEFORE the deferred pool close
	// runs — a run interrupted mid-phase still gets to finalize its
	// batch_run_logs row against a live pool.
	slog.Info("cron shutting down")
	cancel()

	timer := time.NewTimer(shutdownDrainTimeout)
	defer timer.Stop()
	if drainDone([]<-chan struct{}{jobDone, retentionDone}, timer.C) {
		slog.Info("cron stopped")
	} else {
		slog.Warn("cron shutdown timed out waiting for background jobs")
	}
	return nil
}

// shutdownDrainTimeout bounds how long shutdown waits for the job + retention
// goroutines to finish so a hung goroutine can't block exit forever.
const shutdownDrainTimeout = 30 * time.Second

// drainDone waits for every channel in dones to close, or for timeout to
// fire. Returns true when all drained cleanly, false on timeout. Extracted
// from run() so the shutdown join is unit-tested rather than buried in
// unreachable main wiring.
func drainDone(dones []<-chan struct{}, timeout <-chan time.Time) bool {
	for _, done := range dones {
		select {
		case <-done:
		case <-timeout:
			return false
		}
	}
	return true
}

func initLogger(format string) {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: slog.LevelInfo}
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}
