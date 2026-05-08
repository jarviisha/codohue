---
description: "Task list for RESTful API Redesign"
---

# Tasks: RESTful API Redesign

**Input**: Design documents from `/specs/003-restful-api-redesign/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: This feature is a refactor of existing handlers. Existing `handler_test.go` files in affected packages MUST be updated to match the redesigned contract — that is regression maintenance, not new TDD test creation. No new test packages or frameworks are added.

**Organization**: Tasks are grouped by user story so each story can be implemented, tested, and merged independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel with other [P] tasks in the same phase (different files, no dependencies on incomplete tasks)
- **[Story]**: Maps task to a user story from spec.md (US1, US2, US3)
- All file paths are repo-relative

## Path Conventions

Repo layout (Go web service + embedded SPA):

- Backend Go source: `cmd/api/`, `cmd/admin/`, `internal/<domain>/`
- Frontend (admin SPA, embedded into `cmd/admin`): `web/admin/src/`
- Specs and contracts: `specs/003-restful-api-redesign/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Confirm baseline so the redesign starts from a known-good state.

- [X] T001 Confirm `git rev-parse --abbrev-ref HEAD` is `003-restful-api-redesign` and `git status --short` shows only the spec/plan artifacts under `specs/003-restful-api-redesign/` (and any unrelated unstaged config edits the user already had).
- [X] T002 Run `make build && make test` against the current branch tip to capture a baseline. All tests must pass before redesign work begins; record any pre-existing flaky packages so they are not mis-attributed to this feature.

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Decouple admin namespace upsert from the data-plane HTTP server. This unblocks both US1 (which removes the data-plane upsert route) and US2 (whose admin upsert handler must keep working).

**⚠️ CRITICAL**: No user-story work can begin until T003 lands.

- [X] T003 Refactor `internal/admin/service.go` so the namespace-upsert path calls `nsconfig.Service` directly instead of proxying via the data-plane HTTP API. Update the constructor signature in `cmd/admin/main.go` to inject `nsconfig.Service` (already constructed there) and drop `cfg.APIURL`/`cfg.RecommenderAPIKey` from the proxy path. Update `internal/admin/service_test.go` to remove the HTTP-mock fixture for namespace upsert.

**Checkpoint**: With T003 merged, the admin server no longer depends on the data-plane API for namespace mutation. US1 may now safely delete the data-plane upsert route.

---

## Phase 3: User Story 1 - Client integrator works against a single canonical API surface (Priority: P1) 🎯 MVP

**Goal**: Deliver a single canonical, resource-oriented data-plane API on port 2001. All legacy duplicate paths return 404; no request body carries a redundant `namespace` field; embedding writes are PUT (idempotent); rankings live at `POST /v1/namespaces/{ns}/rankings`; recommendations live at `GET /v1/namespaces/{ns}/subjects/{id}/recommendations`.

**Independent Test**: Run quickstart sections 3, 4, 5, 6, 7 — each canonical path returns its expected status code and each negative-check legacy path returns 404. No other code (admin server, web UI) is required to verify US1.

### Implementation for User Story 1

- [X] T004 [P] [US1] Remove the `Namespace` field from `IngestRequest` in `internal/ingest/types.go`. Ensure JSON tags and validators no longer reference it.
- [X] T005 [US1] Update `internal/ingest/handler.go` so the `Ingest` handler reads namespace exclusively from `chi.URLParam(r, "ns")`, ignores any incoming body `namespace` field, and returns `202 Accepted` with empty body. Remove any branch that read `req.Namespace`.
- [X] T006 [P] [US1] Remove the `Namespace` field from `RankRequest` in `internal/recommend/types.go`. Ensure JSON tags and validators no longer reference it.
- [X] T007 [US1] Refactor `internal/recommend/handler.go`:
  - Collapse `Get` and `GetByNamespace` into one handler `GetSubjectRecommendations` that reads `ns` and `id` via `chi.URLParam`. Delete the legacy `Get` and `GetByNamespace` exports.
  - Remove the deferred `validateKey` call from `Rank`; the route-group middleware now enforces auth. Remove the `validateKey` parameter from `NewHandler` and update the constructor signature.
  - Rename the `RankByNamespace` exported method (if separate) to a single `Rank` reading namespace from path.
  - Convert `StoreObjectEmbedding` and `StoreSubjectEmbedding` to expect PUT (the handler logic stays the same; the route binding is the change in T010, but ensure handler returns `204 No Content`).
  - Ensure `DeleteObject` returns `204 No Content`.
