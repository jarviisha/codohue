# Tasks: Backend Audit Remediation

**Input**: Design documents from `/specs/007-backend-audit-remediation/`
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Required by FR-021 and Constitution 2.0.0. Write the listed tests first, confirm they fail for the audited behavior, and keep a direct `_test.go` companion for every changed business-logic file.

**Organization**: Tasks are grouped by user story so each outcome remains independently testable. Production rollout still follows the four release gates in `plan.md`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel because it changes different files and has no dependency on another incomplete task in the phase
- **[Story]**: User story from `spec.md`
- Every task names the exact file paths it changes
- No task may modify `web/admin`

## Phase 1: Setup (Shared Release Gates)

**Purpose**: Establish the repeatable dependency-security gate used by every release.

- [X] T001 Pin `govulncheck` and add a `make vuln` loop covering every entry in `GO_MODULES` in `Makefile`
- [X] T002 Add a blocking vulnerability-scan job that invokes the pinned `make vuln` target after T001 in `.github/workflows/ci.yml`

---

## Phase 2: Foundational (Shared Configuration, Metrics, and Fixtures)

**Purpose**: Add shared configuration, bounded observability, and integration fixtures required across the stories.

**⚠️ CRITICAL**: Complete this phase before implementing user-story behavior.

- [X] T003 [P] Add and validate stream-retention enable/interval settings and the dedicated observability token in `internal/config/config.go`, `internal/config/config_test.go`, `.env.example`, and `.env.app.example`
- [X] T004 [P] Define bounded-cardinality retention, reclaim, stale-generation, lifecycle, and repair metrics with registration tests in `internal/infra/metrics/metrics.go`, `internal/infra/metrics/metrics_test.go`, and `internal/infra/metrics/register_test.go`
- [X] T005 [P] Add reusable Redis group/PEL, Qdrant collection, lifecycle-generation, dependency-fault, and zero-mutation assertions in `e2e/helpers_test.go`

**Checkpoint**: Shared release gates and test infrastructure are ready.

---

## Phase 3: User Story 1 - Keep the service secure and available (Priority: P1) 🎯 MVP

**Goal**: Remove reachable dependency vulnerabilities and reclaim only completed stream history while preserving all unfinished work and surfacing Redis-capacity publish failures.

**Independent Test**: Run the pinned vulnerability scan, then process at least ten retention windows across event, catalog, and generation-1 embed streams with consumer outages and multiple groups; every successful pass leaves zero entries below its safe frontier and every unfinished entry remains recoverable.

### Tests for User Story 1

- [X] T006 [P] [US1] Add no-`MAXLEN` and explicit Redis publish-error tests for every server-side embed-stream producer in `internal/catalog/service_test.go`, `internal/embedder/recovery_sweeper_test.go`, and `internal/admin/catalog_ops_service_test.go`
- [X] T007 [P] [US1] Add no-`MAXLEN` and publish-error compatibility tests for event and catalog producers in `sdk/go/redistream/producer_test.go` and `sdk/go/redistream/catalog_test.go`
- [X] T008 [P] [US1] Add Redis 7 tests for multi-group safe frontiers, pending and empty PELs, no-group and malformed progress, unexpected groups, inspection failures, dry runs, exact `XTRIM MINID`, and the zero-below-frontier postcondition in `internal/infra/redis/retention_test.go`
- [X] T009 [P] [US1] Add disabled, dry-run, enabled, cancellation, and one-minute global-stream retention-loop tests in `cmd/api/main_test.go`
- [X] T010 [P] [US1] Add generation-1-only embed-stream discovery, independent retention state, and clean shutdown tests in `cmd/embedder/main_test.go`
- [X] T011 [US1] Add the ten-window multi-group load/outage, Redis-capacity backpressure, exact-bound, and recovery scenario for all three stream kinds in `e2e/stream_retention_test.go`

### Implementation for User Story 1

