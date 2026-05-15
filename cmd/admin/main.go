package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/compute"
	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/idmap"

	// Side-effect import: internal/embedder.init() registers the V1 hashing
	// strategy with embedstrategy.DefaultRegistry, which the catalog admin
	// endpoints expose to operators via available_strategies.
	_ "github.com/jarviisha/codohue/internal/embedder"
	infrapg "github.com/jarviisha/codohue/internal/infra/postgres"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/jarviisha/codohue/internal/nsconfig"
	adminui "github.com/jarviisha/codohue/web/admin"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.LoadAdmin()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	initLogger(cfg.LogFormat)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := infrapg.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer db.Close()

	redisClient, err := infraredis.NewClient(cfg.RedisURL)
	if err != nil {
		slog.Warn("redis unavailable, TTL display will be disabled", "error", err)
		redisClient = nil
	}
	if redisClient != nil {
		defer redisClient.Close() //nolint:errcheck // best-effort cleanup on shutdown
	}

	qdrantClient, err := infraqdrant.NewClient(cfg.QdrantHost, cfg.QdrantPort)
	if err != nil {
		slog.Warn("qdrant unavailable, sparse vector NNZ will be disabled", "error", err)
		qdrantClient = nil
	}

	idmapRepo := idmap.NewRepository(db)
	idmapSvc := idmap.NewService(idmapRepo)

	nsConfigRepo := nsconfig.NewRepository(db)
	nsConfigSvc := nsconfig.NewService(nsConfigRepo)

	computeRepo := compute.NewRepository(db)
	computeSvc := compute.NewService(computeRepo, idmapSvc, qdrantClient)
	job := compute.NewJob(computeSvc, nsConfigSvc, computeRepo, qdrantClient, idmapSvc, redisClient, 5)

	repo := admin.NewRepository(db)
	nsAdapter := &nsConfigAdapter{svc: nsConfigSvc}
	svc := admin.NewService(repo, cfg.APIURL, cfg.AdminAPIKey, redisClient, qdrantClient, job, nsAdapter)

	// Catalog auto-embedding admin endpoints (US2). The adapter bridges
	// admin.Service → nsconfig.Service + embedstrategy.DefaultRegistry
	// without forcing internal/admin to import either directly.
	catalogAdapter := newCatalogConfigAdapter(nsConfigSvc, embedstrategy.DefaultRegistry())
	svc.SetCatalogConfigurator(catalogAdapter)
	svc.SetCatalogStrategyPicker(catalogAdapter)
	svc.SetCatalogBacklogReader(newCatalogBacklogAdapter(repo, redisClient))

	h := admin.NewHandler(svc, cfg.AdminAPIKey)

	r := newAdminRouter(h, cfg.AdminAPIKey)

	// Static file serving — React SPA embedded in the binary
	distFS, err := fs.Sub(adminui.Files, "dist")
	if err != nil {
		return fmt.Errorf("embed dist: %w", err)
	}
	fileServer := http.FileServer(http.FS(distFS))
	r.Handle("/assets/*", fileServer)
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, distFS, "index.html")
	})

	srv := &http.Server{
		Addr:         ":" + cfg.AdminPort,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("admin dashboard listening", "addr", ":"+cfg.AdminPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("admin server error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down admin")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("admin server shutdown error", "error", err)
	}

	slog.Info("admin stopped")
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
