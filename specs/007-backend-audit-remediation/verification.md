# Verification — backend audit remediation

Recorded on branch `fix/redis-backend-audit-remediation`.

## Gate results

| Command | Result | Notes |
|---------|--------|-------|
| `make lint` | **pass** | 0 issues across all four `go.work` modules. Started at 55 findings, all cleared. |
| `make build` | **pass** | All four binaries build (`./tmp/{api,cron,admin,embedder}`). |
| `make test` | **pass** | Full unit suite green across every module. |
| `make test-race` | **pass** | `-race` clean across every module. |
| `go mod verify` | **pass** | `all modules verified`. |
| `make vuln` | **pass** | 0 vulnerabilities the code calls. 4 in imported packages and 21 in required modules remain unreachable — see below. |
| `make compose-check` | **pass** | `docker-compose.yml`, `.app.yml` and `.prod.yml` all parse. |
| `git diff --check` | **pass** | No whitespace errors. |
| `make test-e2e` | **NOT RUN** | Requires a live postgres + redis + qdrant stack; none was available in this environment. The suite compiles (`go vet -tags=e2e ./e2e/`) but has not been executed. |

### govulncheck detail

`Your code is affected by 0 vulnerabilities.` The 4 import-level and 21
module-level findings are in code paths this repository does not call;
`govulncheck` reports them for awareness. Re-check with `-show verbose` if a
dependency's call graph changes.

## Scope check (T110)

- `git diff --check`: clean.
- Files changed under `web/admin/` in the remediation diff: **0**, both
  committed (`git diff --name-only main...HEAD`) and uncommitted
  (`git status --short`). The no-frontend constraint holds.

## Phase 10 (convergence) gates

Re-run after the convergence tasks T111-T116:

| Command | Result |
|---------|--------|
| `make lint` | **pass** — 0 issues |
| `go build ./...` | **pass** |
| `go test ./...` | **pass** |
| `go test -race` (idmap, cmd/admin, cmd/api) | **pass** |
| `go vet -tags=e2e ./e2e/` | **pass** — suites still compile |

## Phase 11 (convergence) gates

| Command | Result |
|---------|--------|
| `make lint` | **pass** — 0 issues |
| `go build ./...` | **pass** |
| `go test ./...` | **pass** |
| `go test -race` (idmap, cmd/admin, infra/qdrant) | **pass** |
| `go vet -tags=e2e ./e2e/` | **pass** |

Dead-code recheck after wiring: `NextNumericID`, `RetargetMapping`,
`InspectPoint`, `copyableCollections`, `missingSnapshotCollections` and
`verifyItem` all have live call sites. `ValidateSnapshotRefs` and
`affectedCollections` were retired — the manifest-aware check replaced them.

## Phase 12 (convergence) gates

| Command | Result |
|---------|--------|
| `make lint` | **pass** — 0 issues |
| `go build ./...` | **pass** |
| `go test ./...` | **pass** |
| `go test -race` (idmap, cmd/admin, infra/qdrant) | **pass** |
| `go vet -tags=e2e ./e2e/` | **pass** |

Migration 027 adds `id_mapping_repair_runs.rebuilt_namespaces`; include it in
the T109 rehearsal alongside 024-026.

All six `RepairItemState` values are now assigned by the code, so the state
machine matches the data model.

## Phase 13 (convergence) gates

| Command | Result |
|---------|--------|
| `make lint` | **pass** — 0 issues |
| `go build ./...` | **pass** |
| `go test ./...` | **pass** |
| `go test -race` (idmap, cmd/admin) | **pass** |
| `go vet -tags=e2e ./e2e/` | **pass** |

T124 added nine PostgreSQL-backed tests for the repair repository in
`internal/core/idmap/repair_repository_test.go` (not `e2e/`, see below). They
**skip** without `DATABASE_URL` — confirmed skipping rather than passing
vacuously — so they run under `make test` wherever a database is configured.
Until then the repair repository's SQL still has no execution anywhere.

## Phase 14 (convergence) gates

