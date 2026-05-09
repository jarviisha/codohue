# Contract: Redis Streams transport

**Feature**: 004-catalog-embedder
**Date**: 2026-05-09
**Realises**: FR-003, FR-004, FR-010 (uniform retry/dead-letter), R4, R5

This contract pins the message format and consumer-group semantics of the queue between `cmd/api`'s catalog ingest handler and `cmd/embedder`'s worker. Mirrors the existing `ingest` worker pattern.

---

## Stream naming

| Item | Convention |
|---|---|
| Stream name | `catalog:embed:{namespace}` |
| Consumer group | `embedder` |
| Consumer name | `{hostname}:{pid}` (one consumer per `cmd/embedder` replica process) |

One stream per namespace prevents head-of-line blocking across tenants. The consumer group is created on first use with `XGROUP CREATE catalog:embed:{ns} embedder $ MKSTREAM` (`MKSTREAM` so the stream is auto-created).

---

## Entry format (XADD payload)

```
XADD catalog:embed:{ns} *
  catalog_item_id  <int64-as-string>
  namespace        <string>
  object_id        <string>
  strategy_id      <string>
  strategy_version <string>
  enqueued_at      <RFC 3339 UTC>
```

| Field | Required | Notes |
|---|---|---|
| `catalog_item_id` | yes | Postgres `catalog_items.id`. The worker re-reads the row to get `content` and current strategy assignment — never trusts the stream entry's content. |
| `namespace` | yes | Redundant with stream name but kept for log greppability. |
| `object_id` | yes | Convenience for log readability. |
| `strategy_id` | yes | Strategy at time of enqueue. **Worker IGNORES this** and re-resolves from `namespace_configs` at processing time — that re-resolution is what realises Q2 ("new ingests use new active version"). The field exists for auditing only. |
| `strategy_version` | yes | Same as above — for audit, not authority. |
| `enqueued_at` | yes | For p95 freshness latency calculation (SC-002). |

The entry MUST NOT carry `content` — content lives in Postgres. Keeping the entry small keeps Redis memory bounded and lets the worker pick up the latest content if a re-ingest happened between enqueue and dequeue.

---

## Worker consumption protocol

```
XREADGROUP GROUP embedder <consumer-name> COUNT 32 BLOCK 5000 STREAMS catalog:embed:{ns} >
```

Workers read in batches of 32, blocking up to 5 s for new entries. After processing each entry:

- Success (`state='embedded'`) → `XACK catalog:embed:{ns} embedder <entry-id>`.
- Hard failure (zero-norm, dim-mismatch, oversized, dead-letter): `XACK` AND set `catalog_items.state='dead_letter'` in Postgres.
- Transient failure: NO `XACK`. The entry remains in the pending-entries-list (PEL) for that consumer. The worker increments `catalog_items.attempt_count` and updates `last_error`.

A separate goroutine per replica runs `XAUTOCLAIM catalog:embed:{ns} embedder <consumer-name> 60000 0 COUNT 100` every 60 s to reap entries idle in another consumer's PEL (i.e. crashed-consumer recovery). Reaped entries are processed identically.

When `attempt_count >= namespace.catalog_max_attempts`, the worker moves the catalog row to `dead_letter` and `XACK`s the entry. The operator's redrive endpoint resets the row back to `pending` and re-publishes.

---

## Per-namespace and global concurrency

- One consumer group `embedder` per stream; multiple consumers within the group split the stream's pending entries.
- Per-replica concurrency: one goroutine per active stream, multiplexed via per-stream goroutines (NOT a single multiplexer using `reflect.SelectCase`, which Redis doesn't natively support across multiple `XREADGROUP` calls).
- A per-replica "namespace registry" goroutine subscribes to namespace-config changes (via Postgres `LISTEN/NOTIFY` on a `catalog_config_changed` channel; nice-to-have, not required for V1) and starts/stops per-stream goroutines as namespaces enable / disable catalog. For V1, polling `namespace_configs WHERE catalog_enabled=true` every 30 seconds is acceptable since catalog enablement is rare.

---

## Failure modes and recovery

| Failure | Recovery |
|---|---|
| `XADD` fails after Postgres commit | A periodic recovery sweep in `cmd/embedder` (every 60 s per active namespace) runs `SELECT id FROM catalog_items WHERE namespace=$1 AND state='pending' AND id NOT IN (...stream pending ids...)` and re-publishes. The "NOT IN" guard prevents duplicate publishes for entries already in the PEL. |
| Embedder replica crashes mid-process | `XAUTOCLAIM` reaps the entry after 60 s of idle time; another replica re-processes. The operation is idempotent on Postgres (UPSERT by `(ns, object_id)`) and Qdrant (UPSERT by point id). |
| Stream itself lost (Redis flush) | The recovery sweep above re-publishes every `pending` and `failed` row. `embedded` and `dead_letter` rows are unaffected. Operator may also call the bulk re-embed endpoint. |
| Namespace config changes mid-flight | Worker re-resolves strategy on next `Embed` call; the entry currently being processed is re-tagged with the *old* strategy if it succeeded before the config change observed. This is acceptable per Q2 (mixed-version is legitimate during transition). |

---

## Observability

| Metric | Source |
|---|---|
| `XLEN catalog:embed:{ns}` | Redis. Mirrors `catalog_pending_total{namespace}` Prometheus gauge. |
| `XPENDING catalog:embed:{ns} embedder` | Redis. Mirrors `catalog_inflight_total{namespace}` Prometheus gauge. |
| Embed duration per item | Histogram `catalog_embed_duration_seconds{namespace,strategy_id,strategy_version}` (R10). |
| Per-entry attempt count | Postgres `catalog_items.attempt_count`; not exported as a per-item metric, only aggregated. |

The admin endpoint `GET /api/admin/v1/namespaces/{ns}/catalog` joins Redis stream stats with the Postgres state counts to render a single backlog object (see `rest-api.md`).
