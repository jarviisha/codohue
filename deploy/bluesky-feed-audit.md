# Bluesky feed audit — first 24h on development

Findings from running `examples/bskyfeed` against the development stack on
2026-09-16/17. The feeder itself worked; what it exposed is a set of capacity
and throughput limits that were latent until something produced events
continuously instead of in short manual bursts.

Everything below is measured on the development host, not projected.

## Outcome first

The recommendation loop closed on real data. This is the result the exercise
was for:

| Metric | Value |
| --- | --- |
| Events ingested | 846,249 over 7h32m (~31/sec at 10% actor sampling) |
| Subjects | 59,302 |
| Objects | 319,200 |
| **Subjects with ≥5 interactions** | **28,350 (47.8%)** |
| Action mix | LIKE 678,789 · SHARE 110,589 · COMMENT 56,871 |
| Catalog | 90,973 embedded · 62 dead-letter · 22 pending |
| Qdrant | subjects 59,302 · objects 313,729 · subjects_dense 17,018 · objects_dense 90,973 |
| Recommendation | `source: hybrid`, top score 0.6816 |

`source: hybrid` rather than `fallback_popular` is the part that matters: the
collaborative filtering path had enough repeat behaviour to serve real
neighbours. 47.8% of subjects crossing `coldStartThreshold = 5`
(`internal/recommend/service.go:29`) within 7.5 hours is the density that makes
that possible.

## Incident: Redis exhausted its 256MB cap

Ingestion stopped for **2h10m** and trending was empty for **4h08m**. Both
trace to one cause with two different onset times.

### Timeline

| Time | Event |
| --- | --- |
| 16:12:39 | Feeder starts (`development-689b08d`, sample 10%) |
| 20:31:34 | Last batch run with `phase3_ok = t` (248,529 trending items) |
| ~21:03 | Trending key written, `TTL 3600s` |
| 21:04:52 | **Phase 3 starts failing** — OOM on the large `ZADD` |
| ~22:03 | Trending key expires; nothing can rewrite it → **trending empty** |
| 23:44:43 | **Last event ingested** — `XADD` now fails too |
| 01:32 | Audit begins. `used_memory 256.17M / maxmemory 256M`, `noeviction` |
| ~01:55 | Streams trimmed at verified-safe offsets → **256.30M → 31.95M** |
| 01:59:34 | Ingest resumes |
| 02:11:37 | Phase 3 succeeds, trending restored (329,600 members) |

### Why phase 3 died 2h40m before ingest

Both are `denyoom` commands, but allocation size decides which fails first.
Phase 3 writes one ZSET of ~250k–330k members in a single operation; `XADD`
writes tens of bytes. Redis rejected the large write long before the small
ones, so **phase 3 was an early-warning signal that nothing was watching**, and
the outage only became visible when ingest stopped almost three hours later.

Worth noting the trending ZSET is itself a major consumer of the memory it
then fails to allocate from.

## Findings

Ranked by whether they are actively causing harm right now.

### F1 — NUL bytes create a poison-pill loop (FIXED)

Fixed in `internal/catalog` and `cmd/api`; see "Resolution" at the end of this
section. The diagnosis below is what was observed before the fix.

Bluesky post text containing `0x00` cannot be stored in a PostgreSQL `text`
column:

```
persist catalog item: upsert catalog item:
ERROR: invalid byte sequence for encoding "UTF8": 0x00 (SQLSTATE 22021)
```

The catalog worker leaves such an entry pending by design, so it is redelivered
forever — observed delivery counts of 482, 427, 372, 319, 263, 212, all from a
single author DID, across 2,098 `catalog ingest failed; leaving entry pending`
log lines.

The consequence is worse than the lost items: the oldest pending entry pins the
stream, so `XTRIM MINID` cannot reclaim past it. After trimming, `codohue:catalog`
still held **80,004 entries** for the sake of 6 stuck messages. **This directly
defeats the only mechanism currently reclaiming Redis memory.**

#### Resolution

Fixed at the trust boundary rather than at the producer, because every SDK
consumer can send NUL and only the server sees them all.

The original diagnosis naming only `content` was incomplete: `object_id`,
`content`, `metadata`, and `author_subject_id` all ride one transaction, so
any of them could fail it.

`catalog.Service.Ingest` now **rejects identifiers and sanitizes payload**.
`content` and `metadata` (recursively, keys included) are stripped, so the
item survives a junk byte. `object_id` and `author_subject_id` are rejected
instead — stripping them would merge two distinct keys, since `object_id` is
half of `UNIQUE (namespace, object_id)` on `catalog_items` and of the `objects`
primary key, so `a\x00b` and `ab` would clobber each other's content and
embedding. Stripping an identifier would not even buy what sanitizing is for:
the caller never gets its own key back from `ListObjects`, so it re-sends the
item forever anyway.

As a backstop for unstorable values the sanitize does not model, a persist
that fails with PostgreSQL SQLSTATE class `22` (data exception) now returns
`catalog.ErrUnstorable`, which the stream adapter treats as permanent.
Redelivery would hand the database identical bytes for an identical error.
This is deliberately scoped to the write — the same SQLSTATE from a config
read or a lease probe is a defect in that query, not unstorable content, and
stays transient so the entry is retried rather than silently acked off.

`examples/bskyfeed` was deliberately left unchanged — the server fix covers
every producer. One loose end remains there: `decoder.post` gates on
`utf8.RuneCountInString`, which counts invalid bytes as runes, so a post of
mostly junk passes the producer's length filter and is dropped server-side as
`empty_content` instead. Harmless, but the filter is looser than it reads.