- [X] T012 [US1] Upgrade vulnerable `pgx`, `golang.org/x/text`, and gRPC lines to fixed compatible releases and reconcile checksums in `go.mod`, `go.sum`, `pkg/codohuetypes/go.mod`, `sdk/go/go.mod`, `sdk/go/go.sum`, `sdk/go/redistream/go.mod`, and `sdk/go/redistream/go.sum`
- [X] T013 [US1] Remove every producer-side stream length cap while preserving explicit Redis publish errors in `internal/catalog/service.go`, `internal/embedder/recovery_sweeper.go`, `internal/admin/catalog_ops_service.go`, `sdk/go/redistream/producer.go`, and `sdk/go/redistream/catalog.go`
- [X] T014 [US1] Implement fail-closed group discovery, minimum safe-frontier calculation, dry-run inspection, and exact `XTRIM MINID` with no claimed progress on failure in `internal/infra/redis/retention.go`
- [X] T015 [US1] Instrument stream length, pending, undelivered, frontier, exact trimmed count, unexpected groups, and retention failures in `internal/infra/redis/retention.go` using `internal/infra/metrics/metrics.go`
- [X] T016 [US1] Wire disabled-by-default one-minute retention for `codohue:events` and `codohue:catalog` with clean cancellation in `cmd/api/main.go`
- [X] T017 [US1] Wire independent retention for existing generation-1 embed streams only, without lifecycle-qualified discovery, in `cmd/embedder/main.go`
- [X] T018 [US1] Configure Redis `noeviction`, explicit write-failure behavior, and retention rollout flags in `docker-compose.yml`, `docker-compose.app.yml`, and `docker-compose.prod.yml`

**Checkpoint**: Dependency scanning is clean; completed history has an exact consumer-progress bound and unfinished work survives outages.

---

## Phase 4: User Story 2 - Preserve namespace lifecycle integrity (Priority: P1)

**Goal**: Fence every writer with durable generations and leases so delete/reset cannot be followed by stale writes or same-name lifecycle contamination.

**Independent Test**: Run 100 concurrent delete/reset/recreate trials across HTTP, streams, embedder, cron, BYOE, object metadata, Redis, PostgreSQL, and Qdrant; no post-success state survives, partial cleanup stays durably fenced, and generation-less work is accepted only for grandfathered generation 1.

### Tests for User Story 2

- [X] T019 [P] [US2] Add lifecycle persistence, generation increment, durable tombstone, deleting-state resume, and system-reset restart tests in `internal/core/nslifecycle/repository_test.go`
- [X] T020 [P] [US2] Add global/namespace lease ordering, shared/exclusive exclusion, post-lock generation reread, context propagation, and legacy-gate closure tests in `internal/core/nslifecycle/service_test.go`
- [X] T021 [P] [US2] Add generation-1 legacy and generation-2+ Redis/Qdrant physical-name tests for every logical kind in `internal/core/nslifecycle/names_test.go`
- [X] T022 [P] [US2] Add closed-gate, bounded deletion, dependency-failure, retry, and current-generation protection tests in `internal/core/nslifecycle/janitor_test.go`
- [X] T023 [P] [US2] Add golden and client compatibility tests for additive namespace generation fields in `pkg/codohuetypes/golden_test.go` and `sdk/go/namespace_test.go`
- [X] T024 [P] [US2] Add active-generation stamping and legacy-wire compatibility tests in `internal/ingest/event_publisher_test.go`, `internal/embedder/event_publisher_test.go`, `cmd/api/catalog_stream_adapter_test.go`, `sdk/go/redistream/producer_test.go`, and `sdk/go/redistream/catalog_test.go`
- [X] T025 [P] [US2] Add active-state, exact-generation, lifecycle-lease-required mutation, and read-only ID lookup tests in `internal/ingest/service_test.go`, `internal/catalog/service_test.go`, `internal/objects/service_test.go`, `internal/recommend/service_test.go`, `internal/nsconfig/repository_test.go`, `internal/nsconfig/service_test.go`, `internal/core/idmap/repository_test.go`, and `internal/core/idmap/service_test.go`
- [X] T026 [P] [US2] Add stale-generation ACK/drop, grandfathered generation-1 acceptance, closed-gate rejection, and lifecycle-store retry tests in `internal/ingest/worker_test.go`, `internal/ingest/catalog_worker_test.go`, and `internal/embedder/worker_test.go`
- [X] T027 [P] [US2] Add lifecycle fencing and compute-lock ordering tests for scheduled compute, embed service, recovery, and re-embed watcher mutations in `internal/compute/job_test.go`, `internal/compute/service_test.go`, `internal/embedder/service_test.go`, `internal/embedder/recovery_sweeper_test.go`, and `internal/embedder/reembed_watcher_test.go`
- [X] T028 [P] [US2] Add direct generation-aware collection-name regression tests in `internal/infra/qdrant/collections_test.go`
- [X] T029 [P] [US2] Add direct generation-aware trending-key regression tests in `internal/infra/redis/trending_test.go`
- [X] T030 [P] [US2] Add generation-aware cache and embed-repository name-resolution tests in `internal/recommend/repository_test.go` and `internal/embedder/repository_test.go`
- [X] T031 [P] [US2] Add partial-delete, verified absence, restart-resume, recreation blocking, persistent reset, idempotent legacy closure, adoption-evidence, and command-registration tests in `internal/admin/service_test.go`, `internal/admin/repository_test.go`, `cmd/admin/lifecycle_test.go`, and `cmd/admin/main_test.go`
- [X] T032 [P] [US2] Add Release-3 generation-qualified embed-stream discovery and retention-loop tests in `cmd/embedder/main_test.go`
- [ ] T033 [US2] Add the 100-iteration writer/delete/reset/recreate race, partial-store failure, legacy-envelope closure, and stale-generation assertions in `e2e/namespace_lifecycle_test.go`