| Command | Result |
|---------|--------|
| `make lint` | **pass** — 0 issues |
| `go build ./...` | **pass** |
| `go test ./...` | **pass** |
| `go test -race` (idmap) | **pass** |
| `go vet -tags=e2e ./e2e/` | **pass** |

T126 fixed a defect in the T124 tests that no local run could surface: the
mapping insert violated `id_mappings_namespace_fk` (migration 025), and
satisfying it also requires a `namespace_lifecycles` row because
`namespace_configs (namespace, generation)` references it (migration 024). A
`seedNamespace` helper now makes that chain explicit. **This was found by
reading the schema, not by running anything** — the first real run of these
tests is still their first real validation.

## Outstanding — requires a live environment

These cannot be completed from a workstation without the stack running. Each
needs an operator with infra access.

### T108 — end-to-end suite

```bash
make up-infra && make migrate-up && make test-e2e
```

New suites added by this work, none yet executed:

| Suite | Proves |
|-------|--------|
| `e2e/namespace_lifecycle_test.go` | 100 delete/writer races leave no orphan rows; recreate mints a new generation with separately named artifacts; stale envelopes are acked and dropped; delete removes generation-scoped Redis keys |
| `e2e/recommendation_state_test.go` | Expired events clear owned vectors; a same-version foreign strategy still counts as stale; the five-minute skew boundary; every served score is finite |
| `e2e/idmap_repair_test.go` | Ambiguous evidence mutates nothing; a BYOE vector copies byte-identical; manifest drift is detected; snapshots are required; the rollback preflight blocks on duplicates |
| `e2e/honest_failures_test.go` | Stable 404 across every write path with zero mutation and zero cache; catalog attribution is atomic with content; delete is idempotent |
| `e2e/recovery_reconciliation_test.go` | Reclaim reaches every page past a failing head; retention never trims pending work; keyset paging over equal timestamps visits each row exactly once |
| `e2e/observability_security_test.go` | Public health discloses nothing; details and metrics need the observability token and reject the admin key |

`observability_security_test.go` skips unless `CODOHUE_OBSERVABILITY_TOKEN` is
set for the run — set it before treating a pass as meaningful.

### T124 — repair repository SQL

```bash
DATABASE_URL=postgres://... make test-pkg PKG=./internal/core/idmap/...
```

Covers the two-table transactional insert, the partial unique index on
in-flight runs, the jsonb append-and-dedup behind `RecordRebuiltNamespace`,
item/run state transitions, and mapping retarget. Requires migrations 024-027.

### T109 — migration and operational rehearsal

Against a disposable copy of production-shaped data:

1. **Migrations 024–027 up.** Confirm every namespace backfills to generation 1
   and keeps its existing physical names (nothing should move).
2. **Legacy-gate closure.** Run
   `./tmp/admin lifecycle disable-legacy-envelopes --all --adoption-evidence <ref>`;
   confirm it is idempotent (second run reports `changed=false`) and that a
   generation-less envelope is dropped afterwards.
3. **Ambiguous repair refusal.** Seed two points in one collection claiming the
   same object id, run `./tmp/admin idmap-repair audit`, and confirm the item is
   quarantined and `apply` refuses with zero points and zero mappings mutated.
4. **Repair resume.** Interrupt an apply mid-manifest (kill the process), then
   `./tmp/admin idmap-repair resume --run <id>` and confirm it continues from
   durable item state rather than restarting.
5. **Coordinated snapshot restore.** Restore both the PostgreSQL dump and every
   Qdrant snapshot from the same checkpoint; confirm the fleet returns to the
   pre-apply state.
6. **Migration-down safety.** With cross-namespace duplicate string ids
   present, confirm `022_id_mappings_composite.down.sql` and
   `026_id_mapping_repair.down.sql` both raise *before* dropping a constraint,
   leaving the schema unchanged.

Record each result here with the date, the operator, and the run/snapshot
references. The per-namespace evidence template is in
[deploy/backend-audit-remediation.md](../../deploy/backend-audit-remediation.md).