The 6 entries stuck at audit time ack on their next redelivery, releasing the
80,004 entries they pinned in `codohue:catalog`. No manual step needed.

Not fixed, and tracked separately: `internal/infra/redis/stream_consumer.go`
has no max-delivery cap, and `internal/ingest/worker.go` (the events stream)
has no permanent-rejection class at all — the same defect shape, on the larger
stream.

### F2 — Stream retention is disabled, so streams grow without bound

`CODOHUE_STREAM_RETENTION_ENABLED` is unset and defaults to `false`.
Consumed entries are never trimmed: `codohue:events` reached **846,252 entries
(244MB, 95% of the cap)** while reporting `lag=0, pending=0` — every entry had
long since been persisted to Postgres and was pure dead weight.

This is not specific to bskyfeed. Any continuously running producer reaches the
same end; bskyfeed was simply the first to run 24/7 long enough to expose it.

`.env.example` documents the flag as deliberately off *"until the dry-run
frontier canary has been verified"*, so enabling it is a decision that needs
that verification first, not a config tweak.

Interim: `docker exec … redis-cli XTRIM <stream> MINID <id>`, choosing the id
from `XINFO GROUPS` — last-delivered where `pending=0`, oldest pending id
otherwise, so nothing undelivered or unacked is discarded.

### F3 — Phase 1 is 84% of batch time (N+1 queries and an O(n²) inner loop)

| Phase | Duration |
| --- | --- |
| Phase 1 sparse | 2,259s (37.6 min) |
| Phase 2 dense | 417s (7 min) |
| Phase 3 trending | 2s |

38ms per subject × 59,302 subjects. The loop in `RecomputeNamespace`
(`internal/compute/service.go:84`) costs three separate network round-trips per
subject:

- `GetSubjectEvents` — one Postgres query per subject (`service.go:177`)
- `upsertSubjectVector` — one Qdrant upsert per subject, with `Points`
  containing exactly one point (`service.go:248`)
- `accumulateObjectCooccurrence` — `GetOrCreateObjectID` per object per
  subject, uncached (`service.go:152`)

The third is the cheapest to fix: a batch variant, `GetOrCreateObjectIDs`,
already exists at `internal/core/idmap/service.go:95` and is simply not used
here.

Separately, the co-occurrence loop at `service.go:159` is **O(n²) in the number
of objects a subject touched**. Bluesky routinely produces users with 400+
likes in a minute; for those the inner loop runs into the hundreds of
thousands.

### F4 — `trending_ttl` is shorter than the batch cycle is trending toward

| | |
| --- | --- |
| `trending_ttl` | 3,600s (60 min) |
| Observed batch cycle | 45.9 min |
| Margin | 14 min |

Any run slower than usual, or any single phase-3 failure, leaves a window with
no trending at all. Because phase 1 grows with the dataset (F3), the cycle will
cross 60 minutes, and at that point **trending is permanently empty even with
zero errors** — the key always expires before the next write.

Quick mitigation: raise `trending_ttl` (e.g. 10800). Root fix is F3.

### F5 — The trending ZSET is the largest key and is growing

```
trending:bluesky   329,600 members   55,349,031 bytes (55MB)
```

61% of used memory, up from 248,529 members roughly six hours earlier — ~33%
growth. It scales with object count, which scales with ingest. It competes for
the same budget as the streams, and being a single large allocation it is what
fails first (see the timeline).

### F6 — `CODOHUE_BATCH_INTERVAL_MINUTES` has no effect

Set to 5; a full cycle across all namespaces takes **45.9 min**. Cron logs
`batch run done` immediately followed by `batch run started` — it never idles.
The configured interval is inert, and the stack is burning CPU continuously on
a 2-vCPU host.

### F7 — The `events` table has no retention

846,249 rows in 7.5 hours, projecting to ~3M/day at 10% sampling.
`internal/retention` bounds `batch_run_logs` and `catalog_backlog_samples`
only. Postgres was 787MB with 6.9GB free at audit time, so this is not urgent,
but it is unbounded and — for real people's data — overlaps with the storage
limitation duty discussed separately.

### F8 — Deletions do not propagate

`decoder.decode` only handles `operation == "create"`; Jetstream `delete`
operations and account-level events are ignored. A post deleted on Bluesky
stays in Postgres and Qdrant indefinitely.

## Suggested order

1. ~~**F1**~~ — done. It was the only finding actively causing harm, and it
   blocked the memory reclamation everything else depends on.
2. **F2** — verify the canary, then enable retention. Until then the interim
   trim is a manual chore roughly every 7 hours.
3. **F4** — one-line TTL change that buys room while F3 is addressed.
4. **F3** — the batch method swap first (smallest diff, already-existing
   helper), then upsert batching, then the O(n²) loop.
5. **F6, F7, F8, F5** — real but not yet load-bearing.

## Configuration in place

Development host `/opt/codohue-dev/.env` (backup at `.env.bak-before-bskyfeed`):

```dotenv
COMPOSE_PROFILES=local-db,local-redis,local-qdrant,bskyfeed
CODOHUE_BSKY_NAMESPACE=bluesky
CODOHUE_BSKY_BOOTSTRAP=true
CODOHUE_BSKY_EMBEDDING_DIM=256
CODOHUE_BSKY_SAMPLE_PERCENT=10
```

`CODOHUE_REDIS_MAXMEMORY` is unset, so `compose.prod.yaml` applies its 256mb
default with `noeviction`.
