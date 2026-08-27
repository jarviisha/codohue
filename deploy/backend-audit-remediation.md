# Backend audit remediation — rollout

Four gated releases. Each one is independently deployable and independently
reversible; do not cross a gate until its evidence is recorded in
`specs/007-backend-audit-remediation/verification.md`.

| Release | Contents | Schema | Reversible by |
|---------|----------|--------|---------------|
| 1 | Dependency upgrades, honest failures, finite scores, catalog keyset cursor, observability auth | none | redeploy previous image |
| 2 | Exact stream retention + reclaim cursor fairness | none | kill switch (below) |
| 3 | Namespace lifecycle generations, leases, delete/reset, legacy-gate closure | 024, 025 | migrate down (before the gate closes) |
| 4 | Migration-022 identity reconciliation | 026, 027 | coordinated snapshot restore only |

## Release 1 — migration-free correctness

Ships together because none of it touches schema or storage layout:

- Upgraded `pgx`, `golang.org/x/text` and gRPC to fixed releases (`make vuln`).
- Data-plane failures answer 404 / 409 / 503 instead of a blanket 500.
- `object_created_at` beyond five minutes future skew is rejected with
  `invalid_object_created_at`, on events and BYOE alike.
- Recommendation, ranking and trending resolve namespace configuration before
  touching the cache, and never cache a default-backed result.
- Namespace config and catalog attribution writes are atomic.
- Catalog reconciliation exposes an additive `next_cursor`; offset paging keeps
  working for one deprecation window.
- `/metrics` and `/healthz?details=true` require `CODOHUE_OBSERVABILITY_TOKEN`.

**Client-visible change**: a client that treated every non-2xx as retryable now
sees 404s it should stop retrying. That is the point — verify your integration
distinguishes them before deploying.

**Rollback**: redeploy the previous image. No state changes.

## Release 2 — exact stream retention

Producers no longer set `MAXLEN`, so streams grow until retention trims them.
Retention computes a frontier from every consumer group's progress and trims
strictly below it — an entry pending in *any* group is never removed.

Roll out in this order:

1. Deploy with `CODOHUE_STREAM_RETENTION_ENABLED=false` (the default). Streams
   grow; nothing is trimmed. Watch `codohue_stream_length` and
   `codohue_stream_unexpected_groups`.
2. Confirm every expected consumer group is present and that
   `codohue_stream_unexpected_groups` is zero. An unexpected group holds the
   frontier back — that is safe, but you want to know it exists.
3. Enable retention on the generation-1 embed streams first (smallest blast
   radius), then `codohue:catalog`, then `codohue:events`.
4. Watch `codohue_stream_trimmed_total` rise and `codohue_stream_pending` stay
   flat. Pending work must never drop as a result of a trim.

**Kill switch**: set `CODOHUE_STREAM_RETENTION_ENABLED=false` and restart. No
data is recovered by this — a trim is not reversible — but nothing further is
removed. This is why step 1 exists.

**Do not** create a backfill consumer group from `0` while retention is
enabled: the group starts behind the frontier and its work may already be
gone. Disable retention until it catches up.

## Release 3 — namespace lifecycle

Apply migrations 024 and 025, then deploy. Every namespace is backfilled to
generation 1 and keeps its existing physical names, so nothing moves on
upgrade.

### PostgreSQL connection budget — check before deploying

This release doubles the connections each binary can open. Lifecycle leases are
session advisory locks, so a lease lives on its connection for the whole fenced
write; `NewPostgresLocker` gives them a **dedicated pool** so a lock holder can
never be waiting for a work connection that only another lock holder could
release. Sharing one pool deadlocks the data plane permanently once concurrent
fenced writes reach the pool size — see the Phase 19 note in
`specs/007-backend-audit-remediation/verification.md`.

Neither pool sets `MaxConns`, so pgxpool's default of `max(4, NumCPU)` applies
to both. Per host, the ceiling is:

```text
4 binaries x 2 pools x max(4, NumCPU) connections
```

On a 16-core host that is 128 — above PostgreSQL's default `max_connections`
of 100. The lock pool runs at `MinConns=0`, so idle deployments sit well under
the ceiling and the exhaustion only appears under concurrent load.

Pick one before deploying:

- Raise `max_connections` on the server to cover the ceiling above, or
- Cap each binary with the `pool_max_conns` DSN parameter, which
  `pgxpool.ParseConfig` honours and which applies to the lock pool too because
  it is derived from the work pool's config (floored at 4):

  ```text
  DATABASE_URL=postgres://codohue:secret@postgres:5432/codohue?sslmode=disable&pool_max_conns=8
  ```

A too-small cap costs latency, not correctness: writers queue for a lock
session instead of deadlocking. The suite passes at `pool_max_conns=2`.

