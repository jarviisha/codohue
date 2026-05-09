---
description: "Task list for 004-catalog-embedder feature implementation"
---

# Tasks: Catalog Auto-Embedding Service

**Input**: Design documents from `/specs/004-catalog-embedder/`
**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: Per Constitution II, every `service.go`, `repository.go`, `worker.go`, and `job.go` MUST have a corresponding `_test.go` — test tasks are therefore in scope by repository policy, not optional.

**Organization**: Tasks are grouped by user story (US1, US2, US3) so each priority slice can be completed, demoed, and validated independently.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story label — only applies to Phase 3+ story tasks
- File paths are absolute repo-relative (single Go workspace project)

## Path Conventions

- New domains: `internal/catalog/`, `internal/embedder/`, `internal/core/embedstrategy/`
- New binary: `cmd/embedder/`
- Migrations: `migrations/010_*.{up,down}.sql`, `migrations/011_*.{up,down}.sql`
- e2e: `e2e/catalog_test.go` (`-tags=e2e`)
- Frontend: `web/admin/src/...`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project scaffolding before any code lands.

- [X] T001 Create new directories: `internal/catalog/`, `internal/embedder/`, `internal/core/embedstrategy/`, `cmd/embedder/`
- [X] T002 [P] Add Makefile targets `build-embedder`, `dev-embedder`, `run-embedder`, `logs-embedder` mirroring the existing `build-cron` / `dev` / `run-cron` / `logs-cron` shapes in `Makefile`
- [X] T003 [P] Add `embedder` service to `docker-compose.yml` and `docker-compose.app.yml` (image build + Redis/Postgres/Qdrant deps + `EMBEDDER_HEALTH_PORT=2003`)
- [X] T004 [P] Add new env vars to `.env.example`: `CATALOG_MAX_CONTENT_BYTES=32768`, `EMBED_MAX_ATTEMPTS=5`, `EMBEDDER_HEALTH_PORT=2003`, `EMBEDDER_REPLICA_NAME=`, `EMBEDDER_NAMESPACE_POLL_INTERVAL=30s`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Schema, strategy abstraction, and config wiring that EVERY user story depends on.

**⚠️ CRITICAL**: No user story phase may begin until Phase 2 is complete.

### Database migrations

