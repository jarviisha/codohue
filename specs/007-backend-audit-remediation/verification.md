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

## Phase 15 (convergence) gates

| Command | Result |
|---------|--------|
| `make lint` | **pass** — 0 issues |
| `go build ./...` | **pass** |
| `go test ./...` | **pass** |
| `go vet -tags=e2e ./e2e/` | **pass** |

T128 fixed two defects in one e2e test, neither of which compilation or
`go vet` can catch: the item was built without a `state`, which the migration
026 CHECK rejects, and the service was constructed with a nil fence, so `Apply`
returned "requires the global lifecycle fence" long before the snapshot check
the test asserts on. T129 stopped the race test calling `t.Fatalf` from writer
goroutines, where `FailNow` exits the worker without failing the test.

A follow-up sweep for the same two shapes across the other e2e files found
nothing further — but that sweep is reasoning, not execution. **Three
convergence rounds in a row have found defects in tests written the round
before, all of them invisible to compilation.** Running the suites once is
worth more than another reasoning pass.

## Phase 19 — the lifecycle lock pair deadlocked a small pool

CI e2e stayed red after Phase 18. The goroutine dump showed all four writers in
`TestNamespaceLifecycle_ConcurrentWritersNeverOutliveADelete` blocked inside
`http.Client.Do` for 14 minutes, so `writers.Wait()` never returned and the
suite died on its own timeout.

### Root cause — a production deadlock, not a test defect

`WithWriter` fences every data-plane write with `acquirePair`: global-shared,
then namespace-shared. Each was a **separate** `pg_advisory_lock` on a
**separate pooled connection**, because a session advisory lock lives on its
session. So one in-flight write pinned two pool connections and needed a third
to do its actual work.

`NewPool` never set `MaxConns`, so pgxpool's default applied:
`max(4, runtime.NumCPU())`. A 16-core dev box got 16 and never saw it; a 4-vCPU
CI runner got 4 and hit it every run. With four concurrent writers on a pool of
four, all four took the global lock, exhausted the pool, and then waited forever
for a connection to take the namespace lock — one only another blocked writer
could release. Nothing timed out; the requests simply never returned.

Reproduced deterministically at the package level (`MaxConns` pinned to 4):
`took global=4, took namespace=0`, hung until the probe's own deadline.

**This is not test-only.** Any deployment whose concurrent fenced writes reach
the pool size wedges the data plane permanently.

### Fix — both halves are load-bearing

| Change | Why it alone is not enough |
|--------|----------------------------|
| `AcquirePair` takes **both** locks on one session | With two pools the cycle just moves inside the lock pool: writers still hold connection 1 while waiting for connection 2 |
| `PostgresLocker` owns a **dedicated pool**, separate from the work pool | With one pool, `k` writers holding `k` lock connections still deadlock waiting for work connections at `k >= MaxConns` |

Each was verified by reverting it in isolation against
`TestPostgresLockerDoesNotDeadlockASaturatedPool`: sharing the caller's pool
hangs the full 90s and fails; splitting the pair across two connections hangs
the full 90s and fails. Both together pass in 0.02s. The test holds the pair
*and* runs a query on the caller's pool, so it covers both halves of the cycle.

`TestPostgresLockerReleasesBothKeys` pins the other half of a one-session pair:
a released pair must leave zero advisory locks behind, or the next borrower of
that connection inherits locks it never took.

### What the e2e suite could and could not prove

Running the suite against an API server pinned to `pool_max_conns=4` — CI's
effective size, and confirmed honoured by `pgxpool.ParseConfig` — passed **both
before and after the fix**. The e2e deadlock needs all four writers holding the
global lock at once, and a fast local machine closes that window. The
package-level probe is the deterministic guard; the e2e run is not evidence
either way. It now also passes at `pool_max_conns=2`.

`postEventIgnoringStatus` used `http.DefaultClient`, which has no timeout — the
reason a stuck server became a silent 15-minute hang with a goroutine dump
instead of a named failure. It now uses a 30s client, so the writer reports a
transport error and the test fails with the diagnosis in it.

| Command | Result |
|---------|--------|
| `make lint` | **pass** — 0 issues, all four modules |
| `make build` | **pass** |
| `make test` | **pass** |
| `make test-race` | **pass** |
| `make coverage-check-all` | **pass** |
| `make test-e2e` | **pass** — 117 passed, 0 failed, 0 skipped (83s) |
| `make compose-check` | **pass** |
| `go mod verify` | **pass** |
| `make vuln` | **pass** — clean in all four modules |

## Phase 18 — CI parity

Executed 2026-08-26 after the first CI run on the pull request came back red.
**Every gate below had been recorded as passing, and CI still failed** — the
local gate list and the pipeline were not checking the same things.

