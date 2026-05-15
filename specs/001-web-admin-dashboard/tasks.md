# Tasks: Web Admin Dashboard

**Input**: Design documents from `specs/001-web-admin-dashboard/`
**Prerequisites**: plan.md ✅ spec.md ✅ research.md ✅ data-model.md ✅ contracts/admin-api.md ✅ quickstart.md ✅

**Organization**: Tasks grouped by user story. Each story is independently implementable and testable.
**Tests**: Unit/handler tests included per Constitution Gate II (every service.go, repository.go, handler.go must have a _test.go).

## Format: `[ID] [P?] [Story?] Description with file path`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Directory structure, tooling, and build pipeline initialization.

- [x] T001 Create directory structure: `cmd/admin/`, `internal/admin/`, `web/admin/`, update `.gitignore` to exclude `web/admin/dist/` and `web/admin/node_modules/`
- [x] T002 Initialize Vite + React + TypeScript project in `web/admin/` (`npm create vite@latest . -- --template react-ts`), add TanStack Query v5 and React Router v6 dependencies
- [x] T003 [P] Add `make build-admin`, `make run-admin` targets to `Makefile`; `build-admin` runs `npm run build` in `web/admin/` then `go build ./cmd/admin`
- [x] T004 [P] Add `admin` service to `docker-compose.yml` (port 2002, depends on `api`, env vars: `DATABASE_URL`, `REDIS_URL`, `RECOMMENDER_API_KEY`, `CODOHUE_API_URL`, `CODOHUE_ADMIN_PORT`)
- [x] T005 [P] Add `CODOHUE_ADMIN_PORT` and `CODOHUE_API_URL` variables to `.env.example`
- [x] T006 [P] Configure Vite proxy in `web/admin/vite.config.ts` to forward `/api/*` to `http://localhost:2002` during development

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core backend wiring and React shell that every user story depends on. Must be complete before any story work begins.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [x] T007 Write migration `migrations/006_batch_run_logs.up.sql` (table + index per data-model.md) and `migrations/006_batch_run_logs.down.sql` (`DROP TABLE batch_run_logs`)
- [x] T008 Create `internal/admin/docs.go` with `// Package admin implements the HTTP handlers, service logic, and repository layer for the web admin dashboard.`
- [x] T009 Create `internal/admin/types.go` with all request/response types: `NamespaceConfig`, `BatchRunLog`, `TrendingAdminEntry`, `LoginRequest`, `RecommendDebugRequest`, `RecommendDebugResponse`, `NamespaceUpsertRequest`, `BatchRunsResponse` (per contracts/admin-api.md and data-model.md)
- [x] T010 Create `cmd/admin/main.go`: load env config (`DATABASE_URL`, `RECOMMENDER_API_KEY`, `CODOHUE_API_URL`, `CODOHUE_ADMIN_PORT`), initialize pgxpool, initialize chi router, mount `//go:embed web/admin/dist` static file server, wire admin API routes, listen on `CODOHUE_ADMIN_PORT`
- [x] T011 Add `internal/admin/middleware.go`: session JWT validation middleware using HMAC-SHA256 signed cookie (`codohue_admin_session`); apply to all `/api/admin/v1/*` routes
- [x] T012 Add `POST /api/auth/login` and `DELETE /api/auth/logout` handlers in `internal/admin/handler.go`; login validates submitted `api_key` against `RECOMMENDER_API_KEY` env var, issues HTTP-only signed JWT cookie on success
- [x] T013 Create `internal/admin/handler_test.go` with test helpers: `fakeSvc` interface stub, `newTestHandler()` constructor, `assertJSON()` helper
- [x] T014 Add handler tests for auth routes: `TestLoginSuccess`, `TestLoginWrongKey`, `TestLogout`, `TestProtectedRouteWithoutSession` in `internal/admin/handler_test.go`
- [x] T015 Create `internal/admin/repository.go`: initialize `pgxpool.Pool` field, add `// Repository holds prepared queries against PostgreSQL.` struct doc
- [x] T016 Create `internal/admin/repository_test.go` with `newTestRepo()` helper using a test DB connection (consistent with existing repo test patterns in `internal/recommend/`)
- [x] T017 Create `internal/admin/service.go`: struct with `repo *Repository`, `apiURL string`, `apiKey string`, and an `http.Client` for proxying calls to `cmd/api`; add `// Service implements admin business logic` struct doc
- [x] T018 Create `internal/admin/service_test.go` with `fakeRepo` stub and `newTestService()` constructor
- [x] T019 Scaffold React app shell in `web/admin/src/`: `main.tsx` (ReactDOM + QueryClientProvider + Router), `App.tsx` (route definitions for all 5 pages + login redirect), `services/api.ts` (typed fetch wrapper that reads session cookie and handles 401 redirects), `pages/LoginPage.tsx` (API key input form)
- [x] T020 Add `web/admin/src/components/Layout.tsx` (sidebar nav with links to Health, Namespaces, Debug, Batch Runs, Trending) and `components/ErrorBanner.tsx` (reusable error display)