- [X] T005 [P] Write migration 010 at `migrations/010_catalog_items.up.sql` and `migrations/010_catalog_items.down.sql` per [data-model.md §1](./data-model.md#1-new-table-catalog_items-migration-010) (table + four indexes; down drops table)
- [X] T006 [P] Write migration 011 at `migrations/011_namespace_configs_catalog.up.sql` and `migrations/011_namespace_configs_catalog.down.sql` per [data-model.md §2](./data-model.md#2-modified-table-namespace_configs-migration-011) (six new columns; down drops them)

### Strategy abstraction (forward-compat seam)

- [X] T007 [P] Create `internal/core/embedstrategy/docs.go` with the canonical `// Package embedstrategy ...` doc comment per [contracts/strategy-interface.md](./contracts/strategy-interface.md)
- [X] T008 [P] Create `internal/core/embedstrategy/types.go` with the `Strategy` interface, `Params`, `Factory`, `StrategyDescriptor`, and the five sentinel errors (`ErrUnknownStrategy`, `ErrDimensionMismatch`, `ErrZeroNorm`, `ErrInputTooLarge`, `ErrTransient`) per [contracts/strategy-interface.md](./contracts/strategy-interface.md)
- [X] T009 Create `internal/core/embedstrategy/registry.go` with `Registry`, `DefaultRegistry()`, `Register`, `Build`, `Has`, `List` (depends on T008)
- [X] T010 Create `internal/core/embedstrategy/registry_test.go` covering: register success, duplicate `(id, version)` panic, `Build` unknown returns `ErrUnknownStrategy`, `List` returns descriptor for every registered factory, concurrent-safe register/build under `-race`

### Namespace config extensions

- [X] T011 Extend `internal/nsconfig/types.go` with `CatalogEnabled bool`, `CatalogStrategyID string`, `CatalogStrategyVersion string`, `CatalogStrategyParams map[string]any`, `CatalogMaxAttempts int`, `CatalogMaxContentBytes int` per [data-model.md §2](./data-model.md#2-modified-table-namespace_configs-migration-011)
- [X] T012 Update `internal/nsconfig/repository.go` to read/write the six new columns; defaults must mirror migration defaults
- [X] T013 Update `internal/nsconfig/repository_test.go` for the new columns (round-trip read/write, default values, JSONB params encoding)
- [X] T014 Extend `internal/nsconfig/service.go` to validate `(CatalogStrategyID, CatalogStrategyVersion)` against `embedstrategy.DefaultRegistry()` AND to assert `Strategy.Dim() == namespace.embedding_dim` on enable; returns a typed error carrying both numbers (US2 #2 acceptance)
- [X] T015 Update `internal/nsconfig/service_test.go` for the validation paths: unknown strategy → `ErrUnknownStrategy`, dim mismatch → typed error with both dims, valid enable round-trip
- [X] T016 [P] Extend `internal/config` to load `CATALOG_MAX_CONTENT_BYTES`, `EMBED_MAX_ATTEMPTS`, `EMBEDDER_HEALTH_PORT`, `EMBEDDER_REPLICA_NAME`, `EMBEDDER_NAMESPACE_POLL_INTERVAL` (extend `LoadCron` + introduce `LoadEmbedder`); add unit tests in the existing config test file

**Checkpoint**: Foundation ready — migrations applied, strategy abstraction compiled, namespace config knows about catalog.

---

## Phase 3: User Story 1 — Social-network client ingests raw posts and immediately benefits from hybrid recommendations (Priority: P1) 🎯 MVP

**Goal**: A client can `POST /v1/namespaces/{ns}/catalog` with raw text and that item becomes discoverable through dense recommendations within 60 s, end-to-end.

**Independent Test**: Run `make test-e2e-heavy` against the new e2e suite — ingest 100 representative posts into a single namespace, wait for backlog to drain, confirm (a) `socialdemo_objects_dense` Qdrant collection has 100 vectors with `payload.strategy_version='v1'`, (b) recommendations served for a subject who has interacted with one of the posts surface other catalog-only items via the dense leg.

### Tokenizer + V1 hashing strategy

- [X] T017 [P] [US1] Create `internal/embedder/tokenizer.go` and `internal/embedder/tokenizer_test.go` per [research.md R2](./research.md#r2-tokenizer-implementation): NFC normalisation, lowercase, whitespace split via `unicode.IsSpace`, punctuation strip, URL prefix drop, character n-grams n∈[3,5]; tests include Vietnamese sample (`"Hôm nay trời đẹp quá"`), URL drop, hashtag preservation, empty-after-trim
- [X] T018 [P] [US1] Create `internal/embedder/hashing.go` and `internal/embedder/hashing_test.go` per [research.md R1](./research.md#r1-embedding-algorithm--concrete-shape-of-the-v1-deterministic-strategy): feature hashing with sign trick, L2 normalisation, deterministic output for identical input across runs, dim-correctness, zero-norm path returning `ErrZeroNorm` from `embedstrategy`
- [X] T019 [US1] Create `internal/embedder/strategy.go` and `internal/embedder/strategy_test.go` — `init()` calls `embedstrategy.DefaultRegistry().Register("internal-hashing-ngrams", "v1", factory)` where factory reads `Params["dim"]`; tests verify registration is visible from `embedstrategy.DefaultRegistry().Build("internal-hashing-ngrams","v1",Params{"dim":128})` (depends on T009, T018)
- [X] T020 [P] [US1] Create `internal/embedder/docs.go` with the canonical `// Package embedder ...` doc comment

### Catalog domain (data-plane HTTP ingest)

- [X] T021 [P] [US1] Create `internal/catalog/docs.go` with the canonical `// Package catalog ...` doc comment
- [X] T022 [P] [US1] Create `internal/catalog/types.go`: `IngestRequest{ObjectID, Content string; Metadata map[string]any}`, `CatalogItem` struct mirroring [data-model.md §1](./data-model.md#1-new-table-catalog_items-migration-010), and a `ContentHash(content string) []byte` helper (sha256 over `content` only, per FR-002 and Q4)
- [X] T023 [US1] Create `internal/catalog/repository.go` and `internal/catalog/repository_test.go`: UPSERT by `(namespace, object_id)` with idempotency on identical `content_hash`; state transition helpers (`MarkInFlight`, `MarkEmbedded`, `MarkFailed`, `MarkDeadLetter`); tests use a real Postgres test DB (existing pattern in repo)
- [X] T024 [US1] Create `internal/catalog/service.go` and `internal/catalog/service_test.go`: validation (empty content, oversized vs `nsconfig.CatalogMaxContentBytes`), hash compute, persist via repository, `XADD catalog:embed:{ns}` per [contracts/redis-stream.md](./contracts/redis-stream.md); tests cover idempotency no-op, oversized rejection (413 path returned to handler as typed error), namespace-not-enabled rejection, transient Redis failure ⇒ row remains `pending` for the recovery sweep
- [X] T025 [US1] Create `internal/catalog/handler.go` and `internal/catalog/handler_test.go`: `POST /v1/namespaces/{ns}/catalog` with the existing per-namespace bearer auth middleware; status code mapping per [contracts/rest-api.md](./contracts/rest-api.md) (202/400/401/404/413/422)
- [X] T026 [US1] Wire `internal/catalog.Handler.Routes()` into `cmd/api/main.go` and update `cmd/api/main_test.go` to assert the new route is registered (mirrors how the existing ingest handler is wired)

### Embedder domain (worker)

- [X] T027 [P] [US1] Create `internal/embedder/types.go`: worker config struct (`Concurrency`, `MaxAttempts`, `MinIdleReclaim`), Redis stream entry decoding helpers
- [X] T028 [US1] Create `internal/embedder/repository.go` and `internal/embedder/repository_test.go`: read pending `catalog_items` by id, write the four success/failure state transitions (mirrors the catalog repo but is independently testable so the cross-domain rule remains intact), recovery-sweep query `state='pending' AND id NOT IN (XPENDING ids)`
- [X] T029 [US1] Create `internal/embedder/service.go` and `internal/embedder/service_test.go`: per-item orchestration — load row, build cached `Strategy` via `embedstrategy.Build` keyed on `(strategy_id, strategy_version, paramsHash)`, embed, validate dim against `nsconfig.embedding_dim` (defence in depth per FR-009), upsert Qdrant `{ns}_objects_dense` via existing `idmap` flow with `payload.{strategy_id, strategy_version, embedded_at}` per [data-model.md §4](./data-model.md#4-qdrant-point-payload-conventions), mark embedded; service_test mocks Qdrant and registry, exercises the FR-010 error→state mapping table from [contracts/strategy-interface.md](./contracts/strategy-interface.md)
- [X] T030 [US1] Create `internal/embedder/worker.go` and `internal/embedder/worker_test.go`: per-namespace `XREADGROUP > BLOCK 5s COUNT 32` loop, `XAUTOCLAIM` reaper goroutine (60 s min-idle), recovery sweep goroutine, namespace-registry poller per [contracts/redis-stream.md](./contracts/redis-stream.md); worker_test uses `miniredis` (or equivalent) and a mocked service to assert the loop calls service for each entry, ACKs on success, leaves PEL untouched on transient failure, ACKs+dead-letter on hard failure

### Embedder binary

- [X] T031 [US1] Create `cmd/embedder/main.go` and `cmd/embedder/main_test.go` mirroring the shape of `cmd/cron/main.go`: load `LoadEmbedder` config, init pgxpool + redis + qdrant clients via `internal/infra/...`, expose `/healthz` and `/metrics` on `EMBEDDER_HEALTH_PORT`, run worker, handle SIGINT/SIGTERM for graceful shutdown; `main_test.go` is a smoke test asserting `run()` returns cleanly when context is cancelled

### Observability + e2e

- [ ] T032 [US1] Add per-namespace Prometheus metrics in `internal/embedder/service.go` registration: `catalog_pending_total{namespace}` (gauge), `catalog_inflight_total{namespace}` (gauge), `catalog_deadletter_total{namespace}` (gauge), `catalog_items_embedded_total{namespace,strategy_id,strategy_version}` (counter), `catalog_embed_duration_seconds{namespace,strategy_id,strategy_version}` (histogram), `catalog_embed_failures_total{namespace,strategy_id,strategy_version,reason}` (counter), `catalog_strategy_work_volume_total{namespace,strategy_id,strategy_version,unit}` (counter, V1 sets `unit="tokens_processed"`) per [research.md R10](./research.md#r10-observability-indicators-fr-014-fr-015-sc-007); add unit tests for metric registration / increment in `internal/embedder/service_test.go`
- [ ] T033 [US1] Add structured `slog` logging in `internal/catalog/service.go` and `internal/embedder/{service,worker}.go` matching the existing log format used by `internal/ingest` and `internal/compute`
- [ ] T034 [US1] Create `e2e/catalog_test.go` (`-tags=e2e`) covering US1 acceptance scenarios 1–4: enable catalog → ingest sample posts → wait for drain → assert Qdrant point count + payload tags → assert recommendations include catalog items
- [ ] T035 [US1] Update `Makefile` `test-e2e-heavy` target to include the new `e2e/catalog_test.go`
- [ ] T036 [P] [US1] Update [/CLAUDE.md](../../CLAUDE.md) REST API table — add `POST /v1/namespaces/{ns}/catalog` data-plane row per [contracts/rest-api.md](./contracts/rest-api.md)

**Checkpoint**: US1 fully functional. A new client can ingest raw posts and the cycle ingest→embed→recommend works without operator intervention beyond the initial enable. SC-001, SC-002, SC-003, SC-005 all reachable; demo-able as MVP.

---

## Phase 4: User Story 2 — Operator enables and configures the embedding strategy per namespace (Priority: P2)

**Goal**: Operator can enable catalog auto-embedding for a namespace through the admin surface, explicitly select a `(strategy_id, strategy_version)`, and the system validates dimension against `embedding_dim`. BYOE writes are rejected with 409 when catalog is enabled.

**Independent Test**: Configure two namespaces with different strategies; ingest into both; confirm each namespace's items carry the correct `strategy_version` payload tag and the dense vectors land in their respective Qdrant collections at the right dimension. PUT-ing a BYOE vector to a catalog-enabled namespace returns 409.

### Admin endpoints

- [ ] T037 [P] [US2] Add `internal/admin/catalog_handler.go` and `internal/admin/catalog_handler_test.go` exposing `GET /api/admin/v1/namespaces/{ns}/catalog` per [contracts/rest-api.md](./contracts/rest-api.md): joins nsconfig + Redis `XLEN`/`XPENDING` + Postgres state counts + last `batch_run_logs` row + `embedstrategy.Registry.List()` filtered to namespace `embedding_dim`
- [ ] T038 [P] [US2] Add `PUT /api/admin/v1/namespaces/{ns}/catalog` handler/test in the same files (or `catalog_put_handler.go` if file gets large): validates strategy via `nsconfig.Service` (T014), persists, returns 200/400 with the dim-mismatch body shape from [contracts/rest-api.md](./contracts/rest-api.md)
- [ ] T039 [US2] Wire the two admin routes into `cmd/admin/main.go` and update `cmd/admin/main_test.go` to assert registration

### BYOE source-of-truth precedence guard (FR-018)

- [ ] T040 [US2] Update `internal/recommend/handler.go` PUT object embedding handler — add a one-line `nsconfig.GetByNamespace(ns).CatalogEnabled` lookup before the existing dim validation; on `true`, return 409 with body `{"error":"namespace uses catalog auto-embedding; BYOE writes for object dense vectors are not accepted"}` per [research.md R8](./research.md#r8-source-of-truth-conflict-policy-fr-018-assumption-source-of-truth-precedence)
- [ ] T041 [US2] Add 409-path test cases in `internal/recommend/handler_test.go`: catalog enabled → 409, catalog disabled → existing 204 success path

### Frontend (web/admin)

- [ ] T042 [P] [US2] Add `Catalog` config form to `web/admin/src/...` namespace detail page: enable toggle, strategy id/version selector populated from `available_strategies`, max-attempts/max-content-bytes inputs, dim-mismatch error display matching the API body
- [ ] T043 [P] [US2] Add `Catalog` status panel to the namespace detail page rendering backlog counts, last-run summary, and active strategy identifier+version

### Integration

- [ ] T044 [US2] Add e2e coverage in `e2e/catalog_test.go` for two-namespace isolation (US2 acceptance #1) and the BYOE 409 (FR-018 + R8): create ns_a with strategy v1@dim128, ns_b with strategy v1@dim256 (assuming T019 registers the dim variant), ingest into both, assert vectors land in the right collection at the right dim; PUT BYOE to ns_a returns 409
- [ ] T045 [P] [US2] Update [/CLAUDE.md](../../CLAUDE.md) REST API table — add `GET` and `PUT /api/admin/v1/namespaces/{ns}/catalog` admin-plane rows; document the BYOE 409 behaviour change in the existing PUT object embedding row's description

**Checkpoint**: US2 fully functional. Multi-tenant deployments can route different namespaces to different strategies. SC-005 + SC-009 (forward-compat) testable. Admin UI is fully operable.

---

## Phase 5: User Story 3 — Operator triggers a namespace-wide re-embed when the embedding strategy or its version changes (Priority: P3)

**Goal**: Operator can trigger a namespace-wide re-embed; new ingests during the transition use the new active strategy version immediately while existing items keep their old tag until processed. Operator can inspect dead-letter items and re-drive them.

**Independent Test**: Populate a namespace with N items at strategy `v1`, register a fake `v2` strategy in test, bump the namespace to `v2`, trigger re-embed, confirm: progress is observable; new ingests during the window land tagged `v2`; recommendations continue serving without error throughout; final state has every item at `v2`. Bulk-redrive on dead-letter items returns them to `pending`.

### Re-embed orchestration

- [ ] T046 [P] [US3] Add `POST /api/admin/v1/namespaces/{ns}/catalog/re-embed` handler+test in `internal/admin/catalog_reembed_handler.go` (and `_test.go`): inserts `batch_run_logs` row with `trigger_source='admin'`, returns 202 + `Location` header per [contracts/rest-api.md](./contracts/rest-api.md); 409 when an existing run is `running` per [research.md R6](./research.md#r6-re-embed-trigger-mechanism-operator-initiated-namespace-wide)
- [ ] T047 [US3] Add re-embed orchestration service in `internal/admin/catalog_reembed_service.go` (+ `_test.go`): `SELECT id FROM catalog_items WHERE namespace=$1 AND (strategy_version <> $2 OR strategy_version IS NULL) AND state IN ('embedded','failed','dead_letter')`, sets `state='pending'`, `attempt_count=0`, then bulk-XADDs to `catalog:embed:{ns}` (transaction-scoped to keep enqueue idempotent on retry)
- [ ] T048 [US3] Add re-embed completion watcher goroutine in `cmd/embedder/main.go` (or a new `internal/embedder/reembed_watcher.go` file with test): polls `SELECT count(*) FROM catalog_items WHERE namespace=$1 AND strategy_version <> $2 AND state IN ('pending','in_flight','failed')` every 5 s for any namespace with an open `batch_run_logs` row of phase `embed_reembed`; on zero, marks the row complete with timing metadata

### Item browser + redrive endpoints (FR-014, FR-016, SC-008)

- [ ] T049 [P] [US3] `GET /api/admin/v1/namespaces/{ns}/catalog/items` handler+test (paginated browse, state filter, no `content` projection)
- [ ] T050 [P] [US3] `GET /api/admin/v1/namespaces/{ns}/catalog/items/{id}` handler+test (full record including `content` and `metadata`)
- [ ] T051 [P] [US3] `POST /api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive` handler+test (single redrive: state→pending, attempt_count=0, XADD)
- [ ] T052 [P] [US3] `POST /api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter` handler+test (bulk redrive of every dead-letter row in the namespace, satisfies SC-008)
- [ ] T053 [P] [US3] `DELETE /api/admin/v1/namespaces/{ns}/catalog/items/{id}` handler+test (Postgres delete + Qdrant point removal via existing recommend object-deletion path; FR-017)

### Wiring

- [ ] T054 [US3] Wire all five new admin routes into `cmd/admin/main.go` and assert registration in `cmd/admin/main_test.go`

### Frontend (web/admin)

- [ ] T055 [P] [US3] Add re-embed trigger button to namespace detail page; show in-progress state from `last_run` polling
- [ ] T056 [P] [US3] Add catalog items browser with state filter, pagination, and per-item drill-down to full record
- [ ] T057 [P] [US3] Add bulk redrive button on the items browser when filter=`dead_letter`

### Integration

- [ ] T058 [US3] Add e2e coverage in `e2e/catalog_test.go` for the full US3 flow: register a `v2` test strategy in test setup, populate namespace at `v1`, bump to `v2`, trigger re-embed, ingest a new item mid-flight and assert it lands tagged `v2` directly (Q2 transition semantics), assert recommend serves OK throughout, assert final state is fully `v2`, assert pause/cancel preserves partial progress
- [ ] T059 [P] [US3] Update [/CLAUDE.md](../../CLAUDE.md) REST API table — add the six new admin-plane rows from [contracts/rest-api.md](./contracts/rest-api.md)

**Checkpoint**: US3 fully functional. SC-006, SC-007, SC-008 all testable. Operators can run the full lifecycle of a namespace including model upgrades and dead-letter recovery.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Cross-cutting work that touches multiple user stories or is best done after the surface stabilises.

- [ ] T060 [P] Extend `internal/architecture/imports_test.go` to assert: `internal/catalog/` does NOT import `internal/embedder/`, and vice versa; both MAY import `internal/core/embedstrategy/` and `internal/infra/...`; admin MAY import `embedstrategy` (forward-compat seam)
- [ ] T061 [P] Update [/AGENTS.md](../../AGENTS.md) to mention `cmd/embedder` and the new domains in the "Three Binaries" section (renaming to "Four Binaries" with the same prose pattern as the existing CLAUDE.md update)
- [ ] T062 [P] Update [/CLAUDE.md](../../CLAUDE.md) "Three Binaries" section to "Four Binaries" with the embedder description and add `internal/catalog`, `internal/embedder`, `internal/core/embedstrategy` to the Domain Organization table
- [ ] T063 [P] Update `Makefile` `coverage-check-all` target to enforce per-package coverage minima for `internal/catalog`, `internal/embedder`, `internal/core/embedstrategy`
- [ ] T064 Run `make lint` and resolve any golangci-lint findings introduced by the new code
- [ ] T065 Run `make test-race` to detect data races in `internal/embedder/worker` (most likely place given concurrent goroutines) and fix any findings
- [ ] T066 Add a benchmark `BenchmarkHashingEmbed_1KiB` in `internal/embedder/hashing_test.go` that fails the test if p95 over 1000 iterations exceeds 5 ms (matches plan Performance Goals)
- [ ] T067 Run `quickstart.md` end-to-end manually against a fresh `make up` stack to validate the operator workflow (ingest → embed → re-embed → dead-letter redrive → disable → cleanup)

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup, T001–T004)**: No dependencies. Start immediately.
- **Phase 2 (Foundational, T005–T016)**: After Phase 1. **BLOCKS all user stories.**
- **Phase 3 (US1, T017–T036)**: After Phase 2. **MVP slice — independently testable and demoable.**
- **Phase 4 (US2, T037–T045)**: After Phase 2. Depends on Phase 3 only for shared admin scaffolding (none — admin domain already exists). Can run in parallel with Phase 5 if staffed.
- **Phase 5 (US3, T046–T059)**: After Phase 2. Re-embed completion watcher (T048) lives in `cmd/embedder` so it benefits from Phase 3 having wired the binary, but the dependency is mechanical (file present), not logical. Can run in parallel with Phase 4 if staffed.
- **Phase 6 (Polish, T060–T067)**: After all desired user stories.

### Critical task chains within Phase 2

- T008 (types) → T009 (registry) → T010 (registry_test)
- T005 (migration 010) and T006 (migration 011) are independent of each other and can run in parallel
- T011–T015 (nsconfig extensions) are sequential because they touch the same files
- T014 (nsconfig service strategy validation) depends on T009 (registry exists)
- T016 (config) is parallel with everything else in Phase 2

### Critical task chains within Phase 3 (US1)

- T017 (tokenizer), T018 (hashing), T020 (docs.go), T021 (catalog docs), T022 (catalog types), T027 (embedder types) are all parallelisable
- T019 (strategy.go init) depends on T018 + T009
- T023 (catalog repo) depends on T005 + T022
- T024 (catalog service) depends on T023 + T011 (nsconfig has CatalogEnabled)
- T025 (catalog handler) depends on T024
- T026 (cmd/api wire) depends on T025
- T028 (embedder repo) depends on T005
- T029 (embedder service) depends on T028 + T019 (strategy registered)
- T030 (embedder worker) depends on T029
- T031 (cmd/embedder main) depends on T030 + T016
- T032 (metrics) depends on T029, T030 — touches their files
- T033 (logging) parallel to T032 — different concern, sometimes same files
- T034 (e2e) depends on T026 + T031
- T035 (Makefile) depends on T034
- T036 (CLAUDE.md row) parallel with everything in Phase 3

### Within Phase 4 (US2)

- T037, T038 in parallel (both modify admin handler files but different endpoints; coordinate file split)
- T039 depends on T037 + T038
- T040, T041 are sequential (handler then test in same file)
- T042, T043 parallel (different frontend components)
- T044 depends on T039 + T040
- T045 parallel with code work

### Within Phase 5 (US3)

- T046–T053 admin endpoints largely parallelisable across files (each in its own handler file)
- T047 depends on T046
- T048 depends on T031 (cmd/embedder main exists)
- T054 depends on T046 + T049–T053
- T055–T057 frontend parallel
- T058 depends on T054 + T048
- T059 parallel with code work

---

## Parallel Examples

### Foundational phase, parallel kickoff

```bash
# All four can start simultaneously (different files, no internal deps):
Task: "Migration 010 catalog_items in migrations/010_catalog_items.{up,down}.sql"   # T005
Task: "Migration 011 namespace_configs catalog columns"                              # T006
Task: "embedstrategy package docs.go"                                                # T007
Task: "embedstrategy package types.go"                                               # T008
```

### US1 kickoff after Phase 2 lands

```bash
# Six parallel kickoff tasks (all different files, all independent):
Task: "internal/embedder/tokenizer.go + tokenizer_test.go"                           # T017
Task: "internal/embedder/hashing.go + hashing_test.go"                               # T018
Task: "internal/embedder/docs.go"                                                    # T020
Task: "internal/catalog/docs.go"                                                     # T021
Task: "internal/catalog/types.go"                                                    # T022
Task: "internal/embedder/types.go"                                                   # T027
```

### US2 vs US3 in parallel team strategy

```bash
# After Phase 2 + Phase 3 land, US2 and US3 are independent:
Developer A: T037–T045 (US2 — admin config + BYOE 409 + frontend)
Developer B: T046–T059 (US3 — re-embed + items browser + frontend)
```

---

## Implementation Strategy

### MVP first (US1 only)

1. Phase 1 (Setup): T001–T004 — half a day.
2. Phase 2 (Foundational): T005–T016 — 2–3 days. Most parallelisable phase.
3. Phase 3 (US1): T017–T036 — the bulk of the feature, 5–8 days.
4. **Stop and demo**: catalog ingest → auto-embed → recommendation works end-to-end. SC-001, SC-002, SC-003, SC-005 measurable. Constitution checks pass except the documented complexity-tracking violation (cmd/embedder count).

### Incremental delivery after MVP

1. Add US2 (T037–T045): operators can configure per-namespace + BYOE 409. SC-009 (forward-compat) measurable via test that registers a fake second strategy.
2. Add US3 (T046–T059): re-embed + dead-letter ops. SC-006, SC-007, SC-008 measurable.
3. Polish phase (T060–T067): architecture tests, perf benchmark, docs alignment.

### Parallel team strategy

With three engineers, after Phase 2 lands:

- Engineer A: drives US1 to completion (it is the longest phase but also the MVP).
- Engineer B: starts US2 once Phase 3 is far enough along (T026 wired) since US2 reuses the same admin scaffolding.
- Engineer C: starts US3 once US2's admin handler split is decided (T037–T039 file structure) so the new admin files do not conflict.

---

## Notes

- Per Constitution II, `_test.go` is mandatory for every `service.go`/`repository.go`/`worker.go`/`job.go`. The tasks above bundle the production file with its test file as a single unit (e.g. T023 = `repository.go` + `repository_test.go`) — this keeps the task count tractable while still tracking the constitutional requirement.
- All comments in code MUST be in English (Constitution I) and `docs.go` is the only place for package-level prose.
- `e2e/catalog_test.go` is built behind `-tags=e2e`; everyday `make test` does not run it. Use `make test-e2e-heavy`.
- The constitution PATCH bump acknowledging `cmd/admin` and `cmd/embedder` is **out of scope for this feature**; do it as a separate change set whose only job is to update [/.specify/memory/constitution.md](../../.specify/memory/constitution.md) Architecture Constraints from "exactly two binaries" to "exactly four binaries (`cmd/api`, `cmd/cron`, `cmd/admin`, `cmd/embedder`)".
- Stop at any checkpoint to validate that user story end-to-end before continuing.
- Avoid: cross-domain imports between `catalog` and `embedder` (use `core/embedstrategy` as the shared seam — T060 enforces this in test).