| Command | Result |
|---------|--------|
| `make lint` | **pass** — 0 issues, all four modules |
| `make build` | **pass** — all four binaries |
| `make coverage-check-all` | **pass** — 16/16 package gates and the total |
| `make test-race` | **pass** |
| `make vuln` | **pass** — `No vulnerabilities found` in every module |
| `make compose-check` | **pass** |
| `go mod verify` | **pass** |

### `make coverage-check-all` was never a gate

It appears in no gate table above. CI runs it as its `test` job, so seventeen
convergence rounds ran `make test` — which ignores coverage — and never saw
what the pipeline actually enforces. It failed on three packages in sequence,
each hidden behind the one before it:

| Package | Was | Min | Why |
|---------|-----|-----|-----|
| `internal/core/idmap` | 66.0% | 90 | ~700 statements of repair code whose entry points (`Audit`, `Apply`, `applyFenced`, `applyItem`, `Resume`, `PrepareSnapshots`) had **zero** unit coverage — only e2e touched them, and e2e does not feed the unit profile |
| `internal/infra/redis` | 70.2% | 95 | `retention.go`'s go-redis adapter and `RunRetentionLoop` |
| `internal/catalog` | 84.6% | 85 | `UpsertWithAttribution` at 0% — the content+author atomicity contract |

Now 90.8% / 97.5% / 87.6%. The idmap tests were each checked by mutating the
production code and confirming the failure: dropping the `old == target` guard
in `applyItem`, the quarantine refusal, the per-collection snapshot check, the
`supersedable` gate, the manifest-hash comparison, and moving `RetargetMapping`
after the delete loop. All six were caught, one test each.

`redisRetentionBackend` gained four function seams (`xInfoGroupsFn` and
friends) so the go-redis translation is testable without a live server — the
same convention `pingClientFn` and `zRevRangeWithScoresFn` already use.

### `make vuln` passed locally on a technicality

CI installs the Go version named in `go.mod` (`go 1.26.1`); this machine runs a
newer patch. Every advisory CI reported was **standard library** — `crypto/tls`,
`crypto/x509`, `html/template`, `net/http`, `net/url`, `encoding/asn1`,
`encoding/xml` — fixed between go1.26.2 and go1.26.6, so the local pass measured
the local toolchain, not the one that builds the code.

The `go` directive moves to **1.26.7** in `go.mod`, `go.work` and both example
modules; `golang.org/x/net` to v0.56.0 and `golang.org/x/crypto` to v0.53.0.
Every module now reports `No vulnerabilities found`, import-level included. The
production images were never affected: `Dockerfile` pins `golang:1.26-alpine`,
which floats to the current patch.

### The e2e timeout fix had not reached CI

`ci.yml` ran `go test -tags=e2e -timeout=120s` directly instead of
`make test-e2e`, so raising the Makefile cap fixed only half of it — the exact
two-places-one-fixed drift the change was about. The job now calls the Makefile
target.

## Phase 17 (convergence) gates

Executed 2026-08-26 against the live stack.

| Command | Result |
|---------|--------|
| `make lint` | **pass** — 0 issues across all four modules |
| `make build` | **pass** — all four binaries |
| `make test` (with `DATABASE_URL`) | **pass** — every package |
| `make test-race` | **pass** — no data races |
| `make test-e2e` | **pass** — **117 passed, 0 failed, 0 skipped** (91s) |
| `make compose-check` | **pass** |
| `go mod verify` | **pass** |
| `make vuln` | **pass** — 0 vulnerabilities called |
| `git diff --check` | **pass**; 0 files changed under `web/admin` |

### T135 — the first test was too weak, and a mutation proved it

Phase 16 shipped the janitor's keyset query with no test at all; these are the
first execution of that SQL. Each new test was checked by mutating the query and
confirming it fails:

| Mutation | Caught |
|----------|--------|
| Keyset `>` → `>=` (cursor row repeats — the bug the cursor exists to prevent) | yes |
| `generate_series` upper bound for an active lifecycle `generation - 1` → `generation` | **no, at first** |

The second mutation is the dangerous one: it makes a **live** namespace's
current generation a deletion candidate. The original test asked for exactly the
four expected rows, so the two extra candidates fell past the `LIMIT` and were
never seen. Over-fetching and filtering to the fixture prefix catches it, and
the failure now names the live generations that would have been deleted.

The tests are `DATABASE_URL`-gated; confirmed they **skip** without it and
**pass** (not skip) with it, since a vacuous pass has been a repeated failure
mode on this feature.

### T136 — verified by removal

All three counter assertions were checked by deleting the `WithLabelValues`
line at their call site and confirming the test fails. Each also pins label
attribution — a catalog pass must not increment the events series, one
namespace's failure must not land on another, and reset must not share a series
with delete.