**Checkpoint**: Foundation ready. Run `make build-admin` → binary compiles. Open `http://localhost:2002` → login page appears. Authenticated requests to `/api/admin/v1/*` return 401 without session, proceed with session.

---

## Phase 3: User Story 1 — System Health At a Glance (Priority: P1) 🎯 MVP

**Goal**: Operator sees real-time health status of PostgreSQL, Redis, and Qdrant from the dashboard home page.

**Independent Test**: Navigate to `http://localhost:2002/health` after login. Three dependency status cards appear. Bring Redis down → Redis card turns red, overall status shows "degraded". Verify without any other dashboard page existing.

- [x] T021 [US1] Add `GetHealth` method to `internal/admin/service.go`: proxies `GET <CODOHUE_API_URL>/healthz` using admin's `http.Client`, returns parsed health JSON
- [x] T022 [US1] Add `GET /api/admin/v1/health` handler in `internal/admin/handler.go` calling `svc.GetHealth`; forward non-200 status codes from `cmd/api` faithfully
- [x] T023 [US1] Add `TestGetHealth_OK` and `TestGetHealth_Degraded` in `internal/admin/handler_test.go` using `httptest.NewServer` to fake the `cmd/api` response
- [x] T024 [P] [US1] Create `web/admin/src/hooks/useHealth.ts`: TanStack Query hook polling `GET /api/admin/v1/health` every 10 seconds
- [x] T025 [P] [US1] Create `web/admin/src/pages/HealthPage.tsx`: renders three `StatusCard` components (postgres, redis, qdrant) with green/yellow/red indicator and last-checked timestamp; shows overall status banner
- [x] T026 [US1] Create `web/admin/src/components/StatusCard.tsx`: reusable card showing dependency name, status string, and colour-coded indicator dot

**Checkpoint**: US1 complete. Health page works independently. `make test` passes for `internal/admin`.

---

## Phase 4: User Story 2 — Namespace Configuration Management (Priority: P2)

**Goal**: Operator can list all namespaces, view config detail, create new namespaces, and update existing ones via a form UI.

**Independent Test**: Create a new namespace via the dashboard form → it appears in the namespace list. Update `lambda` on an existing namespace → the detail view reflects the new value. No other dashboard pages need to be functional.

