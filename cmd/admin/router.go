package main

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// newAdminRouter wires every admin HTTP route onto a fresh chi router. It is
// extracted from main.run() so cmd/admin/main_test.go can assert that all
// expected paths are registered without spinning up the full binary.
//
// apiKey is forwarded to the session-cookie middleware. allowDevOrigin enables
// credentialed CORS for the Vite dev server when non-empty (dev mode); empty
// in production where the SPA is embedded same-origin.
func newAdminRouter(h *admin.Handler, apiKey, allowDevOrigin string) chi.Router {
	r := chi.NewRouter()
	r.Use(admin.CORSMiddleware(allowDevOrigin))
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			httpapi.WriteError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		httpapi.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	})

	// Auth (sessions as a resource)
	r.Post("/api/v1/auth/sessions", h.CreateSession)

	// Protected admin API routes
	r.Group(func(r chi.Router) {
		r.Use(admin.RequireSession(apiKey))
		r.Delete("/api/v1/auth/sessions/current", h.DeleteCurrentSession)

		r.Get("/api/admin/v1/health", h.GetHealth)

		// SSE smoke endpoint — kept around as a low-cost end-to-end probe for
		// the streaming pipeline. Removeable once the SPA settles on real
		// stream sources.
		r.Get("/api/admin/v1/ping/stream", admin.PingStream)

		// Fleet aggregate — drives the Fleet overview page in one round-trip.
		r.Get("/api/admin/v1/overview", h.GetOverview)

		// Global ops bus — sidebar badges, toast notifications, etc.
		r.Get("/api/admin/v1/stream", h.StreamOps)

		// Namespaces
		r.Get("/api/admin/v1/namespaces", h.ListNamespaces)
		r.Get("/api/admin/v1/namespaces/{ns}", h.GetNamespace)
		r.Put("/api/admin/v1/namespaces/{ns}", h.UpsertNamespace)
		r.Delete("/api/admin/v1/namespaces/{ns}", h.DeleteNamespace)

		// Per-namespace dashboard aggregate.
		r.Get("/api/admin/v1/namespaces/{ns}/dashboard", h.GetNamespaceDashboard)

		// App-wide reset (danger zone).
		r.Post("/api/admin/v1/reset", h.ResetApp)

		// Catalog auto-embedding (feature 004) — per-namespace config.
		r.Get("/api/admin/v1/namespaces/{ns}/catalog", h.GetCatalogConfig)
		r.Put("/api/admin/v1/namespaces/{ns}/catalog", h.UpdateCatalogConfig)

		// Catalog auto-embedding — operator lifecycle endpoints (US3).
		r.Post("/api/admin/v1/namespaces/{ns}/catalog/re-embed", h.TriggerReEmbed)
		r.Get("/api/admin/v1/namespaces/{ns}/catalog/backlog-history", h.GetCatalogBacklogHistory)
		r.Get("/api/admin/v1/namespaces/{ns}/catalog/failures-summary", h.GetCatalogFailuresSummary)
		r.Get("/api/admin/v1/namespaces/{ns}/catalog/stream", h.StreamCatalog)
		r.Get("/api/admin/v1/namespaces/{ns}/catalog/items", h.ListCatalogItems)
		// Note: bulk redrive must be registered BEFORE the {id} variants so
		// chi does not parse "redrive-deadletter" as the id parameter.
		r.Post("/api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter", h.BulkRedriveDeadletter)
		r.Get("/api/admin/v1/namespaces/{ns}/catalog/items/{id}", h.GetCatalogItem)
		r.Post("/api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive", h.RedriveCatalogItem)
		r.Delete("/api/admin/v1/namespaces/{ns}/catalog/items/{id}", h.DeleteCatalogItem)

		// Batch runs — list / stats / detail / lifecycle / stream.
		// stats and {id} are siblings under /batch-runs; chi parses the static
		// "stats" before the {id} parameter, so registering order does not
		// matter, but keeping them together aids readability.
		r.Get("/api/admin/v1/batch-runs", h.GetBatchRuns)
		r.Get("/api/admin/v1/batch-runs/stats", h.GetBatchRunStats)
		r.Get("/api/admin/v1/batch-runs/{id}", h.GetBatchRunDetail)
		r.Get("/api/admin/v1/batch-runs/{id}/stream", h.StreamBatchRun)
		r.Post("/api/admin/v1/batch-runs/{id}/cancel", h.CancelBatchRun)
		r.Post("/api/admin/v1/batch-runs/{id}/retry", h.RetryBatchRun)
		r.Get("/api/admin/v1/namespaces/{ns}/batch-runs", h.GetBatchRuns)
		r.Post("/api/admin/v1/namespaces/{ns}/batch-runs", h.CreateBatchRun)

		// Qdrant inspection
		r.Get("/api/admin/v1/namespaces/{ns}/qdrant", h.GetQdrant)

		// Subjects
		r.Get("/api/admin/v1/namespaces/{ns}/subjects/{id}/profile", h.GetSubjectProfile)
		r.Get("/api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations", h.GetSubjectRecommendations)

		// Trending
		r.Get("/api/admin/v1/namespaces/{ns}/trending", h.GetTrending)

		// Events — list / live tail (SSE) / server-side summary / inject.
		// "stream" and "summary" are static siblings of the list path; chi
		// resolves them before any parameterised segment.
		r.Get("/api/admin/v1/namespaces/{ns}/events", h.GetRecentEvents)
		r.Get("/api/admin/v1/namespaces/{ns}/events/stream", h.StreamEvents)
		r.Get("/api/admin/v1/namespaces/{ns}/events/summary", h.GetEventsSummary)
		r.Post("/api/admin/v1/namespaces/{ns}/events", h.InjectEvent)

		// Curated rolling-window metrics for the fleet dashboard.
		r.Get("/api/admin/v1/metrics/summary", h.GetMetricsSummary)

		// Demo data
		r.Post("/api/admin/v1/demo-data", h.CreateDemoData)
		r.Delete("/api/admin/v1/demo-data", h.DeleteDemoData)
	})

	return r
}