### Implementation for User Story 2

- [X] T034 [US2] Create durable namespace/system lifecycle tables, monotonic generation, generation-1 backfill, deletion/reset states, legacy allowance, closure timestamp, constraints, and reversible removal in `migrations/024_namespace_lifecycle.up.sql` and `migrations/024_namespace_lifecycle.down.sql`
- [X] T035 [US2] Add orphan preflight, `NOT VALID` namespace foreign keys followed by validation, cascade rules, and the catalog keyset index in `migrations/025_namespace_integrity_fks.up.sql` and `migrations/025_namespace_integrity_fks.down.sql`
- [X] T036 [US2] Define lifecycle entities and implement PostgreSQL persistence for state transitions, generations, errors, reset state, and atomic legacy closure in `internal/core/nslifecycle/docs.go`, `internal/core/nslifecycle/types.go`, and `internal/core/nslifecycle/repository.go`
- [X] T037 [US2] Implement fixed-order global/namespace advisory leases, post-acquisition state rereads, context lease assertions, and idempotent legacy-gate orchestration in `internal/core/nslifecycle/service.go`
- [X] T038 [US2] Implement centralized generation-1 legacy and generation-2+ Redis/Qdrant physical naming in `internal/core/nslifecycle/names.go`
- [X] T039 [US2] Persist and return namespace generations during create, read, delete, and recreation operations in `internal/nsconfig/types.go`, `internal/nsconfig/repository.go`, and `internal/nsconfig/service.go`
- [X] T040 [US2] Add generation fields to event, catalog, embed, and namespace provisioning DTOs without breaking legacy JSON in `pkg/codohuetypes/event.go`, `pkg/codohuetypes/catalog.go`, `pkg/codohuetypes/stream.go`, `sdk/go/namespace.go`, and `sdk/go/options.go`
- [X] T041 [US2] Stamp the active namespace generation on HTTP event, Redis SDK catalog, server catalog-to-embed, and retry publications in `internal/ingest/event_publisher.go`, `sdk/go/redistream/producer.go`, `sdk/go/redistream/catalog.go`, `cmd/api/catalog_stream_adapter.go`, and `internal/embedder/event_publisher.go`
- [X] T042 [US2] Enforce exact generation matching, generation-1-only missing-generation acceptance, closed-gate rejection, stale ACK/drop, and transient lifecycle retries in `internal/ingest/worker.go`, `internal/ingest/catalog_worker.go`, and `internal/embedder/worker.go`
- [X] T043 [US2] Acquire shared lifecycle leases before data-plane PostgreSQL, Redis, Qdrant, mapping, and cache mutations in `internal/ingest/service.go`, `internal/catalog/service.go`, `internal/objects/service.go`, `internal/recommend/service.go`, and `internal/nsconfig/service.go`
- [X] T044 [US2] Acquire shared lifecycle leases before cron, embedding, recovery, and re-embed mutations while preserving compute-lock order in `internal/compute/job.go`, `internal/compute/service.go`, `internal/embedder/service.go`, `internal/embedder/recovery_sweeper.go`, and `internal/embedder/reembed_watcher.go`
- [X] T045 [US2] Route current-generation Qdrant collections through the lifecycle name resolver in `internal/infra/qdrant/collections.go`
- [X] T046 [US2] Route current-generation trending keys through the lifecycle name resolver in `internal/infra/redis/trending.go`
- [X] T047 [US2] Route recommendation caches and embed streams through lifecycle-resolved physical names in `internal/recommend/repository.go` and `internal/embedder/repository.go`
- [X] T048 [US2] Require a matching lifecycle lease for ID-map creation and keep lookup paths read-only in `internal/core/idmap/repository.go` and `internal/core/idmap/service.go`
- [X] T049 [US2] Implement resumable namespace delete/reset, verified cross-store cleanup, durable failures, recreation generation increments, and `lifecycle disable-legacy-envelopes --all --adoption-evidence` in `internal/admin/repository.go`, `internal/admin/service.go`, `cmd/admin/lifecycle.go`, and `cmd/admin/main.go`
- [X] T050 [US2] Extend embed retention discovery to generation-qualified streams only after the lifecycle resolver is available in `cmd/embedder/main.go`
- [X] T051 [US2] Implement bounded deleted-generation Redis/Qdrant cleanup that requires the globally closed legacy gate in `internal/core/nslifecycle/janitor.go`