- [x] T027 [US2] Add `ListNamespaces` and `GetNamespace` methods to `internal/admin/repository.go`: query `namespace_configs` table (columns per data-model.md); `HasAPIKey` maps `api_key_hash IS NOT NULL`
- [x] T028 [US2] Add repository tests `TestListNamespaces` and `TestGetNamespace_NotFound` in `internal/admin/repository_test.go`
- [x] T029 [US2] Add `ListNamespaces`, `GetNamespace`, `UpsertNamespace` methods to `internal/admin/service.go`; `UpsertNamespace` proxies to `cmd/api PUT /v1/config/namespaces/{ns}` with `Authorization: Bearer <apiKey>` and returns the response (including `api_key` field on first create)
- [x] T030 [US2] Add service tests `TestUpsertNamespace_Proxy` and `TestListNamespaces` in `internal/admin/service_test.go` using `httptest.NewServer` as a fake `cmd/api`
- [x] T031 [US2] Add `GET /api/admin/v1/namespaces` handler in `internal/admin/handler.go` calling `svc.ListNamespaces`
- [x] T032 [US2] Add `GET /api/admin/v1/namespaces/{ns}` handler in `internal/admin/handler.go` calling `svc.GetNamespace`; return 404 for unknown namespace
- [x] T033 [US2] Add `PUT /api/admin/v1/namespaces/{ns}` handler in `internal/admin/handler.go` calling `svc.UpsertNamespace`; validate request body fields before proxying
- [x] T034 [US2] Add handler tests `TestListNamespaces_Handler`, `TestGetNamespace_NotFound`, `TestUpsertNamespace_NewKey`, `TestUpsertNamespace_ExistingNoKey` in `internal/admin/handler_test.go`
- [x] T035 [P] [US2] Create `web/admin/src/hooks/useNamespaces.ts`: TanStack Query hooks `useNamespaceList()`, `useNamespace(ns)`, `useUpsertNamespace()` mutation
- [x] T036 [P] [US2] Create `web/admin/src/pages/NamespacesPage.tsx`: sortable table of all namespaces showing `namespace`, `dense_strategy`, `max_results`, `updated_at`, link to detail; "Create Namespace" button opens `NamespaceDetailPage` in create mode
- [x] T037 [US2] Create `web/admin/src/pages/NamespaceDetailPage.tsx`: form with all config fields (action_weights as key-value pairs, numeric fields with range validation); shows API key in a dismissable alert on first create only; "Save" calls upsert mutation

**Checkpoint**: US2 complete. Namespace list and create/update form work independently of other pages.

---

## Phase 5: User Story 3 — Recommendation Debugger (Priority: P2)

**Goal**: Operator enters a namespace and subject ID, submits, and sees the ranked recommendation list with scores, ranks, and source strategy.

**Independent Test**: Enter `namespace=darkvoid_feed`, `subject_id=user-123`, `limit=5` → recommendation list appears with `object_id`, `score`, `rank`, and `source`. Enter an unknown namespace → error message shown.

- [x] T038 [US3] Add `DebugRecommend` method to `internal/admin/service.go`: proxies to `cmd/api GET /v1/namespaces/{ns}/recommendations?subject_id=&limit=&offset=` using global API key; maps `codohuetypes.Response` to `RecommendDebugResponse`
- [x] T039 [US3] Add service test `TestDebugRecommend_Proxy` in `internal/admin/service_test.go` using `httptest.NewServer`; test 404 passthrough for unknown namespace
- [x] T040 [US3] Add `POST /api/admin/v1/recommend/debug` handler in `internal/admin/handler.go` calling `svc.DebugRecommend`; validate `namespace` and `subject_id` present in body
- [x] T041 [US3] Add handler tests `TestDebugRecommend_OK`, `TestDebugRecommend_MissingFields`, `TestDebugRecommend_NamespaceNotFound` in `internal/admin/handler_test.go`
- [x] T042 [P] [US3] Create `web/admin/src/hooks/useRecommendDebug.ts`: TanStack Query mutation for `POST /api/admin/v1/recommend/debug`
- [x] T043 [P] [US3] Create `web/admin/src/pages/RecommendDebugPage.tsx`: form with namespace (dropdown populated from namespace list), subject_id input, limit selector (5/10/20/50); on submit shows a ranked table of items with object_id, rank, score, and a source strategy badge; shows clear error for unknown namespace or no results

**Checkpoint**: US3 complete. Recommendation debugger works independently. Operators no longer need curl to test recommendation quality.

---

## Phase 6: User Story 4 — Batch Job & Metrics Overview (Priority: P3)

**Goal**: Operator sees recent batch run history per namespace with timestamp, subjects processed, duration, and success/failure status.