### Orphan preflight before migration 025

Migration 025 adds the namespace foreign keys, and it **refuses to run** while
any child row points at a `namespace_configs` row that no longer exists:

```text
ERROR: namespace integrity preflight failed: orphan rows must be repaired before migration 025
```

Any deployment that has ever deleted a namespace will hit this — namespace
deletion predates the FKs, so it left child rows behind. The rehearsal for this
release found 339 orphan namespaces on a development database. Audit first:

```sql
SELECT 'events' AS child_table, count(*), count(DISTINCT c.namespace) AS namespaces
  FROM events c LEFT JOIN namespace_configs p USING (namespace) WHERE p.namespace IS NULL
UNION ALL SELECT 'id_mappings', count(*), count(DISTINCT c.namespace)
  FROM id_mappings c LEFT JOIN namespace_configs p USING (namespace) WHERE p.namespace IS NULL
UNION ALL SELECT 'batch_run_logs', count(*), count(DISTINCT c.namespace)
  FROM batch_run_logs c LEFT JOIN namespace_configs p USING (namespace) WHERE p.namespace IS NULL
UNION ALL SELECT 'catalog_items', count(*), count(DISTINCT c.namespace)
  FROM catalog_items c LEFT JOIN namespace_configs p USING (namespace) WHERE p.namespace IS NULL
UNION ALL SELECT 'catalog_backlog_samples', count(*), count(DISTINCT c.namespace)
  FROM catalog_backlog_samples c LEFT JOIN namespace_configs p USING (namespace) WHERE p.namespace IS NULL
UNION ALL SELECT 'objects', count(*), count(DISTINCT c.namespace)
  FROM objects c LEFT JOIN namespace_configs p USING (namespace) WHERE p.namespace IS NULL;
```

Review the namespace list before deleting anything — an orphan is normally a
deleted tenant, but a row here would also be the symptom of a
`namespace_configs` row lost by accident, and that case wants a restore, not a
purge:

```sql
SELECT DISTINCT namespace FROM (
  SELECT namespace FROM events UNION SELECT namespace FROM id_mappings
  UNION SELECT namespace FROM batch_run_logs UNION SELECT namespace FROM catalog_items
  UNION SELECT namespace FROM catalog_backlog_samples UNION SELECT namespace FROM objects
) AS child
WHERE NOT EXISTS (SELECT 1 FROM namespace_configs p WHERE p.namespace = child.namespace)
ORDER BY namespace;
```

Once confirmed to be deleted tenants, purge them in one transaction. Take a
backup first: this deletes behavioral history that cannot be recomputed.

```sql
BEGIN;
DELETE FROM events c WHERE NOT EXISTS (SELECT 1 FROM namespace_configs p WHERE p.namespace = c.namespace);
DELETE FROM id_mappings c WHERE NOT EXISTS (SELECT 1 FROM namespace_configs p WHERE p.namespace = c.namespace);
DELETE FROM batch_run_logs c WHERE NOT EXISTS (SELECT 1 FROM namespace_configs p WHERE p.namespace = c.namespace);
DELETE FROM catalog_items c WHERE NOT EXISTS (SELECT 1 FROM namespace_configs p WHERE p.namespace = c.namespace);
DELETE FROM catalog_backlog_samples c WHERE NOT EXISTS (SELECT 1 FROM namespace_configs p WHERE p.namespace = c.namespace);
DELETE FROM objects c WHERE NOT EXISTS (SELECT 1 FROM namespace_configs p WHERE p.namespace = c.namespace);
COMMIT;
```

Re-run the audit — every count must be zero — then apply 025. After it lands
the FKs cascade, so namespace deletion cannot create new orphans.

**If the migration already failed**, golang-migrate has recorded the version as
dirty even though the transaction rolled back and the schema is untouched.
Clear it before retrying:

```bash
make migrate-version                    # reports the dirty version
migrate -path migrations -database "$DATABASE_URL" force 024
make migrate-up
```

### Generation adoption window

Producers that predate this release send no `namespace_generation`. Those
"legacy envelopes" are accepted while the global gate is open, but **only for
generation-1 namespaces** — a recreated namespace is generation 2, and a
generation-less envelope for it is stale by definition.

1. Deploy the server. Legacy envelopes keep working.
2. Upgrade every producer (SDK `>= v0.10`, or your own client) to stamp the
   generation returned at namespace provisioning.
3. Watch `codohue_stale_generation_total`. A non-zero rate with reason
   `legacy` means a producer has not been upgraded.

### Closing the gate

Only after adoption is complete:

```bash
./tmp/admin lifecycle disable-legacy-envelopes --all \
  --adoption-evidence "deploy/2026-08-25-producer-upgrade#all-clients"
```