- [X] T008 [P] [US1] Update `internal/ingest/handler_test.go`:
  - Drop fixtures that include `namespace` in body.
  - Add a case asserting the handler accepts a body with a stray `namespace` field (silently ignored).
  - Assert `202 Accepted` with empty response body.
- [X] T009 [P] [US1] Update `internal/recommend/handler_test.go`:
  - Replace `Get` / `GetByNamespace` cases with `GetSubjectRecommendations` cases reading `ns` and `id` from path.
  - Drop the deferred-auth fixture from `Rank` tests; rely on middleware (test the handler in isolation, then test it under the route group).
  - Verify embedding handlers return `204` and accept PUT only.
  - Verify `DeleteObject` returns `204` and is idempotent.
- [X] T010 [US1] Rewrite the route table in `cmd/api/main.go`:
  - Remove the admin-only group containing `PUT /v1/config/namespaces/{namespace}` and the import of `nsconfig` from this file.
  - Remove the deferred-auth group containing `POST /v1/rank`.
  - Remove every legacy route: `/v1/recommendations`, `/v1/rank`, `/v1/trending/{ns}`, `/v1/objects/{ns}/{id}/embedding`, `/v1/subjects/{ns}/{id}/embedding`, `DELETE /v1/objects/{ns}/{id}`, `POST /v1/namespaces/{ns}/rank`, `GET /v1/namespaces/{ns}/recommendations`, `POST /v1/namespaces/{ns}/objects/{id}/embedding`, `POST /v1/namespaces/{ns}/subjects/{id}/embedding`.
  - Register one chi route group under `auth.RequireNamespace(cfg.RecommenderAPIKey, keyHashFn, func(r *http.Request) string { return chi.URLParam(r, "ns") })` carrying:
    - `POST   /v1/namespaces/{ns}/events`
    - `GET    /v1/namespaces/{ns}/subjects/{id}/recommendations`
    - `POST   /v1/namespaces/{ns}/rankings`
    - `GET    /v1/namespaces/{ns}/trending`
    - `PUT    /v1/namespaces/{ns}/objects/{id}/embedding`
    - `PUT    /v1/namespaces/{ns}/subjects/{id}/embedding`
    - `DELETE /v1/namespaces/{ns}/objects/{id}`
  - Drop the `validateKey` argument passed to `recommend.NewHandler` (handler no longer accepts it after T007).
- [ ] T011 [US1] Live verify: build (`make build-api`), run (`./tmp/api &`), and execute quickstart [sections 3, 4 (data-plane portion), 5, 6, 7](./quickstart.md). Confirm every command's status code matches the documented expectation; in particular, every "Negative check" legacy path returns 404.

**Checkpoint**: At this point, the data-plane API is fully redesigned. US1 is independently shippable and demonstrable.

---

## Phase 4: User Story 2 - Admin operator manages namespaces and pipeline jobs through resource-oriented endpoints (Priority: P2)

**Goal**: Deliver a clean, resource-oriented admin API on port 2002 (sessions, namespaces, batch-runs, qdrant inspection, debug recommendations, demo data) and update the embedded admin SPA to call only the new paths.

**Independent Test**: Run quickstart sections 1, 2, 4 (admin trigger), 8, 9, 10, 11, 12 — every admin endpoint returns its expected status code, the SPA loads with no console errors, and every negative-check legacy admin path returns 404.

### Backend implementation for User Story 2

