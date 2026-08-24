# Implementation Plan: Backend Audit Remediation

**Branch**: `fix/redis-backend-audit-remediation` | **Date**: 2026-08-24 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/007-backend-audit-remediation/spec.md`

## Summary

Remediate every backend finding from the 2026-08-24 audit without changing `web/admin`.
The work ships in four gated releases: (1) remove reachable dependency vulnerabilities and
fix isolated correctness/fail-open defects; (2) make Redis retention consumer-progress-aware;
(3) add a durable namespace generation ledger and shared-writer/exclusive-delete fencing;
and (4) reconcile the migration-022 PostgreSQL/Qdrant identity split under a maintenance
freeze. Public API changes remain additive except that invalid namespaces, unavailable
configuration, unsafe timestamps, and incomplete deletes now fail instead of returning a
false success. Metrics require a dedicated observability credential and health output is
sanitized.

## Technical Context

**Language/Version**: Go 1.26.1 (`go.work` multi-module); SQL migrations; Redis Lua only where an atomic primitive is required
**Primary Dependencies**: pgx v5/PostgreSQL 16, go-redis/Redis 7, Qdrant Go client/Qdrant 1.x, gRPC, Prometheus
**Storage**: PostgreSQL durable state and lifecycle ledger; Redis Streams/cache/trending; four Qdrant collections per namespace generation
**Testing**: `make lint`, `make build`, `make test`, `make test-race`, `make test-e2e`, `make compose-check`, `go mod verify`, pinned `govulncheck`, migration/restore rehearsals
**Target Platform**: Linux server and Docker Compose; existing `cmd/api`, `cmd/cron`, `cmd/admin`, and `cmd/embedder` binaries
**Project Type**: Multi-binary recommendation web service with Go client and Redis-stream SDK modules
**Performance Goals**: No new database round-trip on pure recommendation reads; lifecycle leases only wrap namespace-scoped writers/background mutations; retention runs at most once per minute and reapers scan at most 10 pages per tick; cron remains within `CODOHUE_BATCH_INTERVAL_MINUTES`
**Constraints**: Preserve at-least-once delivery; never producer-trim unprocessed work; fail closed on lifecycle/config uncertainty; rolling-compatible additive schema; finite response scores; no `web/admin` changes; no new binary
**Scale/Scope**: Root module plus `pkg/codohuetypes`, `sdk/go`, and `sdk/go/redistream`; approximately 12 backend packages, three new migration pairs, existing data-plane contracts, deployment/operations documentation

## Constitution Check

*GATE: Passed before Phase 0 research; re-checked after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| **I. Code Quality** — domain in `internal/<domain>/`, `docs.go` present, import boundaries respected, English-only comments | ☑ | Lifecycle coordination lives under `internal/core/nslifecycle`; Redis retention stays under `internal/infra/redis`. Catalog may update the `objects` table in one repository transaction but does not import the objects domain. No new domain is introduced. |
| **II. Testing Standards** — `_test.go` planned for every `service.go`, `repository.go`, `job.go`, `worker.go` | ☑ | Every touched business-logic file gets a matching regression test. New lifecycle, retention, repair, concurrency, migration, and restore paths receive unit plus integration/E2E coverage. |
| **III. API Consistency** — endpoints follow `/v1/<resource>`, two-tier auth, REST API table in CLAUDE.md updated | ☑ | No new public resource path. Existing catalog reconciliation gains an additive cursor; error codes are normalized; metrics authentication and sanitized health are documented in `CLAUDE.md`. |
| **IV. Performance** — Redis cache plan in place, batch phases non-blocking, cold-start fallback accounted for | ☑ | Config is resolved before cache access, degraded results are never cached, generation enters cache keys, retention/reclaim loops have page/time budgets, and empty-event cleanup remains bounded. Cold-start behavior is unchanged. |

The constitution still says the system has exactly two binaries, while the accepted repository
architecture has four. This is a pre-existing governance conflict; this remediation adds no
binary and uses a subcommand of `cmd/admin` for repair.

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
└── tasks.md                 # generated later by /speckit.tasks
```

### Source Code (repository root)