**Checkpoint**: Delete/reset success is authoritative, partial cleanup remains fenced, and old lifecycle work cannot mutate a recreated namespace.

---

## Phase 5: User Story 3 - Keep recommendation state consistent over time (Priority: P1)

**Goal**: Remove expired owned state, compare full embedding-strategy identities, keep scores finite, and repair migration-022 identities without losing recomputable or client-owned vectors.

**Independent Test**: Exercise empty active windows, same-version strategy replacement, boundary/future timestamps, cross-namespace/entity collisions, ambiguous repair points, staged failures, resume, and coordinated restore; stale owned state disappears, scores stay finite, ambiguous repair mutates nothing, and every resolvable vector remains reachable.

### Tests for User Story 3

- [ ] T052 [P] [US3] Add configured-namespace enumeration, zero-event scheduling, and repository failure tests in `internal/compute/job_test.go` and `internal/compute/repository_test.go`
- [ ] T053 [P] [US3] Add direct empty-keep-set sparse cleanup and ownership-boundary tests in `internal/compute/cleanup_test.go`
- [ ] T054 [P] [US3] Add direct item2vec/SVD/catalog/BYOE dense ownership-matrix tests in `internal/compute/dense_test.go` and `internal/compute/user_dense_test.go`
- [ ] T055 [P] [US3] Add same-version/different-strategy reset, stale-count, and immutable-run tuple tests in `internal/admin/catalog_ops_repository_test.go` and `internal/admin/catalog_ops_service_test.go`
- [ ] T056 [P] [US3] Add tuple-based selection, progress, and completion tests in `internal/embedder/repository_test.go` and `internal/embedder/reembed_watcher_test.go`
- [ ] T057 [P] [US3] Add exact five-minute boundary and beyond-boundary event timestamp tests in `internal/ingest/service_test.go`
- [ ] T058 [P] [US3] Add direct non-negative age, `[0,1]` clamp, malformed timestamp, and non-finite candidate tests in `internal/recommend/scoring_test.go`
- [ ] T059 [P] [US3] Add BYOE timestamp validation, finite response serialization, and rejected-candidate behavior tests in `internal/recommend/service_test.go` and `internal/recommend/handler_test.go`
- [ ] T060 [P] [US3] Add direct immutable-manifest, quarantine, item-state, hash, and resume-query persistence tests in `internal/core/idmap/repair_repository_test.go`
- [ ] T061 [P] [US3] Add direct ambiguous preflight, zero-mutation refusal, snapshot requirement, sparse-rebuild port, stage failure, resume, and verify tests in `internal/core/idmap/repair_service_test.go`
- [ ] T062 [P] [US3] Add collection inventory, payload/vector hashing, snapshot reference, verified copy, and old-point cleanup tests in `internal/infra/qdrant/repair_test.go`
- [ ] T063 [P] [US3] Add `audit`, `apply`, `verify`, `resume`, argument-validation, command-registration, and compute-adapter tests in `cmd/admin/idmap_repair_test.go`, `cmd/admin/idmap_repair_adapter_test.go`, and `cmd/admin/main_test.go`
- [ ] T064 [US3] Add expired-event cleanup, strategy-tuple replacement, and timestamp boundary/fuzz scenarios in `e2e/recommendation_state_test.go`
- [ ] T065 [US3] Add PostgreSQL/Qdrant collision, ambiguous-point zero-mutation, BYOE preservation, sparse rebuild, failure-resume, and verification rehearsal in `e2e/idmap_repair_test.go`