- [X] T012 [P] [US2] Update `internal/admin/types.go`:
  - Add `BatchRunCreateResponse{ ID, Namespace, Status, StartedAt }` (replaces the older trigger-response type).
  - Add or update `NamespaceUpsertResponse` to include an optional `APIKey *string` (set only on first-time create).
  - Add `AdminRecommendResponse` with `Items, Total, Source, GeneratedAt, Debug *RecommendDebug`.
  - Add `RecommendDebug{ SparseNNZ, DenseScore, Alpha, SeenItemsCount, InteractionCount }`.
  - Add `CreateSessionRequest{ APIKey }` and `CreateSessionResponse{ ExpiresAt }`.
  - Add `QdrantInspectResponse{ Subjects, Objects, SubjectsDense, ObjectsDense }` and `QdrantCollection{ PointsCount, Exists }` (renames the prior `QdrantStats*` types).
- [X] T013 [US2] Refactor `internal/admin/handler.go`:
  - Rename `Login` → `CreateSession`. Status `201 Created`, set cookie, body returns `CreateSessionResponse`.
  - Rename `Logout` → `DeleteCurrentSession`. Status `204 No Content`, clear cookie, no body.
  - Rename `TriggerBatch` → `CreateBatchRun`. Status `202 Accepted`, set `Location: /api/admin/v1/namespaces/{ns}/batch-runs/{id}`, body returns `BatchRunCreateResponse`.
  - Replace `DebugRecommend` (POST) with debug-mode support inside `GetSubjectRecommendations` (GET). Read `?debug=true` from query and populate the optional `Debug` field on the response.
  - Rename `SeedDemoDataset` → `CreateDemoData` (status `202`), `ClearDemoDataset` → `DeleteDemoData` (status `204`).
  - Rename `GetQdrantStats` → `GetQdrant`. Body uses the new `QdrantInspectResponse`.
  - Update `UpsertNamespace` to invoke the refactored admin service from T003 directly. Return `200` on update, `201` (with `api_key` field) on first-time create.
- [X] T014 [US2] Update `internal/admin/handler_test.go`:
  - Rename test functions to match the renamed handlers.
  - Assert the new status codes (201 for login, 202 + `Location` for batch-run, 204 for logout/clear-demo, etc.).
  - Add a test for the `?debug=true` query mode of `GetSubjectRecommendations`.
  - Add a test for `GET /api/admin/v1/namespaces/{ns}/qdrant` returning the new shape.
  - Add a test for `PUT /api/admin/v1/namespaces/{ns}` calling `nsconfig.Service` directly (mock that service, not the HTTP API URL).
- [X] T015 [US2] Rewrite the route table in `cmd/admin/main.go`:
  - Auth (public): `POST /api/v1/auth/sessions`. Remove `POST /api/auth/login`.
  - Auth (inside `RequireSession` group): `DELETE /api/v1/auth/sessions/current`. Remove `DELETE /api/auth/logout`.
  - Inside the `RequireSession` group, register exactly:
    - `GET    /api/admin/v1/health`
    - `GET    /api/admin/v1/namespaces` (with optional `?include=overview`)
    - `GET    /api/admin/v1/namespaces/{ns}`
    - `PUT    /api/admin/v1/namespaces/{ns}`
    - `GET    /api/admin/v1/batch-runs`
    - `GET    /api/admin/v1/namespaces/{ns}/batch-runs`
    - `POST   /api/admin/v1/namespaces/{ns}/batch-runs`
    - `GET    /api/admin/v1/namespaces/{ns}/qdrant`
    - `GET    /api/admin/v1/namespaces/{ns}/trending`
    - `GET    /api/admin/v1/namespaces/{ns}/events`
    - `POST   /api/admin/v1/namespaces/{ns}/events`
    - `GET    /api/admin/v1/namespaces/{ns}/subjects/{id}/profile`
    - `GET    /api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations`
    - `POST   /api/admin/v1/demo-data`
    - `DELETE /api/admin/v1/demo-data`
  - Remove every legacy admin route: `/api/admin/v1/recommend/debug`, `/api/admin/v1/trending/{ns}`, `/api/admin/v1/subjects/{ns}/{id}/profile`, `/api/admin/v1/namespaces/{ns}/qdrant-stats`, `/api/admin/v1/namespaces/{ns}/batch-runs/trigger`, `/api/admin/v1/demo`, the existing `/api/admin/v1/namespaces/overview` (folded into `?include=overview`).

