# Implementation Plan: Backend Audit Remediation

**Branch**: `fix/redis-backend-audit-remediation` | **Date**: 2026-08-25 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/007-backend-audit-remediation/spec.md`

## Summary

Remediate every backend finding from the 2026-08-24 audit without changing
`web/admin`. The work ships in four gated releases: immediate dependency and
correctness fixes; Redis retention and reclaim fairness; durable namespace
generation and writer fencing; and migration-022 identity reconciliation under
maintenance mode. Public data-plane changes remain additive except where the
existing behavior reports false success or exposes protected operational data.

This revision aligns the design with Constitution 2.0.0 and resolves the prior
planning gaps: stream retention has an exact measurable post-pass bound;
authenticated detailed health uses `/healthz?details=true`; generation-aware
embed retention is introduced only with lifecycle generation support; ID-map
repair depends on compute through a composition-layer port; and legacy envelope
acceptance has an explicit operator-controlled closure step.

## Technical Context

**Language/Version**: Go 1.26.1 (`go.work` multi-module); SQL migrations; no new executable
**Primary Dependencies**: pgx v5/PostgreSQL 16, go-redis/Redis 7, Qdrant Go client/Qdrant 1.x, gRPC, Prometheus
**Storage**: PostgreSQL durable state and lifecycle ledger; Redis Streams/cache/trending; four Qdrant collections per namespace generation
**Testing**: `make lint`, `make build`, `make test`, `make test-race`, `make test-e2e`, `make compose-check`, `go mod verify`, pinned `govulncheck`, migration/restore rehearsals
**Target Platform**: Linux server and Docker Compose; approved `cmd/api`, `cmd/cron`, `cmd/admin`, and `cmd/embedder` binaries
**Project Type**: Multi-binary recommendation web service with Go client and Redis-stream SDK modules
**Performance Goals**: No new database round-trip on pure recommendation reads; lifecycle leases wrap only namespace-scoped writers/background mutations; retention runs at most once per minute; reclaim scans at most 10 pages per tick; cron completes within `CODOHUE_BATCH_INTERVAL_MINUTES`
**Constraints**: Preserve at-least-once delivery; never producer-trim unprocessed work; fail closed on lifecycle/config uncertainty; use rolling-compatible additive schema; return finite scores; do not change `web/admin`; add no binary
**Scale/Scope**: Root module plus `pkg/codohuetypes`, `sdk/go`, and `sdk/go/redistream`; approximately 12 backend packages, three migration pairs, existing data-plane contracts, and deployment/operations documentation

There are no unresolved `NEEDS CLARIFICATION` items.

## Constitution Check

*GATE: Passed before Phase 0 research; re-checked after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| **I. Code Quality** — domain structure, `docs.go`, import boundaries, English comments | ☑ | Lifecycle coordination stays in `internal/core/nslifecycle`; Redis retention stays in `internal/infra/redis`; repair-to-compute coordination is wired in `cmd/admin`, so no peer-domain or reverse core import is introduced. |
| **II. Testing Standards** — matching tests for every changed business-logic file | ☑ | Regenerated tasks must name direct tests for scoring, ID-map service/repository, Qdrant collection naming, Redis trending naming, lifecycle, retention, repair, and every changed service/repository/job/worker. |
| **III. API & Operations** — canonical paths, application authority, dedicated monitoring credential | ☑ | Data/admin paths are unchanged; `/metrics` and `/healthz?details=true` require the dedicated observability token; plain `/healthz` is sanitized. |
| **IV. Performance** — cache, batch budget, and cold-start rules preserved | ☑ | Config resolves before cache access, generation enters cache keys, exact retention is off the request path and guarded by a one-minute interval/kill switch, reclaim is page-bounded, and cold-start behavior is unchanged. |
| **Architecture Constraints** — exactly four binaries, table-backed IDs, migration pairs, full recompute | ☑ | Repair and legacy-gate commands are subcommands of `cmd/admin`; all schema changes use migrations 024-026; sparse identity repair uses full recompute. |

## Project Structure

### Documentation (this feature)

```text
specs/007-backend-audit-remediation/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── data-plane.md
│   ├── operations.md
│   └── stream-processing.md
└── tasks.md
```

### Source Code (repository root)

```text
cmd/
├── api/                     # lifecycle wiring, public/protected ops, global retention
├── cron/                    # configured-namespace cleanup
├── admin/                   # delete/reset, legacy gate, repair CLI and compute adapter
└── embedder/                # generation-aware work, reclaim, embed retention