### Implementation for User Story 3

- [ ] T066 [US3] Enumerate every active configured namespace, including empty event windows, in `internal/compute/repository.go` and `internal/compute/job.go`
- [ ] T067 [US3] Clear stale sparse vectors for empty authoritative keep sets in `internal/compute/cleanup.go`
- [ ] T068 [US3] Apply the item2vec/SVD/catalog/BYOE dense ownership matrix for empty keep sets in `internal/compute/dense.go` and `internal/compute/user_dense.go`
- [ ] T069 [US3] Compare immutable `(strategy_id,strategy_version)` tuples in re-embed reset, stale counts, and batch-run state in `internal/admin/catalog_ops_repository.go` and `internal/admin/catalog_ops_service.go`
- [ ] T070 [US3] Compare immutable strategy tuples in embed selection, progress, and completion in `internal/embedder/repository.go` and `internal/embedder/reembed_watcher.go`
- [ ] T071 [US3] Enforce the shared five-minute future-skew rule for event and BYOE `object_created_at` inputs in `internal/ingest/types.go`, `internal/ingest/service.go`, `internal/recommend/types.go`, and `internal/recommend/handler.go`
- [ ] T072 [US3] Centralize non-negative freshness, clamp multipliers to `[0,1]`, and exclude observable non-finite candidates before JSON serialization in `internal/recommend/scoring.go`, `internal/recommend/service.go`, and `internal/recommend/handler.go`
- [ ] T073 [US3] Add composite mapping uniqueness, durable repair run/item manifests, resumable states, and safe down-preflight support in `migrations/026_id_mapping_repair.up.sql` and `migrations/026_id_mapping_repair.down.sql`
- [ ] T074 [US3] Implement repair-run/item persistence, immutable manifest hashing, quarantine reports, and resume queries in `internal/core/idmap/repair_repository.go`
- [ ] T075 [US3] Implement Qdrant inventory, payload/vector hashing, snapshot reference validation, verified point copying, and old-point cleanup primitives in `internal/infra/qdrant/repair.go`
- [ ] T076 [US3] Implement read-only audit plus globally fenced apply, verify, and resume orchestration that halts before mutation on unresolved evidence and invokes sparse rebuild through a narrow port in `internal/core/idmap/repair_service.go`
- [ ] T077 [US3] Compose the sparse-rebuild adapter over `internal/compute` and register `idmap-repair audit|apply|verify|resume` under the existing admin binary in `cmd/admin/idmap_repair_adapter.go`, `cmd/admin/idmap_repair.go`, and `cmd/admin/main.go`
- [ ] T078 [US3] Add a duplicate preflight that refuses migration-022 rollback before any constraint mutation in `migrations/022_id_mappings_composite.down.sql` and `migrations/026_id_mapping_repair.down.sql`
- [ ] T079 [US3] Document coordinated PostgreSQL/Qdrant snapshots, audit, apply, resume, verification, and all-store restore in `deploy/idmap-repair-runbook.md`

**Checkpoint**: Recommendation state is current and finite; identity repair is lossless, resumable, and stops before mutation when identity is ambiguous.

---

## Phase 6: User Story 4 - Fail writes and reads honestly (Priority: P2)

**Goal**: Return stable failures for unavailable configuration and incomplete cleanup, reject nonexistent namespaces, and make compound PostgreSQL writes atomic.

**Independent Test**: Inject config-store, namespace-resolution, dense-delete, validation, and attribution failures; recommendation paths return stable 404/503 without cache access, nonexistent namespaces mutate nothing, and partial writes or cleanup never report success.

### Tests for User Story 4

- [ ] T080 [P] [US4] Add config-not-found/unavailable tests proving Recommend, Rank, and Trending resolve configuration before cache access and never cache default-backed results in `internal/recommend/service_test.go`
- [ ] T081 [P] [US4] Add global-admin missing/deleted/deleting namespace mutation and stable 404/409/503 contract tests in `internal/ingest/handler_test.go`, `internal/catalog/handler_test.go`, `internal/objects/handler_test.go`, and `internal/recommend/handler_test.go`
- [ ] T082 [US4] Add sparse/dense/metadata deletion fault injection, joined-error, NotFound, and safe-retry tests in `internal/recommend/service_test.go` and `internal/recommend/repository_test.go`
- [ ] T083 [P] [US4] Add effective-dimension validation and base/catalog transaction rollback tests in `internal/nsconfig/repository_test.go`, `internal/nsconfig/repository_unit_test.go`, and `internal/nsconfig/service_test.go`
- [ ] T084 [P] [US4] Add content/author transaction rollback and same-content attribution-update tests in `internal/catalog/repository_test.go` and `internal/catalog/service_test.go`
- [ ] T085 [US4] Add stable 404/409/503, zero-mutation, zero-cache, incomplete-delete, and compound-write rollback scenarios in `e2e/honest_failures_test.go`