```text
cmd/
├── api/                     # lifecycle wiring, health/metrics policy, global retention
├── cron/                    # all-namespace cleanup and repair integration
├── admin/                   # resumable idmap-repair subcommand; delete/reset state machine
└── embedder/                # generation-aware work, reclaim cursor, embed retention

internal/
├── core/
│   ├── idmap/               # mapping reconciliation and lease assertions
│   └── nslifecycle/         # ledger, leases, generation-aware physical naming
├── infra/
│   ├── metrics/             # remediation/retention/lifecycle metrics
│   └── redis/               # consumer-progress retention
├── ingest/                  # guarded event writes and paged reclaim
├── catalog/                 # guarded atomic content+author writes, keyset reconciliation
├── embedder/                # full strategy identity, guarded Qdrant mutations
├── compute/                 # configured-namespace enumeration and empty keep-set cleanup
├── recommend/               # fail-closed config, finite scoring, honest delete
├── objects/                 # guarded metadata mutation
├── nsconfig/                # atomic base+catalog upsert
└── admin/                   # lifecycle-aware delete/reset and re-embed orchestration

migrations/
├── 024_namespace_lifecycle.{up,down}.sql
├── 025_namespace_integrity_fks.{up,down}.sql
└── 026_id_mapping_repair.{up,down}.sql

pkg/codohuetypes/            # additive generation and reconciliation cursor fields
sdk/go/                      # error/cursor/generation contract updates
sdk/go/redistream/           # generation-stamped stream envelopes
e2e/                         # lifecycle races, retention, migration repair, observability
deploy/                      # staged rollout and coordinated snapshot/restore runbook
```

**Structure Decision**: Reuse the existing domain layout and inject the cross-cutting
lifecycle coordinator from each binary. Shared lifecycle types and lock order belong in
`internal/core/nslifecycle`; storage-specific stream retention remains infrastructure.
No peer-domain import or new executable is required.

## Implementation Sequence

### Release 1 — Immediate security and correctness fixes

This release is migration-free and independently deployable.

1. Upgrade `pgx` to at least v5.9.2, `x/text` to at least v0.39.0, and gRPC to at
   least v1.82.1. Add a pinned `make vuln`/CI check across every Go module.
2. Resolve namespace config before recommendation cache access. Return stable 404 for a
   missing namespace and 503 for unavailable config; never cache a default-based result.
3. Reject missing namespaces before BYOE/object mutations even for the global admin key.
4. Apply the existing five-minute future-skew policy to `object_created_at`; centralize a
   freshness multiplier clamped to `[0,1]`; exclude/log/metric any non-finite candidate.
5. Make object deletion attempt sparse, dense, and metadata cleanup and return joined,
   retryable errors for every non-NotFound failure.
6. Compare `(strategy_id,strategy_version)` throughout re-embed reset, progress, and
   completion. Run a one-time `only_state=all` re-embed for namespaces that may have
   changed strategy ID at the same version.
7. Enumerate configured namespaces in cron. For empty event sets, delete sparse state;
   delete item and subject dense state only when cron owns it, preserve catalog object
   vectors and BYOE vectors, and clear catalog-derived subject vectors.
8. Make namespace+catalog upsert one validated transaction and persist catalog content
   plus requested author attribution atomically.
9. Replace timestamp-only catalog pagination with an additive opaque keyset cursor based
   on `(updated_at,id)` while retaining legacy query parameters for one compatibility
   window.
10. Sanitize public health output. Register `/metrics` only when a dedicated
    `CODOHUE_OBSERVABILITY_TOKEN` is configured and require constant-time bearer auth in
    both API and embedder processes.

**Release gate**: all standard checks pass; vulnerability scan has zero reachable
findings; fault injection proves no cache/Qdrant/DB mutation occurs after config failure;
all accepted scores are finite.

### Release 2 — Redis retention and recovery fairness

1. Remove every `XADD MAXLEN`, including the three 100,000-entry caps on embed streams.
2. Add a one-minute consumer-progress retention loop. For every group, the safe frontier
   is its oldest pending ID or, with an empty PEL, its last-delivered ID. Trim only below
   the minimum frontier across all groups with `XTRIM MINID ~`.
3. Fail closed when stream/group/PEL inspection is unavailable. Detect unexpected groups,
   protect their progress, and alert rather than delete their unread work.
4. Carry each `XAUTOCLAIM` cursor across empty pages and ticks. Limit a tick to 10 pages;
   reset only at terminal `0-0`; maintain a separate cursor per embed namespace.
5. Add stream length, pending, undelivered, trimmed, reclaim, error, and unexpected-group
   metrics with bounded label cardinality.

**Rollout gate**: deploy with retention disabled, verify computed frontiers, then enable
embed streams, catalog input, and event input in that order. Ten retention windows of
load/outage testing must preserve all unprocessed entries while completed history stops
growing linearly. The kill switch disables trimming without rolling back binaries.

### Release 3 — Namespace lifecycle fencing

1. Apply migration 024: persistent namespace lifecycle tombstones, monotonically
   increasing generation, durable global reset state, and a generation on active configs.
   Backfilled generation 1 alone may accept generation-less legacy stream envelopes.
2. Apply migration 025 after orphan audit/cleanup: add `ON DELETE CASCADE` namespace FKs
   using `NOT VALID` then validate, plus the catalog keyset index.
