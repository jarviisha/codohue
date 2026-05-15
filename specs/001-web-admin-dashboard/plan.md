# Implementation Plan: Web Admin Dashboard

**Branch**: `001-web-admin-dashboard` | **Date**: 2026-04-28 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/001-web-admin-dashboard/spec.md`

## Summary

Add a standalone web admin dashboard (`cmd/admin`) that gives operators a browser-based control plane for the Codohue recommendation engine. The dashboard serves a React SPA embedded in a Go binary on port 2002. It covers five capability areas: system health, namespace config management, recommendation debugger, batch job history, and trending inspector. A new `batch_run_logs` PostgreSQL table is introduced so cron run history is queryable. All admin API calls to `cmd/api` use `CODOHUE_ADMIN_API_KEY` as the global fallback auth token.

## Technical Context

**Language/Version**: Go 1.26.1 (backend), React 18 + Vite 5 (frontend)
**Primary Dependencies**: chi router (existing), `embed.FS` (stdlib), pgxpool (existing), React, TanStack Query v5, React Router v6
**Storage**: PostgreSQL — new `batch_run_logs` table; existing `namespace_configs` table (read); Redis — `TTL trending:{ns}` for cache TTL display
**Testing**: Go standard `testing` package + `httptest` (backend); Vitest + React Testing Library (frontend)
**Target Platform**: Linux server (Docker Compose); modern desktop browsers (Chrome 120+, Firefox 120+, Safari 17+)
**Project Type**: web-service (Go) + web-application (React SPA)
**Performance Goals**: dashboard page load <2s on LAN; health status refresh <500ms; recommendation debug response reflects `cmd/api` latency (<1s p95)
**Constraints**: `cmd/admin` MUST NOT be on the recommendation hot path; operator sessions expire after 8 hours; the admin binary shares the PostgreSQL pool config with `cmd/api` but runs as a fully independent process

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| **I. Code Quality** — domain in `internal/<domain>/`, `docs.go` present, import boundaries respected, English-only comments | ✅ | New `internal/admin/` domain; `docs.go` planned; no cross-domain imports |
| **II. Testing Standards** — `_test.go` planned for every `service.go`, `repository.go`, `job.go`, `worker.go` | ✅ | `service_test.go`, `repository_test.go`, `handler_test.go` planned |
| **III. API Consistency** — endpoints follow `/v1/<resource>`, two-tier auth, REST API table in CLAUDE.md updated | ✅ | Admin API uses `/api/admin/v1/` prefix (distinct control-plane path); auth via session cookie backed by `CODOHUE_ADMIN_API_KEY`; CLAUDE.md REST table will be updated |
| **IV. Performance** — Redis cache plan in place, batch phases non-blocking, cold-start fallback accounted for | ✅ | Admin is not on the recommendation hot path; no caching layer needed for admin reads |

> Any ☒ violation requires a Complexity Tracking entry below.

## Project Structure

### Documentation (this feature)

```text
specs/001-web-admin-dashboard/
├── plan.md              ← this file
├── research.md          ← Phase 0 output
├── data-model.md        ← Phase 1 output
├── quickstart.md        ← Phase 1 output
├── contracts/
│   └── admin-api.md     ← Phase 1 output
└── tasks.md             ← Phase 2 output (/speckit-tasks — NOT created by /speckit-plan)
```

### Source Code (repository root)

```text
cmd/
  admin/
    main.go              ← binary entry point; wires chi router + embed.FS
internal/
  admin/
    docs.go              ← package doc comment
    handler.go           ← HTTP handlers for admin API routes
    handler_test.go      ← handler tests using httptest
    service.go           ← business logic (session validation, proxy calls, namespace reads)
    service_test.go      ← service unit tests
    repository.go        ← PostgreSQL reads (namespace_configs, batch_run_logs)
    repository_test.go   ← repository tests
    types.go             ← request/response types for admin API
web/
  admin/                 ← React source (Vite project root)
    index.html
    vite.config.ts
    package.json
    src/
      main.tsx
      App.tsx
      pages/
        HealthPage.tsx
        NamespacesPage.tsx
        NamespaceDetailPage.tsx
        RecommendDebugPage.tsx
        BatchRunsPage.tsx
        TrendingPage.tsx
      components/
      services/          ← API client functions (typed fetch wrappers)
      hooks/             ← TanStack Query hooks
    dist/                ← Vite build output (gitignored, embedded at compile time)
migrations/
  006_batch_run_logs.up.sql
  006_batch_run_logs.down.sql
```

**Structure Decision**: Web application layout with Go backend binary (`cmd/admin`) and React frontend (`web/admin/`). The compiled React `dist/` is embedded into the Go binary via `//go:embed web/admin/dist`. This keeps deployment as a single binary artifact with no runtime static file dependencies.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|--------------------------------------|
| Third binary (`cmd/admin`) — constitution requires exactly two binaries | Admin dashboard is a distinct control-plane service with its own port, static asset serving, React build pipeline, and a different security posture than the data-plane API. It must be independently deployable and restartable without touching `cmd/api`. | Embedding admin in `cmd/api` would mix control-plane and data-plane concerns on the same port, making network-level access control (VPN, firewall rules) impossible to apply selectively. It also creates a single point of failure: an admin UI bug could crash the recommendation API. |