### Implementation for User Story 4

- [ ] T086 [US4] Resolve required namespace configuration before recommendation, rank, trending, or cache access and return stable `namespace_not_found`/`namespace_config_unavailable` errors in `internal/recommend/service.go` and `internal/recommend/handler.go`
- [ ] T087 [US4] Require active namespace resolution before event, catalog, object, and BYOE mutations even for global-admin credentials in `internal/ingest/handler.go`, `internal/catalog/handler.go`, `internal/objects/handler.go`, and `internal/recommend/handler.go`
- [ ] T088 [US4] Attempt sparse, dense, and metadata deletion stages, ignore only NotFound, join remaining failures, and preserve idempotent retries in `internal/recommend/service.go` and `internal/recommend/repository.go`
- [ ] T089 [US4] Validate the effective namespace/catalog configuration before one atomic base-and-catalog upsert transaction in `internal/nsconfig/service.go` and `internal/nsconfig/repository.go`
- [ ] T090 [US4] Persist catalog content and requested author attribution in one transaction while allowing same-content attribution updates without duplicate embed work in `internal/catalog/service.go` and `internal/catalog/repository.go`

**Checkpoint**: Clients never receive false success or a cached response based on missing configuration.

---

## Phase 7: User Story 5 - Recover every pending item fairly and reconcile without gaps (Priority: P2)

**Goal**: Advance all reclaim cursors fairly and paginate catalog reconciliation with a total-order opaque cursor.

**Independent Test**: Use at least three reclaim and catalog pages with permanently failing early entries and identical `updated_at` values; every later entry and row is visited without loss, duplication, or cursor starvation.

### Tests for User Story 5

- [ ] T091 [P] [US5] Add multi-page, empty-page, terminal `0-0`, error-retention, `NOGROUP`, ten-page-budget, and early-failure reclaim tests in `internal/ingest/worker_test.go` and `internal/ingest/catalog_worker_test.go`
- [ ] T092 [P] [US5] Add independent per-generation embed cursors, empty pages, terminal reset, error retention, ten-page budget, and permanently failing early-entry tests in `internal/embedder/recovery_sweeper_test.go`
- [ ] T093 [P] [US5] Add equal-timestamp keyset, malformed or mismatched cursor, terminal page, legacy offset, golden response, and SDK compatibility tests in `internal/catalog/repository_test.go`, `internal/catalog/handler_test.go`, `pkg/codohuetypes/golden_test.go`, and `sdk/go/catalog_http_test.go`
- [ ] T094 [US5] Add three-page reclaim and equal-timestamp catalog reconciliation scenarios in `e2e/recovery_reconciliation_test.go`

### Implementation for User Story 5

- [ ] T095 [US5] Persist returned `XAUTOCLAIM` cursors across pages and ticks, retain cursors on errors, reset only at terminal or recreated groups, and enforce ten pages per tick in `internal/ingest/worker.go`, `internal/ingest/catalog_worker.go`, and `internal/embedder/recovery_sweeper.go`
- [ ] T096 [US5] Implement versioned URL-safe `(updated_at,id)` cursors and matching keyset queries while retaining legacy offset for one window in `internal/catalog/types.go`, `internal/catalog/repository.go`, and `internal/catalog/handler.go`
- [ ] T097 [US5] Expose additive `next_cursor` response and cursor request support in `pkg/codohuetypes/catalog.go`, `pkg/codohuetypes/testdata/catalog_objects_response.golden.json`, `sdk/go/catalog_http.go`, and `sdk/go/README.md`

**Checkpoint**: Reclaim and reconciliation eventually visit every eligible item without starvation or timestamp gaps.

---

## Phase 8: User Story 6 - Expose operations safely (Priority: P3)

**Goal**: Keep public health sanitized and require a dedicated constant-time bearer credential for metrics and detailed diagnostics on both existing listeners.

