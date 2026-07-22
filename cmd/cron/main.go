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

	shutdownTimer := time.NewTimer(30 * time.Second)
	defer shutdownTimer.Stop()
	for _, done := range []<-chan struct{}{jobDone, retentionDone} {
		select {
		case <-done:
		case <-shutdownTimer.C:
			slog.Warn("cron shutdown timed out waiting for background jobs")
			return nil
		}
	}

	slog.Info("cron stopped")
	return nil
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
