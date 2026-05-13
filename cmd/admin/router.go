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
// apiKey is forwarded to the session-cookie middleware.
func newAdminRouter(h *admin.Handler, apiKey string) chi.Router {
	r := chi.NewRouter()
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

		// Namespaces
		r.Get("/api/admin/v1/namespaces", h.ListNamespaces)
		r.Get("/api/admin/v1/namespaces/{ns}", h.GetNamespace)
		r.Put("/api/admin/v1/namespaces/{ns}", h.UpsertNamespace)

		// Catalog auto-embedding (feature 004) — per-namespace config.
		r.Get("/api/admin/v1/namespaces/{ns}/catalog", h.GetCatalogConfig)
		r.Put("/api/admin/v1/namespaces/{ns}/catalog", h.UpdateCatalogConfig)

		// Catalog auto-embedding — operator lifecycle endpoints (US3).
		r.Post("/api/admin/v1/namespaces/{ns}/catalog/re-embed", h.TriggerReEmbed)
		r.Get("/api/admin/v1/namespaces/{ns}/catalog/items", h.ListCatalogItems)
		// Note: bulk redrive must be registered BEFORE the {id} variants so
		// chi does not parse "redrive-deadletter" as the id parameter.
		r.Post("/api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter", h.BulkRedriveDeadletter)
		r.Get("/api/admin/v1/namespaces/{ns}/catalog/items/{id}", h.GetCatalogItem)
		r.Post("/api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive", h.RedriveCatalogItem)
		r.Delete("/api/admin/v1/namespaces/{ns}/catalog/items/{id}", h.DeleteCatalogItem)

		// Batch runs
		r.Get("/api/admin/v1/batch-runs", h.GetBatchRuns)
		r.Get("/api/admin/v1/namespaces/{ns}/batch-runs", h.GetBatchRuns)
		r.Post("/api/admin/v1/namespaces/{ns}/batch-runs", h.CreateBatchRun)

		// Qdrant inspection
		r.Get("/api/admin/v1/namespaces/{ns}/qdrant", h.GetQdrant)

		// Subjects
		r.Get("/api/admin/v1/namespaces/{ns}/subjects/{id}/profile", h.GetSubjectProfile)
		r.Get("/api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations", h.GetSubjectRecommendations)

		// Trending
		r.Get("/api/admin/v1/namespaces/{ns}/trending", h.GetTrending)

		// Events
		r.Get("/api/admin/v1/namespaces/{ns}/events", h.GetRecentEvents)
		r.Post("/api/admin/v1/namespaces/{ns}/events", h.InjectEvent)

		// Demo data
		r.Post("/api/admin/v1/demo-data", h.CreateDemoData)
		r.Delete("/api/admin/v1/demo-data", h.DeleteDemoData)
	})

	return r
}