**Independent Test**: For API and embedder, verify plain health never exposes raw dependency data; `details=true` and metrics return 404 without configuration, 401 for missing/invalid credentials, protected content for the observability token, and never accept the admin key as a substitute.

### Tests for User Story 6

- [ ] T098 [P] [US6] Add constant-time bearer parsing, missing/malformed header, disabled-token, and admin-key separation tests in `internal/auth/observability_test.go`
- [ ] T099 [P] [US6] Add sanitized public health, protected `details=true`, conditional authenticated metrics, 404/401, and invalid-header-on-plain-health tests in `cmd/api/healthz_test.go` and `cmd/api/main_test.go`
- [ ] T100 [P] [US6] Add the same public/protected health and conditional metrics contract tests for the embedder listener in `cmd/embedder/main_test.go`
- [ ] T101 [US6] Add public disclosure and trusted monitoring scrape scenarios for API and embedder in `e2e/observability_security_test.go`

### Implementation for User Story 6

- [ ] T102 [US6] Implement dedicated constant-time observability bearer middleware without accepting the global admin key in `internal/auth/observability.go`
- [ ] T103 [US6] Sanitize plain API `/healthz`, add protected `/healthz?details=true`, and conditionally register protected `/metrics` in `cmd/api/main.go`
- [ ] T104 [US6] Sanitize plain embedder `/healthz`, add protected `/healthz?details=true`, and conditionally register protected `/metrics` in `cmd/embedder/main.go`

**Checkpoint**: Public operational endpoints reveal no tenant labels or raw dependency errors while trusted monitoring retains detailed signals.

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Complete documentation, rollout rehearsal, compatibility gates, and repository-wide release proof.

- [ ] T105 [P] Update lifecycle generation, exact stream retention, catalog cursor, stable error, health-details, metrics-auth, and SDK usage in `README.md`, `sdk/go/README.md`, and `sdk/go/redistream/README.md`
- [ ] T106 [P] Document the four gated releases, exact-retention canary and kill switch, generation adoption window, guarded legacy closure, delete/reset enablement, and rollback boundaries in `deploy/backend-audit-remediation.md`
- [ ] T107 Add the compensating `only_state=all` re-embed procedure and namespace completion evidence template in `deploy/backend-audit-remediation.md`
- [ ] T108 Run `make lint`, `make build`, `make test`, `make test-race`, `make test-e2e`, `make compose-check`, `go mod verify`, and `make vuln`, then record every command result in `specs/007-backend-audit-remediation/verification.md`
- [ ] T109 Rehearse migrations 024-026, legacy closure, ambiguous repair refusal, repair resume, coordinated snapshot restore, and migration-down safety, then record results in `specs/007-backend-audit-remediation/verification.md`
- [ ] T110 Confirm `git diff --check` passes and the complete remediation diff contains no changes under `web/admin`, then record the scope check in `specs/007-backend-audit-remediation/verification.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies. T002 follows T001 because CI invokes the target created there.
- **Phase 2 (Foundational)**: Depends on Phase 1 and blocks all user-story implementation.
- **US1 (Phase 3)**: Starts after Phase 2 and delivers the independently deployable security and exact-retention slice.
- **US2 (Phase 4)**: Starts after Phase 2. T050 extends, rather than precedes, the generation-1 retention work from T014-T017.
- **US3 (Phase 5)**: Compute, strategy, and timestamp work starts after Phase 2; identity-repair tasks T073-T079 depend on lifecycle leases T034-T049.
- **US4 (Phase 6)**: Core fail-closed and transaction work starts after Phase 2; lifecycle-specific 409 behavior integrates after T036-T049.
- **US5 (Phase 7)**: Starts after Phase 2; reclaim behavior ships with the Release-2 retention slice, while catalog keyset pagination can ship in Release 1.
- **US6 (Phase 8)**: Starts after Phase 2 and is independent of lifecycle and identity repair.
- **Phase 9 (Polish)**: Depends on every story selected for release; T108-T110 are final gates.

### User Story Dependency Graph

```text
Setup → Foundational → US1 ─────────────┬→ US2 ──────┬→ US3 identity repair
                       │                │             └→ US4 lifecycle statuses
                       └→ US5 reclaim   │
          Foundational ├→ US3 compute/re-embed/timestamps
                       ├→ US4 fail-closed/atomic writes
                       ├→ US5 catalog keyset
                       └→ US6 observability
