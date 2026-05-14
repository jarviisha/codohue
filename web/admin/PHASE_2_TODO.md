# Phase 2 — Task Breakdown

Granular working checklist for Phase 2 of the `web/admin` build. Companion to [BUILD_PLAN.md](BUILD_PLAN.md) §5 — same scope, finer granularity (18 small commits instead of 12 page-sized ones).

## Process

- One commit per checked task.
- Each task lands as a self-contained change: `npm run build`, `npm test`, and `go build -tags=embedui ./cmd/admin` must all be green before commit.
- Tasks are executed sequentially. After each commit, we stop so the user can preview the change. The next task only starts after the user gives the go-ahead.

## Conventions

- Commit message prefix: `feat(web/admin):` for pages, `feat(web/admin/services):` for service-layer files, `feat(web/admin/services+page):` only for combined ones (2.G).
- Service files follow BUILD_PLAN §4.1: request functions + types + TanStack Query hooks all live next to each other in the per-domain file. Keys are exposed via `services/queryKeys.ts` re-export.
- Pages replace their Phase 1.2 stub at the existing route. The route entry in `routes/index.tsx` stays put.
- All status surfaces use `StatusToken`. All forms compose `<Field>` + the form primitives from Phase 1.3/1.6. All HTTP routes through `services/http.ts` — never raw `fetch` inside a page or service (the `urls.test.mjs` smoke test enforces this).
- Update `tests/urls.test.mjs` whenever route paths change.
- Every page registers at least one `useRegisterCommand(...)` entry in the command palette.

---

## 2.A — Namespaces domain

- [x] **2.A.1 `services/namespaces.ts`**
  - Types matching `internal/admin/types.go`: `NamespaceConfig`, `NamespaceUpsertRequest`, `NamespaceUpsertResponse`, `NamespacesOverviewResponse`, `NamespaceHealth`.
  - Request functions: `listNamespaces({ include })`, `getNamespace(name)`, `upsertNamespace(name, payload)`.
  - Hooks: `useNamespaces`, `useNamespacesOverview`, `useNamespace`, `useUpsertNamespace`.
  - Key factory `namespaceKeys`; re-exported from `services/queryKeys.ts`.

- [x] **2.A.2 `NamespacesListPage` — BUILD_PLAN build-order #2**
  - Replace stub at `/namespaces`. Table of every namespace with last-run status token (from overview include).
  - "Create" button routes to `/namespaces/new` via `paths.namespaceCreate`.
  - **Done when:** every row renders with the right status token, no raw `fetch`, Cmd+K registers a "Create namespace" command.

- [x] **2.A.3 `NamespaceCreatePage` — build-order #3**
  - Replace stub at `/namespaces/new`. Form with action-weight matrix, lambda, alpha, embedding_dim, dense-strategy RadioGroup, trending settings.
  - Local + server validation; `Notice` summarises errors at the form top.
  - On success, navigate to `/ns/:name`; surface returned `api_key` once via a one-shot `Notice` so the operator can copy it.

- [x] **2.A.4 `NamespaceOverviewPage` — build-order #4 (BUILD_PLAN §7.1 mockup)**
  - Replace stub at `/ns/:name`. Panels: Health probes, Volume (24h), Embedding, Last batch run with per-phase tokens, Trending top-5.
  - "Run batch now" primary button + palette command.
  - Layout matches the §7.1 ASCII mockup.

- [x] **2.A.5 `NamespaceConfigPage` — build-order #5**
  - Replace stub at `/ns/:name/config`. Reuse the form layout from Create. Hydrate from `useNamespace(name)`; dirty-state guard on navigation away.
  - Save returns to Overview.

---

## 2.B — Catalog domain

- [x] **2.B.1 `services/catalog.ts`**
  - Types matching `internal/admin/types.go`: `NamespaceCatalogConfig`, `NamespaceCatalogResponse`, `NamespaceCatalogUpdateRequest`, `CatalogStrategyDescriptor`, `CatalogBacklog`, `CatalogReEmbedResponse`, `CatalogItemSummary`, `CatalogItemDetail`, `CatalogItemsListResponse`, `CatalogRedriveResponse`, `CatalogBulkRedriveResponse`.
  - Request functions + hooks for: get-config-plus-strategies-plus-backlog, update config, re-embed, list items (paginated + state + object_id filters), get single item, redrive one, bulk redrive deadletter, hard-delete item.

- [x] **2.B.2 `CatalogConfigPage` — build-order #10**
  - Replace stub at `/ns/:name/catalog`. Status panel + form (strategy picker + params + max-attempts + max-content-bytes).
  - Dim-mismatch 400 → inline `Notice` showing both strategy dim and namespace dim.
  - "Re-embed all" via `ConfirmDialog`; 409 (already running) handled with a friendly `Notice`.