The evidence string is required and recorded — it is what a later reviewer
reads to understand why the gate was closed. The command is idempotent: a
second invocation reports `changed=false`.

**This is a one-way door.** After closure, a generation-less envelope is
dropped rather than processed. Do not close it while any producer still sends
one.

### Enabling delete/reset

Namespace delete and app reset become resumable and verified once 024 is
applied. They hold the exclusive lifecycle lease, so they serialize against
every writer. The deleted-generation janitor — which removes superseded
generations' Redis keys and Qdrant collections — refuses to run until the
legacy gate is closed, because an open gate means old work may still arrive.

### Where the janitor runs

It is a background loop inside **`cmd/cron`**, alongside the observability-table
retention prune. It ticks on `CODOHUE_RETENTION_INTERVAL` (default `1h`), takes
an immediate first pass at startup, and reclaims at most **50** superseded
generations per pass, resuming from a keyset cursor so successive passes walk
the whole ledger rather than re-cleaning the head.

Nothing marks a generation reclaimed, so the walk is cyclic by design: once it
reaches the end it starts over, and a generation whose artifacts are already
gone costs one Redis `SCAN` and four Qdrant `CollectionExists` calls that all
find nothing. That repetition is what makes the pass safe to retry.

What to expect in the cron logs:

| Log line | Meaning |
|----------|---------|
| `deleted-generation cleanup idle until legacy envelopes are disabled` | Normal before closure. Logged once per process, not per tick |
| `reclaimed superseded generations count=N` | A pass removed N generations' artifacts |
| `deleted-generation cleanup hit its per-pass bound` | More than 50 candidates are waiting; later passes continue through them |
| `deleted-generation cleanup failed` | A store was unreachable. The pass claims no progress and retries next tick |

The loop never blocks cron's recompute cycle, and a failure here is not fatal to
the binary — the artifacts simply stay until a later pass succeeds.

**Rollback**: `make migrate-down` twice, *before* the legacy gate is closed.
After closure the gate timestamp is durable and a down-migration drops it; you
would reopen a window you deliberately closed. Treat post-closure as
forward-only.

## Release 4 — migration-022 identity reconciliation

See [idmap-repair-runbook.md](idmap-repair-runbook.md) for the full workflow.
Summary of the gate:

1. Apply migrations 026 and 027. **Both** — 027 adds
   `id_mapping_repair_runs.rebuilt_namespaces`, which verification reads; without
   it every apply and verify fails on a missing column mid-window.
2. Run `./tmp/admin idmap-repair audit` — read-only. If it reports zero
   quarantined items and zero needing repair, you are done.
3. Otherwise resolve every quarantined item, re-audit, take coordinated
   PostgreSQL + Qdrant snapshots, and run `apply` inside a maintenance window.
4. `verify` must pass before the fleet is unlocked.

**Rollback**: forward-only once composite duplicates exist. Both 022 and 026
down-migrations refuse before touching a constraint when duplicates are
present. A true rollback restores both stores from the same checkpoint.

## Compensating procedure — re-embed after vector loss

If a repair, a Qdrant incident, or a restore leaves a namespace's dense vectors
inconsistent with `catalog_items`, re-drive the corpus regardless of which
strategy produced it:

```bash
curl -X POST "$ADMIN/api/admin/v1/namespaces/$NS/catalog/re-embed" \
  -H "Authorization: Bearer $CODOHUE_ADMIN_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"only_state":"all"}'
```

`only_state=all` re-drives every embedded/failed/dead-letter row **regardless
of strategy version** — that is the difference from an omitted body, which
re-drives only rows at a different `(strategy_id, strategy_version)`. Use it
when the vectors are gone but the rows still claim to be embedded.

Progress: `GET /api/admin/v1/namespaces/{ns}/catalog` (backlog) and the
`catalog.reembed_progress` SSE event. The run closes when the backlog drains.

## Per-namespace completion evidence

Record this for each namespace as it clears each gate. Paste into
`specs/007-backend-audit-remediation/verification.md`.

```
namespace:            <name>
lifecycle generation: <n>
release 1 deployed:   <date, image tag>
release 2 retention:  disabled | enabled  (trimmed_total at cutover: <n>)
release 3 generation: adopted <date>; legacy gate closed <date or "open">
                      adoption evidence: <string passed to disable-legacy-envelopes>
release 4 repair:     not required | run <id>, manifest <hash>
                      pg snapshot: <ref>
                      qdrant snapshots: <collection>=<ref>, ...
                      migrations applied: 026 __ 027 __
                      verify passed: <date>
post-gate recompute:  <batch_run_logs id, success y/n>
```