**Independent Test**: After `make run-cron` runs at least once, the batch runs page shows at least one row per namespace with `started_at`, `subjects_processed`, `duration_ms`, and `success=true`. A failed run shows the error message. Works without the recommendation debugger or trending pages.

- [x] T044 [US4] Extend `cmd/cron/main.go` (or `internal/compute/job.go`): at the start of each namespace batch cycle, INSERT a `batch_run_logs` row (`success=false`, `completed_at=null`); on cycle completion, UPDATE with final values; on failure, UPDATE with `success=false`, `error_message`
- [x] T045 [US4] Add `GetBatchRunLogs` method to `internal/admin/repository.go`: query `batch_run_logs ORDER BY started_at DESC LIMIT $1` with optional `namespace` filter; cap at 50 rows
- [x] T046 [US4] Add repository test `TestGetBatchRunLogs` in `internal/admin/repository_test.go`
- [x] T047 [US4] Add `GetBatchRuns` method to `internal/admin/service.go`
- [x] T048 [US4] Add `GET /api/admin/v1/batch-runs` handler in `internal/admin/handler.go`; accept `?namespace=` and `?limit=` (cap at 50) query params
- [x] T049 [US4] Add handler tests `TestGetBatchRuns_All`, `TestGetBatchRuns_FilteredByNamespace`, `TestGetBatchRuns_LimitCapped` in `internal/admin/handler_test.go`
- [x] T050 [P] [US4] Create `web/admin/src/hooks/useBatchRuns.ts`: TanStack Query hook for `GET /api/admin/v1/batch-runs`; refetch every 30 seconds
- [x] T051 [P] [US4] Create `web/admin/src/pages/BatchRunsPage.tsx`: namespace filter dropdown, table of recent runs showing `started_at`, `duration_ms`, `subjects_processed`, success badge (green tick / red X), `error_message` expandable on failure; "No runs yet" empty state

**Checkpoint**: US4 complete. Batch history page works. Cron writes are verified by running `make run-cron` once.

---

## Phase 7: User Story 5 — Trending Items Viewer (Priority: P3)

**Goal**: Operator browses current trending items for a namespace with scores and Redis cache TTL remaining.

**Independent Test**: After cron populates trending cache, select namespace `darkvoid_feed` on the trending page → items appear with scores and a TTL countdown. Empty cache state shows "No trending data" message.

- [x] T052 [US5] Add `GetTrending` method to `internal/admin/service.go`: proxies to `cmd/api GET /v1/namespaces/{ns}/trending?limit=&offset=&window_hours=`, then calls `redis.Client.TTL("trending:{ns}")` to get cache TTL; merges into `TrendingAdminEntry` slice
- [x] T053 [US5] Add service test `TestGetTrending_WithTTL` in `internal/admin/service_test.go`; verify `cache_ttl_sec` is populated and -2 when key missing
- [x] T054 [US5] Add `GET /api/admin/v1/trending/{ns}` handler in `internal/admin/handler.go` calling `svc.GetTrending`; pass through `limit`, `offset`, `window_hours` query params
- [x] T055 [US5] Add handler tests `TestGetTrending_OK`, `TestGetTrending_EmptyCache` in `internal/admin/handler_test.go`
- [x] T056 [P] [US5] Create `web/admin/src/hooks/useTrending.ts`: TanStack Query hook for `GET /api/admin/v1/trending/{ns}`
- [x] T057 [P] [US5] Create `web/admin/src/pages/TrendingPage.tsx`: namespace dropdown, ranked list of trending items showing `object_id`, `score` (2 decimal places), and a "Cache expires in Xm Ys" indicator using `cache_ttl_sec`; "No trending data — run cron to populate" empty state when `cache_ttl_sec === -2`

**Checkpoint**: US5 complete. All 5 user stories are independently functional.

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Final integration, documentation, and build validation.