## Phase 16 (convergence) gates

Executed 2026-08-26 against the live PostgreSQL 17 / Redis 7 / Qdrant stack.

| Command | Result |
|---------|--------|
| `make lint` | **pass** — 0 issues across all four modules |
| `make build` | **pass** — all four binaries |
| `make test` (with `DATABASE_URL`) | **pass** — every package |
| `make test-race` | **pass** — no data races |
| `make compose-check` | **pass** — all three compose files valid |
| `go mod verify` | **pass** — all modules verified |
| `make vuln` | **pass** — 0 vulnerabilities called |
| `make test-e2e` | **pass** — **117 passed, 0 failed, 0 skipped** (154s) |
| `git diff --check` | **pass**; 0 files changed under `web/admin` |

### What running the gates found

**`make test-e2e` could not pass, and the timeout was only the symptom.** The
target carried `-timeout=120s`, so it aborted with `panic: test timed out` and
reported the tests in flight as failures. Raising the cap to 900s did not help:
the suite still timed out, now stuck 7 minutes into
`TestNamespaceLifecycle_ConcurrentWritersNeverOutliveADelete`.

The cause was in `newQdrantTestClient`, which dialled a **fresh gRPC client per
collection check** — a connect plus a version-check round trip each time — and
registered a `t.Cleanup` close for every one. The lifecycle race checks four
collections on each of 100 iterations, so it opened 400 clients and held every
connection until the test ended. That test alone took **274s**.

One process-wide client, closed in `TestMain`, takes it to **18s** — and the
whole suite from "times out past 900s" to **90s**. `mustAtoi` lost its only
caller and was removed. The three e2e targets now carry timeouts above their
real runtimes.

This is why the T108 entry above could record 117/117 in 238s under a 120s cap:
that number came from a direct `go test` with a longer timeout, and the
connection leak has been making the suite slower on every run since.

**A metric test passed alone and failed in the suite.** `StreamReclaimCyclesTotal`
is process-global and every other reap test in `internal/ingest` increments it,
so the new assertion had to reset the counter on entry, not only on cleanup.
Caught by `make test`, not by the focused run that preceded it.

### T130 — the janitor could not have reached most candidates

Wiring the janitor exposed a second defect in code the wiring made reachable:
`ListCleanupCandidates` took only a `LIMIT` and nothing marks a generation
reclaimed, so every pass returned the *same* first page forever. This database
holds 513 candidates against a 50-item batch — 463 of them were unreachable.
The query now takes a keyset cursor that the janitor carries across passes and
rewinds on a short page; the cursor advances only past fully cleaned
candidates, so a mid-page failure resumes on the item that failed. Verified by
paging the real query against the live ledger.

### T132 — measured before and after

`namespace_lifecycles` held 781 rows, 777 with no `namespace_configs` parent
and 272 of those still `state <> 'deleted'` — each one walked by
`ListNonDeleted` on every app-wide reset. The e2e cleanup also scanned
`rec:<ns>:*`, a pattern the live cache key shape (`rec:v2:<base64>`) has never
matched, so recommendation cache keys were never cleaned either. Both now
derive from the lifecycle resolver across every generation the namespace
reached.

841 already-orphaned lifecycle rows were purged from the development database
to clear the accumulated backlog; the fix only prevents new ones. A deployment
that has run these suites wants the same one-off cleanup:

```sql
DELETE FROM namespace_lifecycles l
WHERE NOT EXISTS (SELECT 1 FROM namespace_configs c WHERE c.namespace = l.namespace);
```

## Executed against a live stack — 2026-08-25

Infrastructure: the shared local PostgreSQL 17 / Redis 7 / Qdrant containers,
`codohue` database only, migrated to 027. Checkpoint dump at
`/tmp/codohue-backup/t109-checkpoint.dump`; Qdrant snapshots listed inline
below. Everything under "Outstanding" that follows was superseded by these
runs and is kept for the command references.

### T108 — full gate set

| Command | Result |
|---------|--------|
| `make lint` | 0 issues across all four modules |
| `make build` | all four binaries |
| `make test` (with `DATABASE_URL`) | all packages ok |
| `make test-race` | ok, no data races |
| `make test-e2e` | **117 passed, 0 failed, 0 skipped** (238s) |
| `make compose-check` | all three compose files valid |
| `go mod verify` | all modules verified |
| `make vuln` | 0 vulnerabilities called |

The first real run was 105/10/2 — ten failures across four distinct causes,
none of which compilation or reasoning had surfaced:

| Failure | Cause | Resolution |
|---------|-------|------------|
| 4 stream-ingest tests | **Production defect.** New namespaces were created with `legacy_messages_allowed = FALSE`, so every envelope published on the documented Redis Streams transport was acked and dropped with nothing returned to the producer | Generation 1 has one incarnation, so an unqualified envelope is unambiguous; both creation paths now seed the gate open unless it was closed fleet-wide. Pinned by three DB-backed tests in `internal/core/nslifecycle/legacy_gate_test.go` |
| `TestCronRecompute_NamespaceWithoutEventsProducesNoArtifacts` | **Production defect.** Enumerating namespaces from `namespace_configs` (needed for the expiry sweep) also materialised four Qdrant collections for every namespace that had never received an event | Phases 1 and 2 now gate on `Repository.HasAnyEvents`; a namespace whose events merely aged out still reaches the sweep |
| 2 healthz tests | Stale contract. They asserted per-dependency keys on public `/healthz`, removed when that surface was sanitized | Rewritten to pin both surfaces: public is aggregate-only, `?details=true` carries the breakdown and rejects the admin key |
| 3 idmap-repair + 2 recommendation-state tests | Defects in tests added by this work — wrong Qdrant vector name, missing FK parents, counting a collection before cron creates it | Fixed against the established patterns |

Running the unit suite with `DATABASE_URL` set (it had been skipping) exposed
18 further failures across five packages, all the same shape: migration 025's
foreign keys mean a test that seeds an event, object or catalog item for an
invented namespace gets an FK error, not a row. Each package now has an
`ensureNamespace` helper called from its lowest-level seeder.

### T109 — migration and operational rehearsal

| Step | Result |
|------|--------|
| 1. Migrations 024–027 up | Clean to 027. Required deleting 339 orphan rows first — migration 025's integrity preflight refuses while they exist, and e2e cleanup had left them behind (see the finding below) |
| 2. Legacy-gate closure | `changed=true` then `changed=false` on the second run; all 224 namespace gates driven to FALSE. Restored afterwards, since closure is deliberately one-way |
| 3. Ambiguous repair refusal | Two points claiming one object id → audit quarantined the identity; `apply` refused with **exit code 1** and a byte-identical mapping digest and point count before and after |
| 4. Repair resume | Interrupted mid-apply (SIGKILL): run left `applying`, 15 items `cleaned`, 14 `pending`. After two fixes below, `resume` continued to 29 cleaned and rebuilt sparse vectors |
| 5. Coordinated snapshot restore | Collection wiped, then recovered from the checkpoint snapshot; PostgreSQL dump restored to a scratch database. Both stores agreed on the checkpoint — the mapping and the point matched, and a namespace created after the checkpoint was absent |
| 6. Migration-down safety | `022` down refused on a cross-namespace duplicate `string_id`; `026` down refused on a shared `numeric_id`. Both raised before any constraint mutation — the composite primary key and both repair tables survived intact |

Step 4 did not pass on the first attempt, and what blocked it were two defects
that only a live interrupt could expose:

- **`ManifestHash` included `item.State`.** Apply advances item state as it
  runs, and both apply and resume compare the recomputed hash against the
  recorded one before doing anything — so the first partial apply made the
  recorded hash unreproducible and `resume` could never start. The hash now
  covers the audited decision only. Quarantine is still enforced, by the
  explicit gate in `applyFenced` that never depended on the hash.
- **The sparse rebuild ran without a namespace lease.** It executes inside the
  repair's global exclusive fence, which carries no per-namespace lease, while
  minting a numeric id requires one — so every subject upsert failed with
  `ErrLeaseRequired` and took the run down. Acquiring a real lease would
  deadlock against the fence (fixed order: global before namespace), so the
  adapter now asserts one with `nslifecycle.ContextWithLease`.

Two smaller things the rehearsal surfaced:

- `Verify`'s summary line reported "N old point(s) still present" for every
  failure, including "target point is missing" — pointing an operator at
  leftovers when the collection was empty. It now names the count without
  asserting a cause, and quotes the first specific problem.
- A failed down migration leaves golang-migrate's `schema_migrations` row
  `dirty = true` even though the schema is untouched (the transaction rolls
  back). An operator must `migrate force <version>` before retrying; the
  refusal messages should say so.

### Findings for the deployment runbook

1. **Migration 025 will refuse on any deployment with historical deleted
   namespaces.** Its integrity preflight found 339 orphan namespaces here —
   rows in `batch_run_logs`, `catalog_items`, `objects` and
   `catalog_backlog_samples` whose `namespace_configs` row was deleted long
   ago. [deploy/backend-audit-remediation.md](../../deploy/backend-audit-remediation.md)
   gives no remediation query, so an operator hits a hard stop with no
   documented next step.
2. **The e2e cleanup is the source of those orphans.** It deletes
   `namespace_configs` but not the four tables that reference it, so every
   suite run leaves more behind.

## Outstanding — requires a live environment

Superseded by the runs above; kept for the exact command references.

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
