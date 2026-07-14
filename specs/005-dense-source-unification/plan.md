# Implementation Plan: Dense Source Unification

**Branch**: `feat/compute-dense-source-unification` | **Date**: 2026-06-19 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/005-dense-source-unification/spec.md`

## Summary

Collapse the two coupled namespace fields `dense_strategy` (disabled|item2vec|svd|byoe) and
`catalog_enabled` (bool) into one mutually-exclusive enum `dense_source`
(disabled|item2vec|svd|byoe|catalog). Catalog auto-embedding becomes the fifth value rather
than a separate boolean, so the contradictory "two producers writing `{ns}_objects_dense`"
state is unrepresentable and the `conflictsWithCatalog` / `DenseStrategyConflictError`
validation is deleted. Technical approach: two migrations — 016 adds `dense_source` (backfill
+ CHECK, legacy columns kept for the dual-write window) and 017 drops the legacy columns after
readers cut over — plus a phased reader cutover (add+dual-write → migrate readers → drop), and
matching updates to the admin wire types
and the `web/admin` namespace config form. The client wire contract in `pkg/codohuetypes` is
**not** affected (these fields are admin-internal). Source material: [design.md](design.md).

## Technical Context

**Language/Version**: Go 1.x (go.work multi-module); TypeScript/React 19 for `web/admin`  
**Primary Dependencies**: pgx (PostgreSQL), Qdrant gRPC, go-redis, Prometheus; Vite + Tailwind v4 (admin UI)  
**Storage**: PostgreSQL `namespace_configs` table (column change via numbered migration); Qdrant collections unchanged  
**Testing**: `go test` across modules (`make test`, `make test-pkg`), `make test-e2e`; admin contract/golden tests under `internal/admin`  
**Target Platform**: Linux server (four binaries: api, cron, admin, embedder)  
**Project Type**: Web service + embedded admin SPA (backend Go domains + `web/admin` frontend)  
**Performance Goals**: No change to recommend latency or batch budget; this is a config-shape refactor on a cold path (namespace config read once per recompute / per request via existing cache)  
**Constraints**: Phased migration must not break readers mid-deploy; behavior must be identical pre/post migration for all five prior configurations  
**Scale/Scope**: One DB column reshaped; ~6 Go domains + 4 web/admin files touched; no new endpoints, no new binary

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| **I. Code Quality** — domain in `internal/<domain>/`, `docs.go` present, import boundaries respected, English-only comments | ☑ | No new domains; edits stay within existing `nsconfig`/`compute`/`recommend`/`catalog`/`embedder`/`admin`/`core/namespace`. No new cross-domain imports — the change removes coupling, not adds it. |
| **II. Testing Standards** — `_test.go` for every `service.go`/`repository.go`/`job.go`/`worker.go` touched | ☑ | Update existing tests in nsconfig, compute, recommend, embedder; the conflict-validation tests are deleted with the code. Add migration backfill + CHECK-rejection coverage. |
| **III. API Consistency** — `/v1/<resource>`, two-tier auth, REST API table in CLAUDE.md updated | ☑ (mostly N/A) | No new/removed endpoints. The PUT catalog and namespace config endpoints keep their paths; only the request/response field shape changes. CLAUDE.md "Key Design Decisions" + nsconfig/admin notes need a wording update, not a table row. |
| **IV. Performance** — Redis cache plan, batch phases non-blocking, cold-start fallback | ☑ N/A | Cold path; no change to caching, batch budget, or cold-start. Phase 2 gate condition is reworded but functionally identical (item2vec/svd train, others skip). |

> No ☒ violations. Note: the constitution's "exactly two binaries" clause is already superseded
> by the live four-binary architecture documented in CLAUDE.md; this feature adds no binary.

## Project Structure

### Documentation (this feature)

```text
specs/005-dense-source-unification/
├── design.md            # Pre-existing hand-written sketch (source material)
├── spec.md              # Feature spec (/speckit.specify)
├── plan.md              # This file (/speckit.plan)
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (admin config request/response shape)
│   └── namespace-config.md
└── checklists/
    └── requirements.md  # Spec quality checklist (/speckit.specify)
```

### Source Code (repository root)

```text
migrations/
├── 016_dense_source.up.sql               # ADD dense_source, backfill, CHECK (legacy columns kept for dual-write)
├── 016_dense_source.down.sql             # drop dense_source
├── 017_drop_legacy_dense_fields.up.sql   # drop dense_strategy + catalog_enabled (after readers cut over)
└── 017_drop_legacy_dense_fields.down.sql # recreate both + backfill from dense_source

internal/
├── core/namespace/types.go      # Config.DenseStrategy + .CatalogEnabled → .DenseSource
├── nsconfig/
│   ├── types.go                 # drop DenseStrategyConflictError; UpsertRequest field rename
│   ├── service.go               # delete conflictsWithCatalog + both validation branches
│   └── repository.go            # SQL column rename; ListCatalogEnabled→ListCatalogNamespaces
├── compute/job.go               # Phase 2 gate + switch on DenseSource
├── recommend/service.go         # dense-blend gate + BYOE-object-block on DenseSource=="catalog"
├── catalog/service.go           # ingest gate on DenseSource=="catalog"
├── embedder/
│   ├── service.go               # CatalogEnabled check → DenseSource=="catalog"
│   ├── worker.go                # ListCatalogEnabled rename
│   └── backlog_sampler.go       # ListCatalogEnabled rename
├── admin/
│   ├── types.go                 # drop conflict error type; config DTO field shape
│   ├── handler.go               # drop conflict error-mapping blocks
│   ├── repository.go            # SQL column rename
│   └── demo.go                  # DenseStrategy:"disabled" → DenseSource:"disabled"
└── ...

cmd/admin/
├── nsconfig_adapter.go          # drop conflict plumbing
└── catalog_adapter.go           # drop conflict plumbing; enable = set DenseSource="catalog"

web/admin/src/
├── services/namespaces.ts       # types: dense_strategy + catalog_enabled → dense_source
├── pages/ns/config/NamespaceConfigPage.tsx    # single dense_source dropdown
├── pages/namespaces/CreateNamespaceDialog.tsx # dense_source field
├── pages/namespaces/NamespacesListPage.tsx    # display dense_source
└── pages/ns/NamespaceOverviewPage.tsx         # display dense_source
```

**Structure Decision**: Existing four-binary Go service + embedded `web/admin` SPA. The change
is confined to the namespace-config read/write surface and the consumers that branch on it; no
new directories, domains, or binaries. Migration is a single numbered pair under `migrations/`.

## Complexity Tracking

> No constitution violations — section intentionally empty.