- [x] T058 Update `CLAUDE.md` REST API table with `cmd/admin` routes: health proxy, namespace CRUD, batch-runs, trending, recommend debug; note port 2002 and session-cookie auth
- [x] T059 [P] Create `Dockerfile.admin` multi-stage build: stage 1 `node:20-alpine` runs `npm ci && npm run build` in `web/admin/`; stage 2 `golang:1.26.1-alpine` copies `web/admin/dist/` and builds `./cmd/admin`; final stage copies binary only
- [x] T060 [P] Add `web/admin/src/components/NavLink.tsx` active-state styling and keyboard navigation; ensure all pages are reachable from the sidebar without a mouse
- [x] T061 Run `make lint` on `internal/admin/` and fix any golangci-lint violations
- [x] T062 Run `make test` and confirm all tests in `internal/admin/` pass; run `cd web/admin && npm run typecheck` to confirm React TypeScript compiles without errors
- [x] T063 Validate `quickstart.md` end-to-end: apply migration, build admin binary, log in, exercise all 5 pages; confirm SC-001 (health visible in <5s) and SC-003 (recommend debug in <30s)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately.
- **Foundational (Phase 2)**: Depends on Phase 1 completion — **blocks all user stories**.
- **US1 (Phase 3)**: Depends on Phase 2 only. No dependency on US2–US5.
- **US2 (Phase 4)**: Depends on Phase 2 only. No dependency on US1/US3–US5.
- **US3 (Phase 5)**: Depends on Phase 2 only. Benefits from US2 (namespace dropdown), but independently testable with a hardcoded namespace input.
- **US4 (Phase 6)**: Depends on Phase 2 only. T044 (cron write) is independent of all admin handler tasks.
- **US5 (Phase 7)**: Depends on Phase 2 only. Needs Redis client wired in `cmd/admin` (done in T010/T017).
- **Polish (Phase 8)**: Depends on all desired user stories complete.

### User Story Cross-Dependencies

| Story | Hard Dependency | Soft Integration |
|-------|----------------|-----------------|
| US1 (Health) | Phase 2 only | None |
| US2 (Namespaces) | Phase 2 only | None |
| US3 (Debug) | Phase 2 only | US2 namespace list (dropdown) |
| US4 (Batch Runs) | Phase 2 only | None |
| US5 (Trending) | Phase 2 only | US2 namespace list (dropdown) |

### Parallel Opportunities Within Phase 2

These tasks have no file conflicts and can run in parallel:
- T007 (migration SQL) — independent of all Go code
- T008+T009 (docs.go + types.go) — pure declarations
- T019+T020 (React shell) — independent of backend

### Parallel Opportunities Within Each Story Phase

- React hooks [P] and React pages [P] can be developed in parallel with backend handler/service work once types are defined (T009).

---

## Parallel Example: Foundational Phase

```text
Stream A (Backend):
  T007 → T008 → T009 → T010 → T011 → T012 → T013 → T014 → T015 → T016 → T017 → T018

Stream B (Frontend, starts at T009 when types known):
  T019 → T020
```

## Parallel Example: User Story 1 (MVP)

```text
Stream A (Backend):
  T021 → T022 → T023

Stream B (Frontend, parallel after T022 contract known):
  T024 → T025 → T026
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1 (Setup)
2. Complete Phase 2 (Foundational) — **critical blocker**
3. Complete Phase 3 (US1 — Health At a Glance)
4. **STOP and VALIDATE**: Open dashboard, verify health page works
5. All 6 tasks + binary delivers operator value immediately

### Incremental Delivery

1. Setup + Foundational → login page and binary ship
2. + US1 → health monitoring live (**MVP**)
3. + US2 → namespace management without SSH
4. + US3 → recommendation debugging without curl
5. + US4 → batch job history visible
6. + US5 → trending inspector complete

---

## Notes

- **Total tasks**: 63 (T001–T063)
- **[P] parallelizable tasks**: 23
- **Test tasks**: Included throughout (Constitution Gate II requirement)
- Each user story is independently completable without the others
- Commit after each checkpoint to keep the branch green
- `make test` must pass before moving to the next story phase
