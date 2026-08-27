# Stream Processing Contract

## Supported streams and owners

| Stream | Required group | Durable result before ACK |
|--------|----------------|---------------------------|
| `codohue:events` | `codohue-ingest` | Event committed to PostgreSQL |
| `codohue:catalog` | `codohue-catalog-ingest` | Catalog row committed and embed work durably represented |
| generation-1 legacy or generation-qualified `catalog:embed` | `embedder` | Vector/state terminally committed or work permanently rejected |

Additional groups are protected by retention but generate an operational alert. Creating a
backfill group from `0` requires retention to be disabled until that group catches up; live-only
groups start at `$`.

## Envelope

Event, catalog, and embed JSON payloads add:

```json
{
  "namespace": "tenant-a",
  "namespace_generation": 3
}
```

The field is additive on the wire. SDK producers expose it explicitly and may be configured
with a default generation obtained at namespace provisioning.

## Delivery outcome

| Outcome | ACK? | Retry? | Delete/trim eligibility |
|---------|------|--------|-------------------------|
| Durable success | Yes | No | Yes, after all group frontiers pass it |
| Permanent payload/domain rejection | Yes | No | Yes, after all group frontiers pass it |
| Namespace generation mismatch/deleted | Yes | No | Yes; emit stale-generation metric |
| Lifecycle/config/storage unavailable | No | Yes | No |
| Process crash before ACK | No | Yes through reclaim | No |

At-least-once delivery remains the contract. Domain writes must remain idempotent.

## Retention

Producers do not set `MAXLEN` and streams have no TTL. A retention pass:

1. Reads every group with `XINFO GROUPS`.
2. Reads PEL summary for each group.
3. Uses the oldest pending ID when pending exists; otherwise uses last-delivered ID.
4. Selects the minimum frontier across all groups.
5. Executes exact `XTRIM MINID` strictly below that frontier.

No group, malformed/contradictory progress, or Redis error means no trim. A successful pass has
one measurable post-condition: no stream entry remains with an ID strictly below the safe
frontier. Entries at or above the frontier remain protected even when an individual consumer has
already processed them.

Configuration:

- `CODOHUE_STREAM_RETENTION_ENABLED`, default false for the first rollout.
- `CODOHUE_STREAM_RETENTION_INTERVAL`, default `1m`.

Release 2 discovers only existing generation-1 embed stream names. Release 3 extends discovery
to generation-qualified physical names after the lifecycle resolver is deployed.

## Reclaim cursor

Each worker retains its returned `XAUTOCLAIM` cursor. It continues even if a page contains no
eligible messages, stops after 10 pages per tick, and resets only when Redis returns terminal
`0-0`. Errors retain the current cursor; `NOGROUP` resets after group recreation. Embedder
cursors are independent per namespace generation.

## Required observability

- Trimmed entries, retention failures by stage, unexpected group count.
- Stream length, pending and undelivered counts.
- Reclaimed entries and completed cursor cycles.
- Oldest pending age alerts plus Redis memory thresholds.

Namespace labels are used only for bounded per-namespace embed streams; global input metrics
do not parse arbitrary tenant labels from payloads.
