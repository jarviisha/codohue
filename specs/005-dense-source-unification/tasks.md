---
description: "Task list for Dense Source Unification"
---

# Tasks: Dense Source Unification

**Input**: Design documents from `/specs/005-dense-source-unification/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Included. This refactor must be behavior-preserving (SC-003), so verification tasks
are required, not optional. Existing `_test.go` files are updated in place; conflict-validation
tests are deleted with their code.

**Organization**: Tasks grouped by user story. A **dual-write window** (Foundational) lets each
story migrate its own readers independently before legacy columns/fields are dropped in Polish —
matching the phased rollout in research.md R3.

## Path Conventions

Go backend at repo root (`internal/`, `cmd/`, `migrations/`); admin SPA at `web/admin/src/`.

---

## Phase 1: Setup

- [X] T001 Confirm branch `feat/compute-dense-source-unification` is checked out and create empty migration pair files `migrations/016_dense_source.up.sql` and `migrations/016_dense_source.down.sql`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Introduce `dense_source` alongside the legacy fields with dual-write/derive, so the
codebase compiles and behaves identically while each story migrates its readers independently.

**⚠️ CRITICAL**: No user story work can begin until this phase is complete.

- [X] T002 Write `migrations/016_dense_source.up.sql`: `ADD COLUMN dense_source TEXT`; backfill `dense_source = CASE WHEN catalog_enabled THEN 'catalog' ELSE dense_strategy END`; add `CHECK (dense_source IN ('disabled','item2vec','svd','byoe','catalog'))`; **keep** `dense_strategy` and `catalog_enabled` for the dual-write window
- [X] T003 Write `migrations/016_dense_source.down.sql`: drop the CHECK constraint and `DROP COLUMN dense_source`
- [X] T004 Add `DenseSource string` to `namespace.Config` (retain `DenseStrategy`/`CatalogEnabled` for now) in `internal/core/namespace/types.go`
- [X] T005 Update `internal/nsconfig/repository.go` to select `dense_source` and dual-write it alongside `dense_strategy`/`catalog_enabled` on Upsert and UpsertCatalogConfig; populate `Config.DenseSource` on every read
- [X] T006 Add `DenseSource` to `UpsertRequest` and `UpdateCatalogRequest` in `internal/nsconfig/types.go`; derive legacy fields from it when set
- [X] T007 Apply migration and verify compile: `make migrate-up && make build`

**Checkpoint**: `dense_source` is readable/writable everywhere; legacy behavior unchanged.

---

## Phase 3: User Story 1 - Choose the producer in one decision (Priority: P1) 🎯 MVP

**Goal**: Enabling catalog auto-embedding is a single choice (`dense_source='catalog'`) — no
separate toggle, no required pre-setting of another field, no conflict precondition.

**Independent Test**: On a fresh namespace, set `dense_source='catalog'` with strategy params in
one config write; confirm catalog ingest is accepted and objects auto-embed (quickstart §2).

- [X] T008 [US1] Delete `conflictsWithCatalog()` and both conflict-validation branches in `internal/nsconfig/service.go` (lines ~62, ~133)
- [X] T009 [US1] Update `cmd/admin/catalog_adapter.go` so enabling catalog sets `dense_source='catalog'` (and disabling moves it to `'disabled'`/`'byoe'` per request), removing the `Enabled` boolean coupling
- [X] T010 [P] [US1] Change the catalog ingest gate to `cfg.DenseSource == "catalog"` (was `!cfg.CatalogEnabled`) in `internal/catalog/service.go` (line ~84)
- [X] T011 [P] [US1] Change the embedder gate to `cfg.DenseSource == "catalog"` (was `!cfg.CatalogEnabled`) in `internal/embedder/service.go` (line ~168)
- [X] T012 [US1] Rename `ListCatalogEnabled` → `ListCatalogNamespaces` (query `WHERE dense_source = 'catalog'`) across `internal/nsconfig/repository.go` (line ~183/199), `internal/nsconfig/service.go` (line ~26/105), `internal/embedder/worker.go` (line ~45/162), `internal/embedder/backlog_sampler.go` (line ~123); update `internal/embedder/backlog_sampler_test.go` for the renamed lister
- [X] T013 [US1] Replace the `dense_strategy` select with a single `dense_source` dropdown (`disabled|item2vec|svd|byoe|catalog`) and remove the separate catalog-enable control in `web/admin/src/pages/ns/config/NamespaceConfigPage.tsx` (lines ~189, ~258, ~464); render strategy params only when `catalog` is selected
- [X] T014 [P] [US1] Update `web/admin/src/services/namespaces.ts` types (`dense_source` replaces `dense_strategy` + `catalog_enabled`) and `web/admin/src/pages/namespaces/CreateNamespaceDialog.tsx` (line ~61) to send `dense_source`
- [X] T015 [US1] Update tests: `internal/nsconfig/service_test.go` (no-conflict path **+ enabling `dense_source='catalog'` with mismatched `embedding_dim` still returns the dimension-mismatch 400, FR-008**), `internal/catalog/service_test.go` (ingest gate on `catalog`), `internal/embedder/service_test.go` (gate on `catalog`), `internal/embedder/worker_test.go` (renamed lister)

**Checkpoint**: Catalog enabled via one field; bootstrap requires no ordered precondition.

---

## Phase 4: User Story 2 - Conflicting producers unrepresentable (Priority: P2)

**Goal**: The contradictory "two producers" state cannot be expressed; the conflict error surface
is removed, and `catalog` remains the authoritative object-dense producer (blocks object BYOE).

**Independent Test**: Confirm no `dense_strategy_conflict` error exists anywhere; under
`dense_source='catalog'` an object BYOE PUT returns 409 while a subject BYOE PUT returns 204
(quickstart §3–4).

- [X] T016 [US2] Delete `DenseStrategyConflictError` type in `internal/nsconfig/types.go` (lines ~71–79)
- [X] T017 [US2] Remove the two conflict error-mapping blocks in `internal/admin/handler.go` (lines ~182–188, ~266–272)
- [X] T018 [P] [US2] Remove conflict plumbing in `cmd/admin/nsconfig_adapter.go` (lines ~66–70), `cmd/admin/catalog_adapter.go` (lines ~86–98), and the conflict error type in `internal/admin/types.go` (lines ~184–189)
- [X] T019 [US2] Change the BYOE object-write block to `cfg.DenseSource == "catalog" && entityType == "object"` (was `cfg.CatalogEnabled && ...`) in `internal/recommend/service.go` (line ~164)
- [X] T020 [US2] Update tests: delete conflict-rejection tests in `internal/nsconfig/service_test.go` and `internal/admin/handler_test.go`; assert object-BYOE 409 + subject-BYOE 204 under `catalog` in `internal/recommend/service_test.go`

**Checkpoint**: Conflict path is gone from code and API; catalog authority preserved.

---

## Phase 5: User Story 3 - Migration parity across all values (Priority: P3)

**Goal**: Every reader branches on `dense_source` and every prior configuration (catalog,
item2vec, svd, byoe, disabled) yields identical recommendation/embedding behavior.

**Independent Test**: Run the per-value parity table in quickstart §5 against pre/post migration.

- [X] T021 [US3] Update the Phase 2 gate (`dense_source ∈ {item2vec, svd}` runs; others skip) and the `switch cfg.DenseSource` in `internal/compute/job.go` (lines ~223, ~425)
- [X] T022 [P] [US3] Change the dense-blend gate to `cfg.DenseSource != "disabled"` (with existing `0 < alpha < 1`) in `internal/recommend/service.go` (line ~315)
- [X] T023 [P] [US3] Select `dense_source` (and stop selecting `catalog_enabled`/`dense_strategy`) in `internal/admin/repository.go` (lines ~28, ~31, ~45, ~48); update `internal/admin/repository_test.go` to assert `dense_source` is read and the legacy columns are no longer selected (Constitution II)
- [X] T024 [P] [US3] Update `internal/admin/demo.go` (`DenseSource: "disabled"`) and the display in `web/admin/src/pages/namespaces/NamespacesListPage.tsx` + `web/admin/src/pages/ns/NamespaceOverviewPage.tsx` to show `dense_source`
- [X] T025 [US3] Update `internal/compute/job_test.go`: Phase 2 trains for `item2vec`/`svd`, skips for `byoe`/`catalog`/`disabled`
- [X] T026 [P] [US3] Add migration parity tests: backfill maps all five prior configurations correctly (catalog_enabled→catalog; others→prior value) and the `CHECK` rejects an out-of-range value

**Checkpoint**: All readers on `dense_source`; behavior parity verified for every value.

---

## Phase 6: Polish & Cross-Cutting (drop legacy, finalize)

**Purpose**: Remove the dual-write scaffolding now that every reader uses `dense_source`.

> **DEFERRED (rollout-gated):** T027–T028 drop the legacy columns/fields and flip write-authority
> to `dense_source`. Per the implementation strategy + research R3, run these only after the
> dual-write build has shipped and no deployed binary still reads `dense_strategy`/`catalog_enabled`.
> The branch is fully functional and behavior-preserving in the dual-write state without them.

- [ ] T027 Create `migrations/017_drop_legacy_dense_fields.up.sql` / `.down.sql`: `DROP COLUMN catalog_enabled` and `DROP COLUMN dense_strategy` (down recreates both and backfills from `dense_source`)
- [ ] T028 Remove `DenseStrategy` and `CatalogEnabled` from `namespace.Config` (`internal/core/namespace/types.go`) and from `internal/nsconfig/types.go`; delete the dual-write code in `internal/nsconfig/repository.go` and `internal/admin/repository.go`
- [X] T029 [P] Update `CLAUDE.md` "Key Design Decisions" + nsconfig/admin notes and the REST notes wording: `dense_strategy` + `catalog_enabled` → single `dense_source` enum; document that `catalog` = object auto-embed, subject vectors external/none
- [X] T030 [P] Update admin contract/golden tests under `internal/admin` per `contracts/namespace-config.md` (assert `dense_source`, drop the two legacy fields); confirm `pkg/codohuetypes` golden suite is untouched
- [X] T031 Full gate: `make migrate-up && make lint && make test && make test-e2e && (cd web/admin && npm run build)`

---

## Dependencies & Execution Order

- **Setup (T001)** → **Foundational (T002–T007)** block everything.
- **US1 (P1, T008–T015)** is the MVP and can ship once Foundational is done.
- **US2 (P2, T016–T020)** depends only on Foundational; the recommend/admin edits are independent
  of US1's catalog edits and may proceed in parallel with US1.
- **US3 (P3, T021–T026)** depends only on Foundational; independent of US1/US2.
- **Polish (T027–T031)** requires US1+US2+US3 complete (all readers migrated) before dropping
  legacy columns/fields.

```
T001 → T002..T007 → ┬─ US1 (T008..T015) ─┐
                    ├─ US2 (T016..T020) ─┼→ T027..T031 (Polish)
                    └─ US3 (T021..T026) ─┘