### Frontend implementation for User Story 2

- [X] T016 [P] [US2] Update `web/admin/src/services/api.ts` to point every helper at the canonical data-plane paths from [contracts/data-plane.md](./contracts/data-plane.md). In particular: rankings → `/v1/namespaces/{ns}/rankings`, recommendations → `/v1/namespaces/{ns}/subjects/{id}/recommendations`, embeddings → `PUT /v1/namespaces/{ns}/{objects|subjects}/{id}/embedding`.
- [X] T017 [P] [US2] Update `web/admin/src/services/adminApi.ts`:
  - Auth: `POST /api/v1/auth/sessions` (login), `DELETE /api/v1/auth/sessions/current` (logout).
  - Namespaces overview: `GET /api/admin/v1/namespaces?include=overview`.
  - Qdrant: `GET /api/admin/v1/namespaces/{ns}/qdrant`.
  - Subject profile: `GET /api/admin/v1/namespaces/{ns}/subjects/{id}/profile`.
  - Trending: `GET /api/admin/v1/namespaces/{ns}/trending`.
  - Debug recommendations: `GET /api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations?debug=true`.
  - Demo data: `POST /api/admin/v1/demo-data` and `DELETE /api/admin/v1/demo-data`.
- [X] T018 [P] [US2] Update `web/admin/src/hooks/useBatchRuns.ts`:
  - List: `GET /api/admin/v1/batch-runs` (cross-NS) and `GET /api/admin/v1/namespaces/{ns}/batch-runs` (scoped).
  - Trigger: `POST /api/admin/v1/namespaces/{ns}/batch-runs` (no `/trigger` suffix). Handle `202 Accepted` + `Location` header (extract the new run's id from `Location` and immediately add it to the list).
- [X] T019 [US2] Rename `web/admin/src/hooks/useQdrantStats.ts` → `useQdrant.ts` and update its target URL to `.../qdrant`. Find and update all imports (`grep -RIn 'useQdrantStats' web/admin/src/`) so the rename leaves no dangling references; rename the exported hook function symmetrically (`useQdrantStats` → `useQdrant`).
- [X] T020 [P] [US2] Audit `web/admin/src/hooks/useNamespacesOverview.ts` and any other hook (`grep -RIn '/api/admin/v1\|/api/auth\|/v1/' web/admin/src/`) for stale URLs not yet covered by T016–T019; update each to its canonical form.
- [X] T021 [US2] Add a URL-constants smoke test that asserts every URL string used by `api.ts` and `adminApi.ts` matches the contract in [contracts/data-plane.md](./contracts/data-plane.md) and [contracts/admin-plane.md](./contracts/admin-plane.md). Run it as part of `npm test` for the admin SPA.
- [X] T022 [US2] Build the admin SPA bundle: from `web/admin/`, run the project's existing build command (`npm run build` or the equivalent `make` target) and verify the bundle is produced. Then `make build-admin` and confirm the SPA is embedded into `./tmp/admin` without errors. Resolve any TypeScript errors surfaced by the URL changes.
- [ ] T023 [US2] Live verify: run `./tmp/admin &` and execute quickstart [sections 1, 2, 4 (admin trigger portion), 8, 9, 10, 11, 12](./quickstart.md). Confirm every status code matches and every negative-check legacy admin path returns 404. For section 12, walk through every admin page in the browser; DevTools Network tab must show only canonical URLs and zero 404s on user actions.

**Checkpoint**: With Phase 3 + Phase 4 merged, the entire HTTP surface (data plane + admin plane + admin SPA) follows the redesigned contract. Both stories are independently shippable.

---

## Phase 5: User Story 3 - Internal contributor adds a new endpoint following established conventions (Priority: P3)

**Goal**: Documentation and post-condition audits that lock the new conventions into place. After this phase a contributor reading the repo finds exactly one set of conventions for routes, DTOs, response shapes, and auth wiring.

**Independent Test**: A contributor reading `CLAUDE.md` and the route registration files can identify the canonical pattern in under 5 minutes; static audits (T025, T026) report zero violations.

- [X] T024 [US3] Rewrite the REST API tables in `CLAUDE.md` to reflect the redesigned surface only. Use [contracts/data-plane.md](./contracts/data-plane.md), [contracts/admin-plane.md](./contracts/admin-plane.md), and [contracts/auth-plane.md](./contracts/auth-plane.md) as the authoritative source. Remove every legacy row. Update the "Two-tier auth" prose paragraph to clarify that namespace mutation now lives only on the admin plane.
- [X] T025 [P] [US3] Static grep verification — from repo root, run:
  - `grep -RIn -E '(/(login|logout|trigger|qdrant-stats)|/recommend/debug|/v1/rank([^i]|$))' cmd/ internal/ web/admin/src/ | grep -v _test.go | grep -v specs/ | grep -v CLAUDE.md`
  - Expected: zero matches. Any match is a residual legacy reference and must be cleaned up before this story closes.
- [X] T026 [P] [US3] DTO audit — read `internal/ingest/types.go`, `internal/recommend/types.go`, and `internal/admin/types.go`. Confirm no request DTO defines a `Namespace string` field whose corresponding handler route includes `{ns}` in the path. Capture a one-line confirmation comment in the PR description.

**Checkpoint**: Documentation and audits done. The redesign is complete and self-documenting.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Quality gates and end-to-end verification across the whole change set.

- [X] T027 [P] Run `make fmt` and `make lint`; resolve any new lint errors introduced by the redesign. The lint pass must be green before the PR opens.
- [X] T028 [P] Run `make test` end-to-end; every Go package must pass. From `web/admin/`, run the SPA test command (`npm test --run` or equivalent) and confirm the URL-constants test from T021 passes.
- [ ] T029 Run the full [quickstart.md](./quickstart.md) (sections 1–11) against locally running `./tmp/api`, `./tmp/admin`, and `./tmp/cron`. Tick each section's pass criterion.
- [ ] T030 Manual UI walkthrough per [quickstart.md section 12](./quickstart.md). Open every admin page, perform every interactive action (login, create namespace, trigger batch, inject test event, debug recommendation, seed/clear demo, logout), and confirm zero broken pages and zero browser-console errors.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies. Can start immediately.
- **Phase 2 (Foundational)**: Depends on Phase 1. Blocks every user story.
- **Phase 3 (US1)**: Depends on Phase 2. Independently mergeable when complete.
- **Phase 4 (US2)**: Depends on Phase 2. Independently mergeable when complete. Phase 3 and Phase 4 can be developed in parallel by different contributors because they touch disjoint files (cmd/api vs cmd/admin, recommend+ingest vs admin handler, no frontend in US1).
- **Phase 5 (US3)**: Depends on Phases 3 and 4 — the documentation and audits assert post-conditions of the redesigned surface, so the surface must exist first.
- **Phase 6 (Polish)**: Depends on all preceding phases.

### Within-Phase Dependencies

**Phase 3 (US1)**:
- T004 → T005 (handler imports types).
- T006 → T007 (handler imports types).
- T008 [P] T009 (different test files; both depend on their respective handlers being updated).
- T010 depends on T005 and T007 (must not delete routes that point to handlers being refactored on a different branch).
- T011 depends on T010 (live server must be built from the rewritten route table).

**Phase 4 (US2)**:
- T012 → T013 (handler uses new DTOs).
- T013 → T014 (tests follow handler signatures).
- T013 → T015 (route table binds the renamed handler methods).
- T016, T017, T018, T020 are [P] (different files in `web/admin/src/`).
- T019 (rename) is sequential because it touches multiple importing files.
- T021 depends on T016–T020 (tests the URL constants those files now export).
- T022 depends on T016–T021 (build needs the SPA to type-check).
- T023 depends on T015 and T022 (live admin server + built SPA).

**Phase 5 (US3)**:
- T024 [P] T025 [P] T026 — all three are independent (documentation write vs read-only audits on disjoint targets).

**Phase 6**:
- T027 [P] T028 (different commands; can run in parallel mechanically, though most CI pipelines run them serially).
- T029 depends on T027 and T028 (manual e2e after lint+tests are green).
- T030 depends on T029 (UI walkthrough after backend e2e).

---

## Parallel Opportunities

### Parallel within Phase 3 (US1)

Once T003 has merged, both DTO updates can run in parallel:

```bash
# Pair 1
Task: T004 — Remove Namespace from internal/ingest/types.go
Task: T006 — Remove Namespace from internal/recommend/types.go::RankRequest
```

After their respective handlers (T005, T007) are updated, the test updates can run in parallel:

```bash
Task: T008 — Update internal/ingest/handler_test.go
Task: T009 — Update internal/recommend/handler_test.go
```

### Parallel within Phase 4 (US2)

Once the backend handler refactor (T013) is on the branch, all four frontend updates can run in parallel:

```bash
Task: T016 — Update web/admin/src/services/api.ts
Task: T017 — Update web/admin/src/services/adminApi.ts
Task: T018 — Update web/admin/src/hooks/useBatchRuns.ts
Task: T020 — Audit and patch any remaining hooks for stale URLs
```

### Parallel between US1 and US2

Phases 3 and 4 are file-disjoint and can be developed by different contributors simultaneously after Phase 2 completes. Recommended only if there are at least two contributors; otherwise serial in priority order is faster overall.

### Parallel within Phase 5 (US3)

```bash
Task: T024 — Rewrite REST API tables in CLAUDE.md
Task: T025 — Run grep audit for residual legacy paths
Task: T026 — DTO audit
```

### Parallel within Phase 6 (Polish)

```bash
Task: T027 — make fmt && make lint
Task: T028 — make test (Go) and npm test (admin SPA)
```

---

## Implementation Strategy

### MVP First (US1 only)

1. Complete Phases 1 and 2.
2. Complete Phase 3 (US1).
3. **STOP and validate**: live-test the data-plane API per quickstart sections 3–7. The data-plane API surface is now clean and shippable on its own — DarkVoid (or any new client) can integrate against it without touching the admin server.

### Incremental Delivery

1. Setup + Foundational → foundation ready.
2. US1 → data-plane MVP. Merge to `main`. Demo to stakeholders.
3. US2 → admin-plane redesign + admin SPA update. Merge. Demo.
4. US3 → docs and audits. Merge.
5. Polish → lint, full test suite, end-to-end smoke. Tag the release.

### Parallel Team Strategy

With two contributors after Phase 2:

- Contributor A: US1 (Phase 3) — Go data-plane refactor.
- Contributor B: US2 (Phase 4) — Go admin-plane refactor + SPA.
- Both rejoin for US3 (Phase 5) and Polish (Phase 6).

---

## Notes

- **No new tests beyond regression**: every test task in this list updates an existing `_test.go` file. No new test packages or frameworks are introduced. Constitution gate II (every business-logic file has a `_test.go`) was already satisfied before this feature.
- **No DB changes**: zero migrations. The schema before and after this feature is identical.
- **Single change set assumption**: the spec assumes data-plane, admin-plane, and SPA ship together. If you split the merge across multiple PRs, the admin SPA must not be deployed against a backend that lacks the canonical paths, and vice versa.
- **Commit cadence**: commit after each task (or each [P] group). Constitutional rule "create new commits, never amend" applies.
- **Stop at any checkpoint to validate** — that is the value of the per-story phase boundaries.