internal/
├── core/
│   ├── idmap/               # mapping lease assertions and repair state machine
│   └── nslifecycle/         # ledger, leases, generation names, janitor
├── infra/
│   ├── metrics/             # bounded remediation metrics
│   ├── qdrant/              # repair inventory/snapshot/copy primitives
│   └── redis/               # consumer-progress retention
├── ingest/                  # guarded event/catalog workers and reclaim cursors
├── catalog/                 # atomic content+author and keyset reconciliation
├── embedder/                # full strategy identity and guarded mutations
├── compute/                 # configured namespaces and ownership cleanup
├── recommend/               # fail-closed config, finite scoring, honest delete
├── objects/                 # guarded object metadata
├── nsconfig/                # atomic base+catalog config and generation
└── admin/                   # lifecycle-aware delete/reset and re-embed orchestration

migrations/
├── 024_namespace_lifecycle.{up,down}.sql
├── 025_namespace_integrity_fks.{up,down}.sql
└── 026_id_mapping_repair.{up,down}.sql

pkg/codohuetypes/            # generation and reconciliation cursor fields
sdk/go/                      # stable errors, cursor, generation contracts
sdk/go/redistream/           # generation-stamped stream envelopes
e2e/                         # retention, lifecycle races, repair, observability
deploy/                      # staged rollout and coordinated restore runbooks
```

**Structure Decision**: Reuse the accepted four-binary domain layout. Cross-domain
coordination occurs only in binary composition roots. `internal/core/idmap`
defines a narrow sparse-rebuild port; `cmd/admin` adapts `internal/compute` to
that port so core never imports a domain and `compute -> idmap` cannot become a
cycle.

## Implementation Sequence

### Release 1 — Immediate security and correctness fixes

This release is migration-free and independently deployable.

1. Upgrade `pgx` to at least v5.9.2, `x/text` to at least v0.39.0, and gRPC to
   at least v1.82.1. Pin `make vuln` and CI scanning across every Go module.
2. Resolve namespace config before recommendation cache access. Missing config
   returns stable 404; unavailable storage returns stable 503. No degraded
   default response is served or cached.
3. Reject missing namespaces before event, catalog, BYOE, and object mutations,
   including calls authenticated by the global admin authority.
4. Apply the five-minute future-skew policy to every `object_created_at` and
   centralize finite freshness scoring clamped to `[0,1]`.
5. Make object deletion attempt sparse, dense, and metadata cleanup and return
   joined retryable errors for every non-NotFound failure.
6. Compare `(strategy_id,strategy_version)` throughout re-embed selection,
   progress, and completion. Run compensating `only_state=all` re-embeds where
   a strategy ID may have changed at the same version.
7. Enumerate configured namespaces in cron. Empty keep sets clear sparse state;
   dense cleanup follows the item2vec/SVD/catalog/BYOE ownership matrix.
8. Commit namespace+catalog config in one validated transaction and commit
   catalog content+author attribution in one transaction.
9. Add the opaque `(updated_at,id)` catalog cursor while retaining legacy
   offset for one compatibility window. The migration-025 index is a later
   performance optimization, not a functional Release-1 dependency.
10. Sanitize plain `/healthz`. Register `/metrics` only when
    `CODOHUE_OBSERVABILITY_TOKEN` exists. Add `/healthz?details=true` on the
    existing listener; missing/invalid observability credentials return 401,
    while a valid credential receives component diagnostics.

**Release gate**: Standard checks pass; vulnerability scan has zero reachable
findings; fault injection proves zero cache/Qdrant/DB mutation after config
failure; every accepted score is finite; public health contains no raw error.

### Release 2 — Redis retention and recovery fairness

1. Remove every producer-side `XADD MAXLEN`, including embed-stream caps.
2. Once per minute, inspect every group. The safe frontier is the oldest pending
   ID when the PEL is non-empty, otherwise the group's last-delivered ID. Trim
   only IDs strictly below the minimum frontier using exact `XTRIM MINID`.
3. A successful pass leaves zero entries below the computed safe frontier. This
   is the documented completed-history bound; every retained entry is protected
   by at least one group frontier. Any inspection/trim failure means no claimed
   progress and raises an alert.
4. Protect unexpected groups in the frontier calculation. Backfill groups from
   `0` require retention to remain disabled until catch-up.
5. Carry every `XAUTOCLAIM` cursor across empty pages and ticks. Scan at most 10
   pages per tick; reset only at terminal `0-0`; retain a separate embed cursor.
6. Add stream length, pending, undelivered, trimmed, reclaim, error, and
   unexpected-group metrics with bounded labels.
7. Release 2 discovers and retains only existing generation-1 embed streams.
   Generation-qualified embed stream discovery is added in Release 3 after the
   lifecycle name resolver exists.

**Rollout gate**: Deploy with retention disabled, verify frontiers, then enable
generation-1 embed, catalog, and event streams in order. Ten retention windows
must leave zero entries below the safe frontier after each successful pass and
recover 100% of unfinished work. The kill switch disables trimming.

### Release 3 — Namespace lifecycle fencing

1. Apply migration 024 for durable namespace tombstones, monotonic generation,
   persistent reset state, and generation on active config. Only grandfathered
   generation 1 may accept missing envelope generations.
2. Apply migration 025 after orphan audit/cleanup. Add namespace FKs as
   `NOT VALID`, validate them, and add the catalog keyset index.
3. Acquire leases in fixed order: global shared then namespace shared for normal
   mutation; global shared then namespace exclusive for delete/recreate; global
   exclusive for reset. Compute acquires its existing lock last.
4. Re-read active lifecycle/generation after lease acquisition. DB-only writes
   use transaction locks; Redis/Qdrant writers hold session leases until the
   external mutation finishes. Read paths never create mappings.
5. Stamp event, catalog, and embed envelopes with `namespace_generation`.
   Stale/deleted work is permanently ACKed with a metric; lifecycle storage
   errors remain pending.
6. Centralize generation-aware cache, trending, embed-stream, and collection
   names. Generation 1 retains legacy names; generation 2+ uses scoped names.
   Extend Release-2 retention discovery to those scoped embed streams here.
7. Implement durable delete/reset state machines. A failure remains `deleting`
   or `resetting`, records the error, blocks work/recreation, and resumes
   idempotently. Recreation increments generation only from `deleted`.
8. Add `cmd/admin lifecycle disable-legacy-envelopes --all`. It acquires the
   global exclusive lease, verifies producer-adoption evidence, atomically sets
   `legacy_messages_allowed=false` for every lifecycle, and is idempotent.
9. Only after the legacy gate closes may the janitor reclaim deleted-generation
   Redis and Qdrant artifacts.

**Release gate**: 100 concurrent delete/reset/recreate trials cover HTTP, both
global streams, embedder, cron, BYOE, metadata, and old envelopes. No stale
generation writes remain and the legacy gate cannot be reopened implicitly.

### Release 4 — Migration-022 identity reconciliation

1. Apply migration 026 for composite numeric-ID uniqueness, durable repair
   run/item manifests, and a rollback preflight that aborts before constraint
   mutation when global string-ID duplicates exist.
2. Add `cmd/admin idmap-repair audit|apply|verify|resume`. Audit PostgreSQL and
   all four Qdrant collection kinds by namespace, entity type, payload string
   ID, vector hash, and numeric ID. Missing/ambiguous evidence is quarantined.
3. Audit/preflight completes for the full immutable manifest before any repair
   mutation. Any unresolved item stops apply and produces an actionable report.
4. Before apply, acquire the global exclusive lifecycle lease and record a
   coordinated PostgreSQL backup plus every affected Qdrant snapshot.
5. Preserve correct mappings; otherwise choose one fresh target ID, copy dense
   vector/payload exactly, verify hashes, then remove the old point. Failures
   halt and persist resumable item state.
6. `internal/core/idmap` invokes sparse rebuild through a narrow port. The
   adapter in `cmd/admin` calls `internal/compute`; no core/domain reverse import
   is allowed. Sparse collections are rebuilt after subject mappings settle.
7. Verify every manifest tuple, count/hash, payload ID, sparse rebuild, and old
   point absence before releasing maintenance mode.

**Rollback gate**: Migration 022 is forward-only after composite duplicates
exist. Normal recovery is resume. A true rollback restores the recorded
PostgreSQL backup and every Qdrant snapshot from the same checkpoint while the
global lease remains held.

## Cross-Release Verification Matrix

| Area | Unit/package | Integration/E2E | Operational gate |
|------|--------------|-----------------|------------------|
| Dependencies | Module build/tests | pgx/Qdrant/gRPC E2E | `govulncheck` zero reachable |
| Streams | Exact frontier and reclaim cursor | Redis 7 multi-group/outage/load | zero post-pass entries below frontier |
| Lifecycle | Lease/state/name/legacy-gate tests | 100 delete-reset-recreate races | no stale-generation writes |
| Compute/re-embed | Ownership matrix; full strategy tuple | expired-event and replacement fixtures | accurate cleanup/progress |
| Recommendation | config faults, finite scoring, delete errors | 404/503/204/5xx contracts | no false cache/success |
| Config/catalog | transaction rollback | concurrent dimension/author writes | zero partial state |
| ID mapping | manifest/hash/collision/resume | PostgreSQL+Qdrant rehearsal | coordinated snapshots verified |
| Observability | auth and health representation | public/protected scrape checks | zero public disclosure |

## Complexity Tracking

| Design complexity | Why needed | Simpler alternative rejected because |
|-------------------|------------|--------------------------------------|
| Persistent lifecycle ledger plus leases and generations | Cross-store deletion must fence in-flight writers and old queued work independently | FKs cannot fence Redis/Qdrant; locks cannot identify old queue entries; generation alone cannot drain in-flight writes |
| Resumable repair manifest and coordinated snapshots | Migration 022 affects unrecomputable BYOE vectors and sparse coordinate identities across two stores | Full recompute cannot recover client-provided vectors or prove cross-store rollback |

No constitution violation remains after Phase 1 design.
