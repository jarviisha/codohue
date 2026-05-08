# Implementation Plan: RESTful API Redesign

**Branch**: `003-restful-api-redesign` | **Date**: 2026-05-07 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/003-restful-api-redesign/spec.md`

## Summary

Consolidate Codohue's HTTP surface into a single canonical, resource-oriented REST API before production launch. Remove all legacy duplicate paths, eliminate RPC verbs (`/trigger`, `/debug`, `/login`, `/logout`, `/rank`, `/qdrant-stats`), move namespace mutation off the data plane to the admin plane only, adopt a sub-resource style for recommendations (`/v1/namespaces/{ns}/subjects/{id}/recommendations`), standardize on bare typed responses (no `{data: ...}` wrapper) with consistent status codes, and update the embedded admin web UI to call only the new paths.

The change is contained to the HTTP transport layer (route tables in `cmd/api/main.go` and `cmd/admin/main.go`, handler request/response DTOs in `internal/<domain>/handler.go` and `types.go`, auth middleware wiring) plus the admin web UI's API client (`web/admin/src/services/`). Domain logic (services, repositories, batch jobs, ingest worker, vector compute) is unchanged. Database schema is unchanged.

## Technical Context

**Language/Version**: Go 1.26.1
**Primary Dependencies**: `github.com/go-chi/chi/v5` v5.2.5 (router), `pgx/v5` (PostgreSQL), `redis/go-redis/v9`, `qdrant/go-client`, `prometheus/client_golang`. Frontend: React + TypeScript + Tailwind v4 (in `web/admin`).
**Storage**: PostgreSQL (no schema change), Redis (cache + Streams + trending ZSET, unchanged), Qdrant (vector store, unchanged).
**Testing**: Go `testing` stdlib + `httptest`; `make test` and `make test-pkg PKG=...`.
**Target Platform**: Linux server (two binaries: `cmd/api` port 2001, `cmd/admin` port 2002, plus `cmd/cron` daemon).
**Project Type**: Web service (Go backend) + embedded SPA (`web/admin` built into the admin binary via `embed.FS`).
**Performance Goals**: Unchanged. Recommend p95 stays under existing thresholds (Redis cache 5min TTL preserved, cold-start path preserved).
**Constraints**: No DB migration. No protocol break for the embedded admin UI mid-flight — frontend and backend ship together. The 500-item ranking cap is preserved.
**Scale/Scope**: ~6 routes consolidated (legacy → canonical), ~6 routes renamed (admin verb-suffix → resource), ~21 functional requirements, ~5 handler files modified, ~6 web/admin client files updated.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| **I. Code Quality** — domain layout, `docs.go`, import boundaries, English comments | ☑ | No new domains. Existing domain structure preserved. Handler refactors stay inside their package. Comments remain English. |
| **II. Testing Standards** — `_test.go` for every business-logic file | ☑ | Existing `handler_test.go` files in `recommend`, `ingest`, `nsconfig`, `admin` packages must be updated to cover renamed routes and DTO changes. No new business-logic file is added. |
| **III. API Consistency** — `/v1/<resource>` convention, uniform error JSON, two-tier auth, REST API table in CLAUDE.md updated | ☑ | This feature is the API consistency feature. Every change strengthens this gate. CLAUDE.md REST API table will be rewritten as part of the implementation tasks. |
| **IV. Performance** — Redis cache, batch phases non-blocking, cold-start fallback | ☑ | Transport-layer-only change; performance characteristics unchanged. No new hot-path code. |

> No ☒ violations. No Complexity Tracking entries required.

**Note on architecture constraint (two binaries):** The constitution prescribes "exactly two binaries: `cmd/api` and `cmd/cron`," but `cmd/admin` already exists in the repository (introduced by feature 002). This pre-existing condition is unchanged by feature 003 and is governed by feature 002, not by this plan.

## Project Structure

### Documentation (this feature)

```text
specs/003-restful-api-redesign/
├── plan.md              # This file
├── research.md          # Phase 0 — design decisions resolved
├── data-model.md        # Phase 1 — DTO changes (no DB change)
├── quickstart.md        # Phase 1 — end-to-end verification walkthrough
├── contracts/           # Phase 1 — endpoint contracts in Markdown
│   ├── data-plane.md
│   ├── admin-plane.md
│   └── auth-plane.md
├── checklists/
│   └── requirements.md  # From /speckit-specify
└── tasks.md             # From /speckit-tasks (NOT created by this command)
```

### Source Code (repository root)

```text
cmd/
├── api/main.go              # Data-plane router — rewrite the route table
└── admin/main.go            # Admin server router — rewrite the route table

internal/
├── ingest/
│   ├── handler.go           # Drop `Namespace` field handling from POST events DTO
│   ├── handler_test.go      # Update HTTP-level tests
│   └── types.go             # Remove `Namespace` from IngestRequest
├── recommend/
│   ├── handler.go           # Collapse Get/GetByNamespace into one handler reading path param;
│   │                        # remove deferred auth in Rank; rename embedding handlers to PUT
│   ├── handler_test.go      # Update tests for canonical paths only
│   └── types.go             # Drop `Namespace` from RankRequest
├── nsconfig/
│   ├── handler.go           # Move Upsert handler usage off cmd/api (still callable from cmd/admin)
│   └── handler_test.go      # Update test fixtures for admin-plane usage
└── admin/
    ├── handler.go           # Rename TriggerBatch → CreateBatchRun; DebugRecommend becomes
    │                        # query-mode of GetSubjectRecommendations; Login → CreateSession;
    │                        # Logout → DeleteCurrentSession; SeedDemoDataset/ClearDemoDataset
    │                        # bound to /demo-data
    ├── handler_test.go      # Update for renamed paths and shapes
    └── types.go             # Add per-shape DTOs as needed; remove namespace-in-body fields

web/admin/src/
├── services/
│   ├── api.ts               # Update all data-plane URL constructors
│   └── adminApi.ts          # Update all admin URL constructors (auth, batch-runs, qdrant, demo)
└── hooks/
    ├── useBatchRuns.ts      # Use POST .../batch-runs (no /trigger)
    ├── useQdrantStats.ts    # Rename to useQdrant or update path to .../qdrant
    └── useNamespacesOverview.ts  # Update if affected by query-param shifts

CLAUDE.md                    # Rewrite REST API tables to reflect the new canonical paths
```

**Structure Decision**: The redesign is an in-place refactor. No new packages or directories are created. Each handler package owns the changes to its own routes/DTOs. The admin web UI's `services/` layer is the single seam between TypeScript fetch calls and Go HTTP handlers, so all client-side changes funnel through two files (`api.ts`, `adminApi.ts`) plus a small number of hook files that hard-code paths.

## Complexity Tracking

> No constitutional violations introduced by this plan. No entries.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| — | — | — |
