package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	qdrantpb "github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/embedder"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	infrapg "github.com/jarviisha/codohue/internal/infra/postgres"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

// Indirection points so cmd/embedder/main_test.go can stub out the heavy
// infra dependencies — same pattern as cmd/cron/main.go and cmd/api/main.go.
var (
	loadConfigFn      = config.LoadEmbedder
	newPoolFn         = infrapg.NewPool
	newRedisFn        = infraredis.NewClient
	newQdrantFn       = infraqdrant.NewClient
	registerMetricsFn = metrics.Register
	signalNotifyFn    = signal.Notify
	closePoolFn       = func(db *pgxpool.Pool) { db.Close() }
	closeRedisFn      = func(client *goredis.Client) error { return client.Close() }
	hostnameFn        = os.Hostname
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

	registerMetricsFn()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := newPoolFn(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer closePoolFn(db)

	redisClient, err := newRedisFn(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer func() {
		if err := closeRedisFn(redisClient); err != nil {
			slog.Error("close redis failed", "error", err)
		}
	}()

	qdrantClient, err := newQdrantFn(cfg.QdrantHost, cfg.QdrantPort)
	if err != nil {
		return fmt.Errorf("connect qdrant: %w", err)
	}

	// Domain wiring. The embedder service depends on:
	//   - catalog_items state transitions  (embedder.Repository)
	//   - namespace config lookups         (nsconfig.Service)
	//   - the embedstrategy registry       (DefaultRegistry, populated by
	//                                       internal/embedder/strategy.go init)
	//   - the id_mappings allocator        (idmap.Service)
	//   - the qdrant client                (for upserts to {ns}_objects_dense)
	idmapRepo := idmap.NewRepository(db)
	idmapSvc := idmap.NewService(idmapRepo)

	nsConfigRepo := nsconfig.NewRepository(db)
	nsConfigSvc := nsconfig.NewService(nsConfigRepo)

	embedderRepo := embedder.NewRepository(db)
	embedderSvc := embedder.NewService(
		embedderRepo,
		nsConfigSvc,
		embedstrategy.DefaultRegistry(),
		idmapSvc,
		qdrantClient,
	)
	// Pub/sub publisher broadcasts item state changes + backlog snapshots
	// + dead-letter growth alerts to codohue:catalog-events:{ns}; cmd/admin's
	// catalog bridge subscribes and republishes onto the admin event bus
	// for SSE fan-out. One publisher is shared by the embed service (per-
	// item events) and the sampler (snapshot + alert events).
	catalogPublisher := embedder.NewRedisCatalogEventPublisher(redisClient)
	embedderSvc.SetEventPublisher(catalogPublisher)

	consumerName := cfg.ReplicaName
	if consumerName == "" {
		if h, err := hostnameFn(); err == nil {
			consumerName = h
		} else {
			consumerName = "embedder-replica"
		}
	}

	worker := embedder.NewWorker(redisClient, embedderSvc, nsConfigSvc, embedder.WorkerConfig{
		ConsumerName: consumerName,
		PollInterval: cfg.NamespacePollInterval,
	})

	// Re-embed completion watcher (US3): closes batch_run_logs rows once a
	// namespace's catalog backlog at the new strategy_version has drained.
	// Also emits one reembed_progress event per open run per tick so the
	// SPA overlay can render a live progress bar.
	reembedWatcher := embedder.NewReembedWatcher(embedder.NewPgReembedRepo(db), 5*time.Second)
	reembedWatcher.SetEventPublisher(catalogPublisher)

	// Backlog sampler — snapshots per-namespace catalog backlog into
	// catalog_backlog_samples on a 30s tick. Backs the admin /catalog/
	// backlog-history endpoint so the timeline survives reload. Wired to
	// the same publisher so each persisted sample also fans out a live
	// backlog_snapshot event to the SPA (and dead_letter_grew on rises).
	backlogSampler := embedder.NewBacklogSampler(embedderRepo, redisClient, nsConfigSvc, embedder.BacklogSamplerConfig{})
	backlogSampler.SetEventPublisher(catalogPublisher)

	// Recovery sweeper — re-publishes catalog_items rows whose stream entry
	// was lost (failed producer XADD, ack without a terminal state write),
	// so nothing stays 'pending'/'in_flight' forever. Only acts on
	// namespaces whose stream is fully drained; see RecoverySweeper docs.
	recoverySweeper := embedder.NewRecoverySweeper(embedderRepo, redisClient, nsConfigSvc, embedder.RecoverySweeperConfig{})

	// Liveness signal for the admin overview — without it that page has no
	// way to tell a running embedder from a dead one.
	heartbeat := embedder.NewHeartbeat(redisClient, consumerName)

	// Liveness + Prometheus metrics endpoint runs on a separate port from
	// cmd/api so production deployments can scrape both independently.
	healthSrv := newHealthServer(cfg.HealthPort, db, redisClient, qdrantClient)
	go func() {
		slog.Info("embedder health endpoint listening", "addr", ":"+cfg.HealthPort)
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("health server error", "error", err)
		}
	}()

	// Worker runs in its own goroutine; main goroutine watches for
	// SIGINT/SIGTERM and orchestrates graceful shutdown.
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- worker.Run(ctx)
	}()

	// Re-embed watcher runs alongside the worker; its lifecycle is bound to
	// the same context. Errors are logged inside Run; the goroutine exits
	// cleanly on context cancellation.
	watcherDone := make(chan error, 1)
	go func() {
		watcherDone <- reembedWatcher.Run(ctx)
	}()

	// Backlog sampler runs as a fire-and-forget goroutine — best-effort
	// observability writer. Returns when ctx is cancelled.
	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		backlogSampler.Run(ctx)
	}()

	sweeperDone := make(chan struct{})
	go func() {
		defer close(sweeperDone)
		recoverySweeper.Run(ctx)
	}()

	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		heartbeat.Run(ctx)
	}()

	slog.Info("embedder started",
		"consumer", consumerName,
		"poll_interval", cfg.NamespacePollInterval,
		"max_attempts", cfg.EmbedMaxAttempts,
	)

	quit := make(chan os.Signal, 1)
	signalNotifyFn(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-quit:
		slog.Info("embedder shutting down (signal)")
	case err := <-workerDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("worker exited with error", "error", err)
		}
		slog.Info("embedder shutting down (worker exit)")
	}

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("health server shutdown error", "error", err)
	}

	// Wait for the worker goroutine to drain (it may already have exited if
	// the trigger was workerDone above).
	select {
	case <-workerDone:
	case <-shutdownCtx.Done():
		slog.Warn("worker shutdown timed out")
	}

	// Wait for the watcher goroutine to drain (it returns nil on context cancel).
	select {
	case <-watcherDone:
	case <-shutdownCtx.Done():
		slog.Warn("reembed watcher shutdown timed out")
	}

	// Wait for the sampler goroutine to drain.
	select {
	case <-samplerDone:
	case <-shutdownCtx.Done():
		slog.Warn("backlog sampler shutdown timed out")
	}

	// Wait for the recovery sweeper goroutine to drain.
	select {
	case <-sweeperDone:
	case <-shutdownCtx.Done():
		slog.Warn("recovery sweeper shutdown timed out")
	}

	select {
	case <-heartbeatDone:
	case <-shutdownCtx.Done():
		slog.Warn("heartbeat shutdown timed out")
	}

	slog.Info("embedder stopped")
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

