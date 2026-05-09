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
		_ = json.NewEncoder(w).Encode(checks)
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
