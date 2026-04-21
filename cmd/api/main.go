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
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	qdrantpb "github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/auth"
	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	infrapg "github.com/jarviisha/codohue/internal/infra/postgres"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/jarviisha/codohue/internal/ingest"
	"github.com/jarviisha/codohue/internal/nsconfig"
	"github.com/jarviisha/codohue/internal/recommend"
)

var (
	loadConfigFn      = config.Load
	newPoolFn         = infrapg.NewPool
	newRedisFn        = infraredis.NewClient
	newQdrantFn       = infraqdrant.NewClient
	registerMetricsFn = metrics.Register
	signalNotifyFn    = signal.Notify
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
	idmapRepo := idmap.NewRepository(db)
	idmapSvc := idmap.NewService(idmapRepo)

	// nsconfig
	nsConfigRepo := nsconfig.NewRepository(db)
	nsConfigSvc := nsconfig.NewService(nsConfigRepo)
	nsConfigHandler := nsconfig.NewHandler(nsConfigSvc)

	// ingest
	ingestRepo := ingest.NewRepository(db)
	ingestSvc := ingest.NewService(ingestRepo, nsConfigSvc)
	ingestHandler := ingest.NewHandler(ingestSvc)
	ingestWorker := ingest.NewWorker(redisClient, ingestSvc)

	if err := ingestWorker.Init(ctx); err != nil {
		return fmt.Errorf("init ingest worker: %w", err)
	}

	// recommend
	recommendRepo := recommend.NewRepository(db)
	recommendSvc := recommend.NewService(recommendRepo, nsConfigSvc, idmapSvc, qdrantClient, redisClient)

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
	validateKey := func(ctx context.Context, token, namespace string) bool {
		return auth.ValidateNamespaceKey(ctx, token, cfg.RecommenderAPIKey, keyHashFn, namespace)
	}
	recommendHandler := recommend.NewHandler(recommendSvc, validateKey)

	// HTTP Router
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/ping", pingHandler())
	r.Get("/healthz", healthzHandler(db, redisClient, qdrantClient))
	r.Handle("/metrics", promhttp.Handler())

	// Admin-only routes: protected by the global RECOMMENDER_API_KEY.
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAdmin(cfg.RecommenderAPIKey))
		r.Put("/v1/config/namespaces/{namespace}", nsConfigHandler.Upsert)
	})

	// Namespace-scoped routes: protected by per-namespace API keys.
	// Falls back to global key when a namespace has not been provisioned with its own key.
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireNamespace(cfg.RecommenderAPIKey, keyHashFn, func(r *http.Request) string {
			return r.URL.Query().Get("namespace")
		}))
		r.Get("/v1/recommendations", recommendHandler.Get)
	})

	// POST /v1/rank: namespace is inside the request body, so auth is handled in the handler
	// after the body is parsed.
	r.Group(func(r chi.Router) {
		r.Post("/v1/rank", recommendHandler.Rank)
	})

	// BYOE + trending endpoints: namespace is a URL path param {ns}.
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireNamespace(cfg.RecommenderAPIKey, keyHashFn, func(r *http.Request) string {
			return chi.URLParam(r, "ns")
		}))
		r.Get("/v1/trending/{ns}", recommendHandler.GetTrending)
		r.Post("/v1/objects/{ns}/{id}/embedding", recommendHandler.StoreObjectEmbedding)
		r.Post("/v1/subjects/{ns}/{id}/embedding", recommendHandler.StoreSubjectEmbedding)
		r.Delete("/v1/objects/{ns}/{id}", recommendHandler.DeleteObject)
		r.Post("/v1/namespaces/{ns}/events", ingestHandler.Ingest)
		r.Get("/v1/namespaces/{ns}/recommendations", recommendHandler.GetByNamespace)
		r.Post("/v1/namespaces/{ns}/rank", recommendHandler.RankByNamespace)
		r.Get("/v1/namespaces/{ns}/trending", recommendHandler.GetTrending)
		r.Post("/v1/namespaces/{ns}/objects/{id}/embedding", recommendHandler.StoreObjectEmbedding)
		r.Post("/v1/namespaces/{ns}/subjects/{id}/embedding", recommendHandler.StoreSubjectEmbedding)
		r.Delete("/v1/namespaces/{ns}/objects/{id}", recommendHandler.DeleteObject)
	})

	// Goroutines
	go ingestWorker.Run(ctx)

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

	slog.Info("API stopped")
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
		checks["status"] = status

		code := http.StatusOK
		if status != "ok" {
			code = http.StatusServiceUnavailable
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(checks) //nolint:errcheck // writing to ResponseWriter never returns a meaningful error
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