- [x] **2.B.3 `CatalogItemsListPage` — build-order #11 (BUILD_PLAN §7.2 mockup)**
  - Replace stub at `/ns/:name/catalog/items`. Filter toolbar (state Select, object_id Input). Table with `StatusToken` per row.
  - `Pagination` footer. Per-row "redrive" ghost button on `[FAIL]` rows; bulk "Redrive deadletter (N)" action in the header.
  - Filter state mirrored to URL query params.

- [x] **2.B.4 `CatalogItemDetailModal` — build-order #12**
  - Replace stub modal at `/ns/:name/catalog/items/:id`. Content via `CodeBlock`, metadata via `KeyValueList`. Redrive / delete via `ConfirmDialog`. Renders as an `Outlet` over the list page.

---

## 2.C — Events

- [ ] **2.C.1 `services/events.ts`**
  - Types `Event`, `EventsListResponse`, `InjectEventRequest`.
  - Functions: `listEvents({ namespace, limit, offset, subject_id })`, `injectEvent(namespace, payload)`.
  - `InjectEventRequest` mirrors the current backend contract: `subject_id`, `object_id`, `action`, optional `occurred_at`. Do not add `weight` in the UI unless backend support lands first.
  - Hooks: `useEvents` (poll cadence configurable), `useInjectEvent`.

- [ ] **2.C.2 `EventsListPage` + `InjectEventModal` — build-order #6 (BUILD_PLAN §7.3 mockup)**
  - Replace stub at `/ns/:name/events`. Table with mono ms-precision timestamps, age delta column. Subject filter persisted to URL.
  - Inject event modal: form with action, subject_id, object_id, occurred_at.
  - Live-tail footer using the Phase 1.6 `Switch` to toggle a faster poll cadence; `[ RUN]` / `[IDLE]` status reflects state.

---

## 2.D — Batch Runs

- [ ] **2.D.1 `services/batchRuns.ts`**
  - Types `BatchRun`, `BatchRunsResponse`, `BatchRunCreateResponse`.
  - Functions: `listBatchRuns({ namespace?, status?, limit, offset })`, `createBatchRun(namespace)`, `getBatchRun(id)`.
  - Hooks: `useBatchRuns`, `useCreateBatchRun`.

- [ ] **2.D.2 `BatchRunsListPage` — build-order #9**
  - Replace stub at `/ns/:name/batch-runs`. List + status / trigger-source filters. Per-row detail Modal with per-phase `StatusToken` and timings.
  - `log_lines` rendered in `CodeBlock` if present.

---

## 2.E — Trending

- [ ] **2.E.1 `services/trending.ts`**
  - Types `TrendingItem`, `TrendingAdminResponse`.
  - Function `listTrending({ namespace, limit, offset, window_hours })`.
  - Hook `useTrending`.

- [ ] **2.E.2 `TrendingPage` — build-order #7**
  - Replace stub at `/ns/:name/trending`. Table + window selector (1h / 6h / 24h / 7d) in URL query. Redis TTL surfaced in panel header.

---

## 2.F — Recommend Debug

- [ ] **2.F.1 `services/recommend.ts`**
  - Types `RecommendDebugRequest`, `RecommendDebug`, `SubjectProfileResponse`.
  - Functions: `recommendDebug({ namespace, subject_id, limit })`, `subjectProfile({ namespace, subject_id })`.
  - Hooks.

- [ ] **2.F.2 `DebugPage` — build-order #8**
  - Replace stub at `/ns/:name/debug`. Form: subject_id + limit + "debug=true" toggle. Result table with score breakdown columns; the raw debug envelope rendered via `CodeBlock`.

---

## 2.G — Demo Data

- [ ] **2.G `DemoDataPage` — build-order #13**
  - Replace stub at `/ns/:name/demo-data`. The endpoint surface is tiny (just `POST /api/admin/v1/demo-data` and `DELETE` of same), so the request functions live inline in this file rather than getting their own `services/demoData.ts`.
  - Seed / clear actions, both gated by `ConfirmDialog`. Existing dataset state (last seeded at, row counts) surfaced in a `KeyValueList`.

---

## Phase 2 — Definition of done

- All 12 page stubs replaced by real implementations.
- Every route reachable; no broken navigation.
- Every page module registers at least one entry in `CommandPalette` (asserted by a follow-up smoke test in 2.A.2 or first page that lands).
- `tests/urls.test.mjs` enumerates all final routes.
- `npm run build`, `npm test`, `make build-admin`, and `go build -tags=embedui ./cmd/admin` all green at the end of every commit.