func newHealthServer(port string, db *pgxpool.Pool, rdb *goredis.Client, qdrantClient *qdrantpb.Client) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler(db, rdb, qdrantClient))
	mux.Handle("/metrics", promhttp.Handler())
	return &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
}

func healthzHandler(db *pgxpool.Pool, rdb *goredis.Client, qdrantClient *qdrantpb.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := map[string]string{
			"postgres": pingPostgres(ctx, db),
			"redis":    pingRedis(ctx, rdb),
			"qdrant":   pingQdrant(ctx, qdrantClient),
		}
		status := "ok"
		for _, v := range checks {
			if v != "ok" {
				status = "degraded"
				break
			}
		}
		checks["status"] = status
		code := http.StatusOK
		if status != "ok" {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if err := json.NewEncoder(w).Encode(checks); err != nil {
			slog.Warn("healthz encode failed", "error", err)
		}
	}
}

func pingPostgres(ctx context.Context, db *pgxpool.Pool) string {
	if db == nil {
		return "error: db nil"
	}
	if err := db.Ping(ctx); err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}

func pingRedis(ctx context.Context, rdb *goredis.Client) string {
	if rdb == nil {
		return "error: redis nil"
	}
	if err := rdb.Ping(ctx).Err(); err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}

func pingQdrant(ctx context.Context, client *qdrantpb.Client) string {
	if client == nil {
		return "error: qdrant nil"
	}
	if _, err := client.HealthCheck(ctx); err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}
