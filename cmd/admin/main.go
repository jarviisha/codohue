package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/admin/eventbus"
	"github.com/jarviisha/codohue/internal/compute"
	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/infra/metrics"

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

	// Wire the admin-plane event bus. Batch-run and catalog events arrive
	// over Redis pub/sub (cron and the embedder are separate processes);
	// SSE handlers subscribe with run-id / namespace filters.
	// Hook the bus's optional callbacks into the admin self-observability
	// collectors so Grafana can chart publish rate, subscriber gauge, and
	// backpressure drops without poking into bus internals.
	bus := eventbus.NewBus(
		eventbus.WithPublishCallback(func(kind string) {
			metrics.AdminEventbusPublishTotal.WithLabelValues(kind).Inc()
		}),
		eventbus.WithSubscribeCallback(func() {
			metrics.AdminEventbusSubscribersActive.Inc()
		}),
		eventbus.WithUnsubscribeCallback(func() {
			metrics.AdminEventbusSubscribersActive.Dec()
		}),
		eventbus.WithDropCallback(func(e eventbus.Event) {
			// Backpressure drops don't carry the receiving stream name —
			// we attribute by the event kind's prefix so dashboards can
			// still slice (ops/batch_run/catalog).
			metrics.AdminSSEDroppedTotal.WithLabelValues(streamLabelForKind(e.Kind), "backpressure").Inc()
		}),
	)
	defer bus.Close()
	// Runs publish to Redis so every admin replica — and streams for runs
	// started by cron — see them. Without Redis we fall back to the
	// in-process observer, which only covers runs this process started.
	if redisClient != nil {
		job.SetObserver(compute.NewRedisBatchRunObserver(redisClient))
	} else {
		job.SetObserver(newBatchRunObserverAdapter(bus))
	}

	// Catalog events bridge — subscribes to the Redis pub/sub channel
	// cmd/embedder publishes item state changes onto and republishes each
	// message as a `catalog.item_state_changed` event on the local bus.
	// Goroutine lifecycle is bound to ctx; bus shutdown happens via defer.
	//
	// Events-tail bridge — same shape for ingest: cmd/api publishes every
	// processed event to `codohue:events-tail:*`; this republishes them as
	// `events.ingested` on the bus (driving the live tail SSE) and feeds the
	// per-namespace rate tracker that backs fleet events/min + /metrics/summary.
	eventRate := admin.NewEventRateTracker()
	if redisClient != nil {
		bridge := newCatalogEventsBridge(redisClient, bus)
		go bridge.Run(ctx)
		runBridge := newBatchRunEventsBridge(redisClient, bus)
		go runBridge.Run(ctx)

		eventsBridge := newEventsTailBridge(redisClient, bus, eventRate)
		go eventsBridge.Run(ctx)
		go eventRate.Run(ctx, 10*time.Second)
	}

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
	svc.SetEventRateTracker(eventRate)

	// Session tokens are signed with independent random material — never the
	// API key. No pinned secret means a fresh one per boot: every restart
	// logs all operators out, which multi-replica deployments avoid by
	// setting CODOHUE_ADMIN_SESSION_SECRET.
	sessions, err := admin.NewSessionManager([]byte(cfg.SessionSecret))
	if err != nil {
		return fmt.Errorf("init session manager: %w", err)
	}
	if cfg.SessionSecret == "" {
		slog.Info("admin session secret generated for this boot; restarts invalidate sessions (set CODOHUE_ADMIN_SESSION_SECRET to pin)")
	}

	h := admin.NewHandler(svc, cfg.AdminAPIKey, sessions)
	h.SetEventBus(bus)

	r := newAdminRouter(h, sessions, cfg.AllowDevOrigin)

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
		Addr:    ":" + cfg.AdminPort,
		Handler: r,
		// ReadTimeout is fine for SSE — the handshake completes well within
		// it and SSE is one-way (server → client) after that. WriteTimeout
		// stays out of the struct because it's a fixed deadline from request
		// start; SSE handlers opt out of it per-connection via
		// http.NewResponseController inside sse.NewWriter. Non-SSE handlers
		// rely on chi's request-level timeout middleware (if any) plus
		// shutdownCtx below.
		ReadTimeout: 10 * time.Second,
		// BaseContext ties every request context to the app root ctx, so a
		// cancel() on shutdown propagates straight into in-flight SSE
		// handlers' r.Context().Done() select arms — without this, Shutdown
		// would block on the full shutdownCtx timeout because long-lived
		// SSE handlers never see the app stopping.
		BaseContext: func(_ net.Listener) context.Context { return ctx },
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

// streamLabelForKind maps an eventbus kind to the same stream label the SSE
// handlers expose on the connection gauge. Used by the drop callback to keep
// backpressure drops sliceable by stream in Grafana.
func streamLabelForKind(kind string) string {
	switch {
	case len(kind) > len("batch_run.") && kind[:len("batch_run.")] == "batch_run.":
		// "batch_run.started" / "batch_run.completed" / "batch_run.cancelled"
		// are fanned out on both /stream (ops) and /batch-runs/{id}/stream
		// (batch_run). Without a per-subscriber tag we can't tell which side
		// dropped — attribute to "ops" because every run lifecycle event
		// fans out there.
		return "ops"
	case len(kind) > len("catalog.") && kind[:len("catalog.")] == "catalog.":
		return "catalog"
	case len(kind) > len("events.") && kind[:len("events.")] == "events.":
		return "events"
	default:
		return "unknown"
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