All selected stories → Polish and release verification
```

### Within Each User Story

- Add the story's tests first and confirm the audited behavior fails.
- Apply migrations before repository or service code that requires their schema.
- Implement types and repositories before services, workers, handlers, and binary wiring.
- Complete package tests before E2E and operational gates.
- Do not cross a production release gate until its rollback or kill-switch path is verified.

### Parallel Opportunities

- T003-T005 touch independent shared areas and can proceed in parallel after Setup.
- The `[P]` tests within each story touch distinct test files or isolated package groups and can be authored concurrently.
- US3 compute/re-embed/timestamp work can proceed while US2 lifecycle work is implemented; only identity repair waits for lifecycle leases.
- US5 catalog cursor work and US6 observability can proceed independently of retention and lifecycle implementation.
- T105 and T106 can proceed in parallel after contracts stabilize.

---

## Parallel Execution Examples

### User Story 1

```text
T006: Server producer no-MAXLEN tests
T007: Redis SDK producer no-MAXLEN tests
T008: Exact safe-frontier retention tests
T009: API retention-loop tests
T010: Generation-1 embed retention-loop tests
```

### User Story 2

```text
T019: Lifecycle repository tests
T020: Lifecycle service and lease tests
T021: Generation-aware naming tests
T022: Janitor gate tests
T023: DTO and client compatibility tests
T024: Producer generation-stamping tests
T025: Data-plane writer fencing tests
T026: Worker generation tests
T027: Background writer fencing tests
T028: Qdrant collection-name tests
T029: Redis trending-name tests
T030: Cache and embed-repository name tests
T031: Delete/reset and legacy-closure tests
T032: Generation-qualified embed retention tests
```

### User Story 3

```text
T052: Configured-namespace enumeration tests
T053: Sparse cleanup tests
T054: Dense ownership-matrix tests
T055: Admin strategy-tuple tests
T056: Embedder strategy-tuple tests
T057: Event timestamp tests
T058: Direct scoring tests
T059: BYOE and response tests
T060: Repair repository tests
T061: Repair service tests
T062: Qdrant repair tests
T063: Repair CLI and adapter tests
```

### User Story 4

```text
T080: Config fail-closed and cache tests
T081: Missing namespace handler tests
T083: Atomic namespace config tests
T084: Atomic catalog attribution tests
```

### User Story 5

```text
T091: Global-stream reclaim tests
T092: Embed-stream reclaim tests
T093: Catalog keyset cursor tests
```

### User Story 6

```text
T098: Observability authentication tests
T099: API health and metrics tests
T100: Embedder health and metrics tests
```

---

## Implementation Strategy

### MVP First (User Story 1)

1. Complete Phase 1 and Phase 2.
2. Complete T006-T018 for US1.
3. Stop and validate the vulnerability and ten-window exact-retention gates independently.
4. Deploy retention disabled, inspect frontiers, then canary generation-1 embed, catalog, and event streams in order.

### Incremental Delivery

1. Ship migration-free dependency and isolated correctness, cursor, and observability fixes from US1, US3, US4, US5, and US6 as Release 1.
2. Ship US1 retention plus US5 reclaim cursors as Release 2 with trimming disabled first.
3. Apply migrations 024-025 and ship US2 generation/lease fencing as Release 3; add generation-qualified embed retention only here.
4. Close the legacy-envelope gate with adoption evidence before enabling deleted-generation janitors.
5. Apply migration 026 and execute the US3 ID-map audit/snapshot/apply/verify workflow as Release 4 under the global lease.
6. Finish Phase 9 only after every selected release gate has evidence in `verification.md`.

### Suggested Team Split

```text
Track A: US1 exact retention and US5 reclaim fairness
Track B: US2 lifecycle ledger, leases, naming, delete/reset, legacy closure
Track C: US3 recommendation consistency and ID-map repair
Track D: US4 honest failures, US5 keyset cursor, US6 observability
```

---

## Notes

- `[P]` tasks modify independent files and can run concurrently at that point in the dependency graph.
- Every user-story task carries its `[USn]` label; Setup, Foundational, and Polish tasks do not.
- Type-only, migration-only, documentation, configuration, and composition-only files may be covered by adjacent contract or integration tests; every changed business-logic file has a direct test task.
- Commit after each task or focused logical group using the repository Conventional Commit format.
- Stop at any checkpoint to validate the story independently.