3. Introduce fixed-order leases: global shared then namespace shared for normal mutation;
   global shared then namespace exclusive for delete/recreate; global exclusive for reset.
   Compute retains its own lock after acquiring a lifecycle lease.
4. Require active state/generation after lease acquisition. DB-only writers use a
   transaction-scoped shared lease; Qdrant/Redis multi-store writers hold a session lease
   until external mutation finishes. Read paths stop creating ID mappings.
5. Add `namespace_generation` to event, catalog, and embed envelopes. A mismatched/deleted
   generation is a permanent ACK/drop; lifecycle storage errors are transient and remain
   pending.
6. Centralize physical names. Generation 1 resolves to legacy Redis/Qdrant names; later
   generations use generation-scoped cache, trending, embed-stream, and collection names.
7. Implement durable delete/reset state machines. Failure remains `deleting` or
   `resetting`, blocks new work, records the error, and resumes idempotently. Recreate is
   allowed only from `deleted` and increments generation.
8. After the SDK adoption window, disable generation-less envelopes and janitor deleted
   generation artifacts.

**Release gate**: 100 concurrent delete/reset trials across HTTP, both global streams,
embedder, cron, BYOE, and object metadata leave no post-success state. Recreating the same
name must reject every old envelope and expose only the new generation.

### Release 4 — Migration-022 identity reconciliation

1. Apply migration 026: replace global numeric-ID uniqueness with uniqueness within
   `(namespace,entity_type)`, add resumable repair run/item manifests, and add a rollback
   preflight that aborts before modifying constraints when global `string_id` duplicates
   exist.
2. Add `cmd/admin idmap-repair audit|apply|verify|resume`. Audit PostgreSQL and all four
   Qdrant collections, grouping points by namespace, entity type, string payload ID, vector
   hash, and numeric point ID. Missing/ambiguous payloads are quarantined, never guessed.
3. Before apply, acquire the global exclusive lifecycle lease, record a coordinated
   PostgreSQL backup and Qdrant snapshot set, and persist their references plus manifest
   hash.
4. Preserve an already-correct mapping; otherwise select one globally fresh target ID.
   Copy dense vector and payload exactly to the target, verify hashes, then remove the old
   point. Conflicts halt and remain resumable.
5. Rebuild sparse collections for every configured namespace because sparse object
   coordinates contain subject numeric IDs. Recompute item2vec/SVD; preserve copied
   catalog/BYOE vectors, with optional catalog re-embed as validation rather than recovery.
6. Verify every manifest tuple, vector count/hash, payload ID, sparse rebuild, and absence
   of mismatched old points before releasing maintenance mode.

**Rollback gate**: migration 022 is forward-only after composite duplicates exist. Normal
failure recovery is roll-forward/resume. A true rollback requires restoring the recorded
PostgreSQL backup and all Qdrant snapshots from the same checkpoint; restoring one store
alone is forbidden.

## Cross-Release Verification Matrix

| Area | Unit/package | Integration/E2E | Operational gate |
|------|--------------|-----------------|------------------|
| Dependencies | module build/tests | pgx/Qdrant/gRPC E2E | `govulncheck` zero reachable |
| Streams | retention frontier and cursor tests | Redis 7 multi-group/outage/load | memory, XLEN, PEL alerts healthy |
| Lifecycle | lease ordering/state transitions/naming | 100 delete-reset races | no stale-generation writes |
| Compute/re-embed | empty ownership matrix; full strategy tuple | expired-event and same-version strategy fixtures | accurate cleanup/progress |
| Recommendation | config faults, finite scoring, joined delete errors | API 404/503/204/500 contracts | no false cache/success |
| Config/catalog | transaction rollback fixtures | concurrent dimension/author writes | zero partial state |
| ID mapping | manifest/hash/collision/resume tests | PostgreSQL+Qdrant migration rehearsal | coordinated snapshots verified |
| Observability | auth and sanitization handlers | compose scrape/health checks | public disclosure test passes |

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Repository currently operates four binaries although constitution says exactly two | This is accepted live architecture and the remediation must cover `cmd/admin` and `cmd/embedder` | Adding a fifth repair binary would worsen the violation; the repair workflow is therefore a `cmd/admin` subcommand |
| Persistent lifecycle ledger plus leases and generations | Cross-store deletion must fence in-flight HTTP/background writers and old queued work independently | FKs cannot fence Redis/Qdrant; locks cannot identify old queue entries; generation alone cannot drain in-flight writes |
| Resumable repair manifest and coordinated snapshots | Migration 022 may affect unrecomputable BYOE vectors and sparse coordinate identities across two stores | A full cron recompute cannot recover client-provided vectors or safely prove rollback |