```

## Parallel Opportunities

- Within Foundational: T004 (types) and T002/T003 (migration SQL) can be drafted in parallel; T005/T006 follow T004.
- Across stories: once Foundational lands, US1, US2, US3 are independent and can be worked concurrently by different people.
- `[P]`-marked tasks touch distinct files with no ordering dependency within their phase (e.g. T010/T011, T013/T014, T022/T023/T024, T029/T030).

## Implementation Strategy

- **MVP = Foundational + US1**: delivers the headline win (one-decision catalog enable, no ordered
  precondition) while legacy fields still exist behind dual-write — shippable and reversible.
- **Increment 2 = US2**: removes the conflict surface and locks catalog authority.
- **Increment 3 = US3**: completes reader migration and proves behavior parity.
- **Finalize = Polish**: second migration drops the legacy columns; remove scaffolding. Run T027
  drop only after confirming no binary still reads the old columns (rolling-deploy safety, research R3).

## Test Summary

| Story | Verification |
|-------|--------------|
| US1 | nsconfig no-conflict; catalog ingest gate; embedder gate; renamed lister; web build |
| US2 | conflict tests deleted; object-BYOE 409 + subject-BYOE 204 under catalog |
| US3 | Phase 2 train/skip per value; dense-blend gate; backfill parity + CHECK rejection |
| Polish | full `make test`/`test-e2e`/lint, admin contract tests, codohuetypes golden untouched |
