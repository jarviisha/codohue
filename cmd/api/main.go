package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	qdrantpb "github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/auth"
	"github.com/jarviisha/codohue/internal/catalog"
	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	infrapg "github.com/jarviisha/codohue/internal/infra/postgres"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/jarviisha/codohue/internal/ingest"
	"github.com/jarviisha/codohue/internal/nsconfig"
	"github.com/jarviisha/codohue/internal/objects"
	"github.com/jarviisha/codohue/internal/recommend"
)

var (
	loadConfigFn      = config.LoadAPI
	newPoolFn         = infrapg.NewPool
	newRedisFn        = infraredis.NewClient
	newQdrantFn       = infraqdrant.NewClient
	registerMetricsFn = metrics.Register
	signalNotifyFn    = signal.Notify
	hostnameFn        = os.Hostname
	closePoolFn       = func(db *pgxpool.Pool) { db.Close() }
	closeRedisFn      = func(client *goredis.Client) error { return client.Close() }
	checkPostgresFn   = checkPostgres
	checkRedisFn      = checkRedis
	checkQdrantFn     = checkQdrant
	dbPingFn          = func(ctx context.Context, db *pgxpool.Pool) error { return db.Ping(ctx) }
	redisPingRawFn    = func(ctx context.Context, rdb *goredis.Client) error { return rdb.Ping(ctx).Err() }
	qdrantHealthFn    = func(ctx context.Context, client *qdrantpb.Client) error {
		_, err := client.HealthCheck(ctx)
		if err != nil {
			return fmt.Errorf("qdrant health check: %w", err)
		}
		return nil
	}
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

	// Infrastructure
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

	// Core
	lifecycleRepo := nslifecycle.NewRepository(db)
	lifecycleLocker, err := nslifecycle.NewPostgresLocker(db)
	if err != nil {
		return fmt.Errorf("create lifecycle locker: %w", err)
	}
	defer lifecycleLocker.Close()
	lifecycleSvc := nslifecycle.NewService(lifecycleRepo, lifecycleLocker)
	idmapRepo := idmap.NewRepository(db)
	idmapSvc := idmap.NewService(idmapRepo)

	// nsconfig — read-only on the data plane (used for keyHashFn lookups).
	// Namespace mutation lives only on the admin plane (cmd/admin).
	nsConfigRepo := nsconfig.NewRepository(db)
	nsConfigSvc := nsconfig.NewService(nsConfigRepo)

	// ingest
	ingestRepo := ingest.NewRepository(db)
	ingestSvc := ingest.NewService(ingestRepo, nsConfigSvc)
	ingestSvc.SetLifecycleWriter(lifecycleSvc)
	// Live event tail: every processed event (HTTP or Redis-stream path) is
	// published best-effort to Redis pub/sub so cmd/admin's SSE tail can fan
	// it out. Async + drop-on-full → never back-pressures ingest.
	tailPublisher := ingest.NewRedisEventTailPublisher(redisClient, 0)
	ingestSvc.SetTailPublisher(tailPublisher)
	ingestHandler := ingest.NewHandler(ingestSvc)
	consumerName := resolveConsumerName(cfg.IngestReplicaName, hostnameFn)
	ingestWorker := ingest.NewWorker(redisClient, ingestSvc, consumerName)
	ingestWorker.SetLifecycleEvaluator(lifecycleSvc)

	if err := ingestWorker.Init(ctx); err != nil {
		return fmt.Errorf("init ingest worker: %w", err)
	}

	// catalog auto-embedding ingest path. Producer-only here in cmd/api;
	// the embedder consumer lives in cmd/embedder. The handler validates
	// the request, persists the row, and publishes to Redis Streams.
	catalogRepo := catalog.NewRepository(db)
	catalogSvc := catalog.NewService(catalogRepo, nsConfigSvc, redisClient)
	catalogSvc.SetLifecycleWriter(lifecycleSvc)
	catalogSvc.SetDefaultMaxContentBytes(cfg.CatalogMaxContentBytes)
	catalogHandler := catalog.NewHandler(catalogSvc)

	// per-object metadata, independent of embedding. Wired into catalog as an
	// interface so catalog never imports this peer domain directly.
	objectsSvc := objects.NewService(objects.NewRepository(db))
	objectsSvc.SetLifecycleWriter(lifecycleSvc)
	objectsHandler := objects.NewHandler(objectsSvc)
	catalogSvc.SetAuthorWriter(&objectAttributionAdapter{svc: objectsSvc})

	// Durable catalog transport: the ingest worker consumes codohue:catalog
	// and hands entries to catalog.Service through the adapter (the import
	// rule keeps ingest and catalog apart).
	catalogWorker := ingest.NewCatalogWorker(redisClient, &catalogStreamAdapter{svc: catalogSvc}, consumerName)
	catalogWorker.SetLifecycleEvaluator(lifecycleSvc)
	if err := catalogWorker.Init(ctx); err != nil {
		return fmt.Errorf("init catalog ingest worker: %w", err)
	}
	streamRetention := infraredis.NewRetention(redisClient, retentionDryRun(cfg.StreamRetentionEnabled))

	// recommend
	recommendRepo := recommend.NewRepository(db)
	recommendSvc := recommend.NewService(recommendRepo, nsConfigSvc, idmapSvc, qdrantClient, redisClient)
	recommendSvc.SetLifecycleWriter(lifecycleSvc)
	recommendSvc.SetObjectMetadataDeleter(objectsSvc)

	// keyHashFn bridges nsconfig.Service to auth.KeyHashFn without coupling packages.
	keyHashFn := func(ctx context.Context, namespace string) (string, error) {
		cfg, err := nsConfigSvc.Get(ctx, namespace)
		if err != nil {
			return "", fmt.Errorf("get ns config: %w", err)
		}
		if cfg == nil {
			return "", nil
		}
		return cfg.APIKeyHash, nil
	}
	recommendHandler := recommend.NewHandler(recommendSvc)

	// HTTP Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/ping", pingHandler())
	r.Get("/healthz", observabilityHealthHandler(cfg.ObservabilityToken,
		healthzHandler(db, redisClient, qdrantClient), healthzDetailsHandler(db, redisClient, qdrantClient)))
	r.Handle("/metrics", auth.RequireObservability(cfg.ObservabilityToken)(promhttp.Handler()))

	// All client-facing routes live under /v1/namespaces/{ns}/* and authenticate
	// via per-namespace bcrypt-hashed keys (with fallback to the global
	// CODOHUE_ADMIN_API_KEY when no namespace key is provisioned).
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireNamespace(cfg.AdminAPIKey, keyHashFn, func(r *http.Request) string {
			return chi.URLParam(r, "ns")
		}))
		r.Post("/v1/namespaces/{ns}/events", ingestHandler.Ingest)
		r.Post("/v1/namespaces/{ns}/catalog", catalogHandler.Ingest)
		r.Post("/v1/namespaces/{ns}/catalog/batch", catalogHandler.BatchIngest)
		r.Get("/v1/namespaces/{ns}/catalog/objects", catalogHandler.ListObjects)
		r.Get("/v1/namespaces/{ns}/subjects/{id}/recommendations", recommendHandler.GetSubjectRecommendations)
		r.Post("/v1/namespaces/{ns}/rankings", recommendHandler.Rank)
		r.Get("/v1/namespaces/{ns}/trending", recommendHandler.GetTrending)
		r.Put("/v1/namespaces/{ns}/objects/{id}", objectsHandler.Upsert)
		r.Put("/v1/namespaces/{ns}/objects/{id}/embedding", recommendHandler.StoreObjectEmbedding)
		r.Put("/v1/namespaces/{ns}/subjects/{id}/embedding", recommendHandler.StoreSubjectEmbedding)
		r.Delete("/v1/namespaces/{ns}/objects/{id}", recommendHandler.DeleteObject)
	})

	// Goroutines. Joined on shutdown before the deferred pool/redis closes
	// fire, so in-flight events finish their write + XACK against live clients.
	var workerWG sync.WaitGroup
	workerWG.Add(4)
	go func() {
		defer workerWG.Done()
		ingestWorker.Run(ctx)
	}()
	go func() {
		defer workerWG.Done()
		catalogWorker.Run(ctx)
	}()
	go func() {
		defer workerWG.Done()
		tailPublisher.Run(ctx)
	}()
	go func() {
		defer workerWG.Done()
		infraredis.RunRetentionLoop(ctx, cfg.StreamRetentionInterval, streamRetention, func(context.Context) ([]infraredis.StreamSpec, error) {
			return apiRetentionSpecs(), nil
		})
	}()

	// HTTP Server
	srv := &http.Server{
		Addr:         ":" + cfg.APIPort,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("API listening", "addr", ":"+cfg.APIPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signalNotifyFn(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down API")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	workerDone := make(chan struct{})
	go func() {
		workerWG.Wait()
		close(workerDone)
	}()
	select {
	case <-workerDone:
	case <-shutdownCtx.Done():
		slog.Warn("ingest worker shutdown timed out")
	}

	slog.Info("API stopped")
	return nil
}

func retentionDryRun(enabled bool) bool { return !enabled }

func apiRetentionSpecs() []infraredis.StreamSpec {
	return []infraredis.StreamSpec{
		{Name: "codohue:events", Kind: "events", ExpectedGroups: []string{"codohue-ingest"}},
		{Name: "codohue:catalog", Kind: "catalog", ExpectedGroups: []string{"codohue-catalog-ingest"}},
	}
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

// resolveConsumerName picks the ingest worker's Redis Streams consumer name:
// the configured replica name, else the OS hostname, else empty (the worker
// falls back to its own default). Extracted so the resolution is unit-tested
// rather than buried in run().
func resolveConsumerName(configured string, hostname func() (string, error)) string {
	if configured != "" {
		return configured
	}
	if h, err := hostname(); err == nil {
		return h
	}
	return ""
}

func pingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck // writing to ResponseWriter never returns a meaningful error
	}
}

func healthzHandler(db *pgxpool.Pool, rdb *goredis.Client, qdrantClient *qdrantpb.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := map[string]string{
			"postgres": checkPostgresFn(ctx, db),
			"redis":    checkRedisFn(ctx, rdb),
			"qdrant":   checkQdrantFn(ctx, qdrantClient),
		}

		status := "ok"
		for _, v := range checks {
			if v != "ok" {
				status = "degraded"
				break
			}
		}
		code := http.StatusOK
		if status != "ok" {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"status": status}) //nolint:errcheck // writing to ResponseWriter never returns a meaningful error
	}
}

func healthzDetailsHandler(db *pgxpool.Pool, rdb *goredis.Client, qdrantClient *qdrantpb.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		checks := map[string]string{
			"postgres": checkPostgresFn(ctx, db), "redis": checkRedisFn(ctx, rdb), "qdrant": checkQdrantFn(ctx, qdrantClient),
		}
		status := "ok"
		for _, value := range checks {
			if value != "ok" {
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
			slog.Warn("write health response failed", "error", err)
		}
	}
}

func observabilityHealthHandler(token string, public, detailed http.Handler) http.HandlerFunc {
	protected := auth.RequireObservability(token)(detailed)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("details") == "true" {
			protected.ServeHTTP(w, r)
			return
		}
		public.ServeHTTP(w, r)
	}
}

func checkPostgres(ctx context.Context, db *pgxpool.Pool) string {
	if err := dbPingFn(ctx, db); err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}

func checkRedis(ctx context.Context, rdb *goredis.Client) string {
	if err := redisPingRawFn(ctx, rdb); err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}

func checkQdrant(ctx context.Context, client *qdrantpb.Client) string {
	if err := qdrantHealthFn(ctx, client); err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}
