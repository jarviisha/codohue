# Backend audit remediation — rollout

Four gated releases. Each one is independently deployable and independently
reversible; do not cross a gate until its evidence is recorded in
`specs/007-backend-audit-remediation/verification.md`.

| Release | Contents | Schema | Reversible by |
|---------|----------|--------|---------------|
| 1 | Dependency upgrades, honest failures, finite scores, catalog keyset cursor, observability auth | none | redeploy previous image |
| 2 | Exact stream retention + reclaim cursor fairness | none | kill switch (below) |
| 3 | Namespace lifecycle generations, leases, delete/reset, legacy-gate closure | 024, 025 | migrate down (before the gate closes) |
| 4 | Migration-022 identity reconciliation | 026 | coordinated snapshot restore only |

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

**Rollback**: `make migrate-down` twice, *before* the legacy gate is closed.
After closure the gate timestamp is durable and a down-migration drops it; you
would reopen a window you deliberately closed. Treat post-closure as
forward-only.

## Release 4 — migration-022 identity reconciliation

See [idmap-repair-runbook.md](idmap-repair-runbook.md) for the full workflow.
Summary of the gate:

1. Apply migration 026.
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
                      verify passed: <date>
post-gate recompute:  <batch_run_logs id, success y/n>
```
