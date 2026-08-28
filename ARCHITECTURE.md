# Codohue — Architecture

This document describes Codohue's current architecture: the processes, communication boundaries, storage layers, data flow, and important design decisions. The README only carries the overview + quickstart; everything architectural lives here.

## 1. Overview

Codohue is a **hybrid sparse + dense collaborative filtering** recommendation service for multi-tenant behavioral personalization. The system is organized around four small binaries that run independently, a four-module Go workspace, and three infrastructure components (PostgreSQL, Redis, Qdrant).

- **Sparse CF** is built from the event log (view, like, comment, share, skip, …) by a cron job and stored in Qdrant `{ns}_subjects` / `{ns}_objects` collections (sparse dot product).
- **Dense vectors** have exactly one producer per namespace, named by the `dense_source` enum: `disabled`, `item2vec`, `svd`, `byoe`, or `catalog` (exported as `codohuetypes.DenseSource*`; **`catalog` is the core, recommended mode** — the only one where both dense collections have an in-system owner). Stored in `{ns}_subjects_dense` / `{ns}_objects_dense` (cosine).
- **Hybrid blending** mixes sparse and dense scores at serve time, while applying time-decay and γ-freshness at rerank.
- **Multi-tenant** by `namespace`; each namespace owns its config, Qdrant collections, Redis keys, and API key.

```
┌──────────────┐  HTTP / Redis Streams         ┌──────────────────────┐
│ Main Backend │  (events + catalog)           │ cmd/api  (port 2001) │
│              │ ─────────────────────────────▶│  + ingest workers    │
└──────────────┘                               │    (events, catalog) │
       │                                       │  + events-tail pub   │
       │  POST /catalog (+/batch)              └──────────┬───────────┘
       │                                                  │ writes
       │                                                  ▼
       │                                          ┌────────────────┐
       │                                          │  PostgreSQL    │
       │                                          │  events,       │
       │                                          │  catalog_items │
       │                                          │  objects,      │
       │                                          │  configs, …    │
       │                                          └───┬─────────┬──┘
       │                                              │ reads   │
       │                                              ▼         ▼
       │                                       ┌─────────┐ ┌────────────┐
       │                                       │cmd/cron │ │cmd/embedder│
       │                                       │(batch + │ │(stream     │
       │                                       │retention│ │ consumer + │
       │                                       │)        │ │ sweepers)  │
       │                                       └────┬────┘ └─────┬──────┘
       │                                            │            │
       │                                            ▼            ▼
       │                                       ┌──────────────────────┐
       │                                       │  Qdrant collections   │
       │                                       │  sparse + dense       │
       │                                       └────────┬──────────────┘
       │                                                ▲
       │  GET recommendations / rankings / trending     │
       └────────────────────────────────────────────────┘

                    ┌──────────────────────┐
   Operator / SPA → │ cmd/admin (port 2002)│ → cmd/api (/healthz, inject)
                    │  session cookie auth │ → PostgreSQL, Redis, Qdrant
                    │  + SSE event bus     │ ← Redis pub/sub bridges
                    └──────────────────────┘
```

## 2. Processes (Binaries)

All four binaries are built from the same repo, each with a clearly scoped role. Inter-binary communication goes through PostgreSQL, Redis, and Qdrant — there is **no** direct RPC between binaries (except `cmd/admin` proxying `/healthz` and injecting test events into `cmd/api`).

| Binary          | Port | Role |
| --------------- | ---- | ---- |
| [cmd/api](cmd/api)           | 2001 | HTTP data-plane (events, catalog + batch + reconciliation read, recommendations, rankings, trending, object metadata, BYOE) plus three goroutines: the `ingest` worker consuming `codohue:events`, the catalog worker consuming `codohue:catalog` (durable catalog transport, fed into `internal/catalog` via a `cmd/api` adapter), and the events-tail publisher fanning ingested events onto `codohue:events-tail:{ns}` |
| [cmd/cron](cmd/cron)         | —    | Batch daemon driven by `CODOHUE_BATCH_INTERVAL_MINUTES` (default 5 min); each tick runs three phases per namespace. Also runs the retention prune and deleted-generation janitor, and publishes run lifecycle events to `codohue:batchrun-events` so `cmd/admin` can stream cron runs live |
| [cmd/admin](cmd/admin)       | 2002 | Admin server: session-cookie **or** bearer (`CODOHUE_ADMIN_API_KEY`) auth, `/api/admin/v1/*`, SSE streams over an in-process event bus fed by three Redis pub/sub bridges (batch runs, catalog signals, events tail). Embeds the `web/admin` SPA via the `embedui` build tag. The same binary provides the non-HTTP `lifecycle disable-legacy-envelopes` and `idmap-repair audit\|quarantine\|apply\|verify\|resume` operator commands |
| [cmd/embedder](cmd/embedder) | 2003 | Catalog worker: consumes `catalog:embed:{ns}` streams, embeds via `embedstrategy.Strategy`, upserts the dense vector. Also runs the re-embed completion watcher, the backlog sampler (writes `catalog_backlog_samples`), the recovery sweeper (re-publishes rows whose stream entry was lost), and the liveness heartbeat (`codohue:embedder:heartbeat`, TTL 90s) |

### 2.1 Process ↔ storage matrix

| Storage             | api | cron | admin | embedder |
| ------------------- | :-: | :--: | :---: | :------: |
| PostgreSQL          | RW  | RW (compute + retention prune) | RW | RW |
| Redis               | R (consume `codohue:events` + `codohue:catalog`), W (reco cache, `catalog:embed:{ns}`, `codohue:events-tail:{ns}`) | W (trending ZSET, `codohue:batchrun-events`) | R (trending, heartbeats), SUB (three bridges) | RW (consume `catalog:embed:{ns}`, heartbeat, `codohue:catalog-events:{ns}`) |
| Qdrant              | R (search), W (BYOE upsert, object delete) | RW (sparse + dense upsert; reads back object vectors under `catalog`) | R (counts) | W (`{ns}_objects_dense` upsert) |

## 3. Go Workspace

The repo is a Go workspace ([go.work](go.work)) with **four modules**. `make lint`/`test`/`coverage` iterate over each module.

| Module path                                          | Role |
| ---------------------------------------------------- | ---- |
| `github.com/jarviisha/codohue` (`.`)                 | Server application — four binaries, every `internal/` domain, the e2e suite |
| `github.com/jarviisha/codohue/pkg/codohuetypes`      | Shared wire types so the SDK doesn't pull pgx/qdrant/prometheus deps |
| `github.com/jarviisha/codohue/sdk/go`                | Public Go SDK for clients embedding Codohue; includes the `sdk/go/admin` package (bearer-authenticated admin client, `ProvisionCatalogNamespace`) |
| `github.com/jarviisha/codohue/sdk/go/redistream`     | Redis Streams transport helper for the SDK |

The server module targets Go `1.26.1`. The SDK modules (`pkg/codohuetypes`, `sdk/go`, `sdk/go/redistream`) deliberately stay on Go `1.24.13` for broader downstream adoption.

## 4. Internal layering

Each feature domain lives at `internal/<domain>/` with a consistent file set: `handler.go`, `service.go`, `repository.go`, `types.go`, plus a mandatory `docs.go` as the single canonical place for the package doc.

| Package                              | Responsibility |
| ------------------------------------ | -------------- |
| [internal/ingest](internal/ingest)             | Accepts events via HTTP and Redis Streams, validates, persists to `events`; publishes the tail feed. Also hosts the `CatalogWorker` consuming `codohue:catalog` (hands items to `internal/catalog` through a `cmd/api` adapter) |
| [internal/compute](internal/compute)           | Batch: sparse + dense recompute, trending |
| [internal/recommend](internal/recommend)       | CF, hybrid dense/sparse, rank, trending, BYOE embeddings, object delete |
| [internal/nsconfig](internal/nsconfig)         | CRUD for per-namespace config (weights, decay, dense hybrid, catalog) |
| [internal/objects](internal/objects)           | Per-object metadata independent of embedding — currently `author_subject_id`; the `objects` table |
| [internal/admin](internal/admin)               | Handlers/services/repos for `cmd/admin`, including SSE handlers |
| [internal/catalog](internal/catalog)           | Data-plane HTTP content ingest; persists `catalog_items`, publishes `catalog:embed:{ns}` |
| [internal/embedder](internal/embedder)         | Per-item pipeline (load → embed → upsert → mark embedded), re-embed watcher, backlog sampler, recovery sweeper, heartbeat |
| [internal/retention](internal/retention)       | Periodic prune of `batch_run_logs` + `catalog_backlog_samples`; runs inside `cmd/cron` |
| [internal/auth](internal/auth)                 | Bearer-token validation: admin key + per-namespace bcrypt key, with a negative cache |
| [internal/config](internal/config)             | Env-var loader |
| [internal/core/embedstrategy](internal/core/embedstrategy) | Forward-compat seam: `Strategy` interface + registry (both `catalog` and `embedder` depend on the seam, never on each other) |
| [internal/core/namespace](internal/core/namespace)         | Shared `namespace.Config` contract |
| [internal/core/idmap](internal/core/idmap)                 | String ID → BIGSERIAL numeric ID through `id_mappings` |
| [internal/core/httpapi](internal/core/httpapi)             | JSON response helpers, `DecodeStrict`, middleware |
| [internal/core/batchrun](internal/core/batchrun)           | Shared batch-run logging types |
| [internal/admin/{eventbus,sse,metricsroll}](internal/admin) | Admin-only subpackages: fan-out bus, SSE writer, rolling metrics windows |
| [internal/architecture](internal/architecture)             | Repo-wide import-rule enforcement test |
| [internal/infra/{postgres,redis,qdrant,metrics}](internal/infra) | pgxpool, go-redis, Qdrant gRPC, Prometheus collectors |

### 4.1 Import rule (hard)

Enforced by [internal/architecture/imports_test.go](internal/architecture/imports_test.go):

- Packages under `internal/` may import only `internal/config`, `internal/core/...`, `internal/infra/...`, and their own subpackages (e.g. `internal/admin` may import `internal/admin/sse`).
- Peer-domain imports are **forbidden** (for example, `recommend` may not import `ingest`).
- Cross-domain coordination happens at the wiring layer in `cmd/api` and `cmd/admin` (see [cmd/admin/nsconfig_adapter.go](cmd/admin/nsconfig_adapter.go)). `internal/catalog` accepting an author and writing it to `objects` works the same way, as does the catalog stream: `internal/ingest`'s `CatalogWorker` calls a narrow interface that [cmd/api/catalog_stream_adapter.go](cmd/api/catalog_stream_adapter.go) implements around `catalog.Service` — ingest and catalog never import each other.

This shape lets any domain be split into a separate microservice later without untangling coupling.

## 5. Data model

### 5.1 PostgreSQL

Migrations live under [migrations/](migrations/) as `NNN_name.up.sql` / `NNN_name.down.sql`.

| Table                      | Role |
| -------------------------- | ---- |
| `namespace_configs`        | Per-namespace config: `action_weights`, `lambda`, `gamma`, `max_results`, `seen_items_days`, `alpha`, `dense_source`, `embedding_dim`, `dense_distance`, `trending_*`, `exclude_authored`, `api_key_hash`, `catalog_strategy_id`, `catalog_strategy_version`, `catalog_strategy_params`, nullable `catalog_max_attempts` / `catalog_max_content_bytes` |
| `events`                   | Behavioral events: `namespace`, `subject_id`, `object_id`, `action`, `occurred_at`, `object_created_at`. Indexed on `(namespace, subject_id)`, `occurred_at`, and `(namespace, subject_id, occurred_at DESC)` |
| `id_mappings`              | String ID → BIGSERIAL numeric, **primary key `(namespace, entity_type, string_id)`**. Used as the Qdrant point ID to avoid hash collisions |
| `objects`                  | Per-object metadata: `namespace`, `object_id`, `author_subject_id`. Independent of `dense_source` |
| `catalog_items`            | Raw content per object: state machine `pending → embedding → embedded` (plus `failed` / `dead_letter`); `content`, `metadata`, strategy version |
| `batch_run_logs`           | History of every cron tick / admin re-embed: `trigger_source ∈ {cron, manual, admin_reembed}`, phase{1,2,3} ok/duration/entities/objects/error, `log_lines` JSONB, `cancel_requested`, `target_strategy_*` |
| `catalog_backlog_samples`  | Backlog time-series, one row per namespace per 30s sampler tick (skip-on-unchanged) |

Schema evolution after `001_initial`:

- **002** `gamma` (object freshness rerank)
- **003** `seen_items_days` (recency filter window, default 30)
- **004** `events.object_created_at`
- **005** `api_key_hash`, `alpha`, `dense_strategy`, `embedding_dim`, `trending_*`
- **006** `batch_run_logs` table
- **007** phase breakdown columns
- **008** `trigger_source`
- **009** `log_lines` JSONB
- **010** `catalog_items` table
- **011** catalog columns on `namespace_configs`
- **012** pre-prod hardening: CHECK on `trigger_source`; `target_strategy_id` / `target_strategy_version`; rename `subjects_processed` → `entities_processed`
- **013** `cancel_requested` + partial index for operator cancel between phases
- **014** `catalog_backlog_samples` table
- **015** indexes on `batch_run_logs.started_at` + `catalog_backlog_samples.sampled_at` for the retention prune
- **016** `dense_source` enum, backfilled from `catalog_enabled` / `dense_strategy` + CHECK; legacy columns kept for the dual-write window
- **017** drops `dense_strategy` + `catalog_enabled`; `dense_source` is the single source of truth
- **018** `idx_events_ns_subject_occurred` so the admin subject browser aggregates via index-only scan
- **019** `author_subject_id` on `catalog_items` + partial index
- **020** `exclude_authored` on `namespace_configs` (default FALSE)
- **021** `objects` table; **moves** `author_subject_id` off `catalog_items` and drops that column, so attribution works under every `dense_source`
- **022** re-keys `id_mappings` on `(namespace, entity_type, string_id)`; requires a full recompute per namespace after deploy
- **023** makes `catalog_max_attempts` / `catalog_max_content_bytes` nullable so NULL means "use the env default"
- **024** adds `namespace_lifecycles` and `system_lifecycle`: durable delete/reset states, monotonic namespace generations, and the legacy-envelope gate
- **025** adds validated namespace foreign keys and the catalog keyset index backing `next_cursor`
- **026** scopes numeric ID uniqueness to `(namespace, entity_type)` and adds the durable ID-mapping repair run/item manifests
- **027** records rebuilt namespaces on ID-mapping repair runs so verification uses durable evidence

### 5.2 Redis

| Key                                | Kind        | Producer        | Consumer / TTL |
| ---------------------------------- | ----------- | --------------- | -------------- |
| `codohue:events`                   | Stream      | Main Backend    | `cmd/api` ingest worker (consumer group, `CODOHUE_INGEST_REPLICA_NAME`) |
| `codohue:catalog`                  | Stream      | Main Backend (SDK `redistream.CatalogProducer`) | `cmd/api` catalog worker (consumer group, same replica name). **Not producer-trimmed** — entries persist until consumed and acked, so content published during a Codohue outage is ingested on recovery |
| `catalog:embed:{ns}`               | Stream      | `internal/catalog` (publishes on POST catalog) | `cmd/embedder` (consumer group, `CODOHUE_EMBEDDER_REPLICA_NAME`) |
| `trending:{ns}`                    | Sorted set  | `cmd/cron` phase 3 | `recommend` service; TTL = `trending_ttl` |
| `rec:{ns}:{subject}:limit=N:offset=M` | String   | `recommend`     | `recommend`; TTL 5 minutes |
| `codohue:events-tail:{ns}`         | Pub/Sub     | `cmd/api` tail publisher | `cmd/admin` events-tail bridge → SSE |
| `codohue:batchrun-events`          | Pub/Sub     | `cmd/cron` observer | `cmd/admin` batch-run bridge → SSE |
| `codohue:catalog-events:{ns}`      | Pub/Sub     | `cmd/embedder`  | `cmd/admin` catalog bridge → SSE |
| `codohue:embedder:heartbeat`       | String      | `cmd/embedder`  | `cmd/admin` overview; TTL 90s |

The cron liveness signal has no Redis key — the admin overview derives it from the most recent row in `batch_run_logs`.

The table shows generation-1 names. Delete/recreate increments the namespace
generation; generation 2+ uses `nslifecycle`-qualified Redis and Qdrant names
(for example `trending:demo:g2` and `demo_g2_objects_dense`). Stream envelopes
carry `namespace_generation`; stale-generation work is acknowledged and dropped.

### 5.3 Qdrant

Each namespace generation has **four collections**:

| Collection              | Vector kind | Distance     | Writer       | Purpose |
| ----------------------- | ----------- | ------------ | ------------ | ------- |
| `{ns}_subjects`         | Sparse      | Dot          | `cmd/cron`   | Sparse CF subject vectors |
| `{ns}_objects`          | Sparse      | Dot          | `cmd/cron`   | Sparse CF object vectors  |
| `{ns}_subjects_dense`   | Dense       | Cosine       | `cmd/cron` (mean-pool) or `cmd/api` (BYOE PUT) | Dense subject vector |
| `{ns}_objects_dense`    | Dense       | Cosine       | `cmd/cron` (`item2vec`/`svd`), `cmd/api` (`byoe`), or `cmd/embedder` (`catalog`) | Dense object vector |

Point IDs are `int64` values from `id_mappings`. The dimension of each dense collection is taken from `namespace_configs.embedding_dim`.

## 6. Batch job (`cmd/cron`)

Each tick iterates over every namespace and runs three sequential phases; each phase can be skipped independently and is logged separately into `batch_run_logs`.

| Phase | Name      | Description |
| ----- | --------- | ----------- |
| 1     | Sparse    | Reads `events` from the last 90 days, applies `action_weights × e^(-λ × days_since)`, builds subject/object sparse vectors, upserts into `{ns}_subjects` / `{ns}_objects` |
| 2     | Dense     | Derives subject vectors by mean-pooling the dense vectors of each subject's interacted items and upserts `{ns}_subjects_dense`. Runs when `dense_source ∈ {item2vec, svd, catalog}`; **skipped** for `byoe` and `disabled` |
| 3     | Trending  | Computes time-decayed trending from recent events into a Redis ZSET. **Skipped** when Redis is unavailable |

Phase 2's object vectors come from different places depending on `dense_source`: `item2vec` / `svd` train them in this phase and also upsert `{ns}_objects_dense`, while `catalog` reads back the vectors `cmd/embedder` already wrote and writes **only** subject vectors — loading just the objects that appear in events, since only those can contribute to a mean.

Phase 1 failure aborts the run. Phase 2 failure is logged but lets phase 3 proceed (dense is an optional surface); phase 3 failure folds into the run status, so an all-green run list means every phase that ran actually succeeded. Between phases the job polls `batch_run_logs.cancel_requested` and stops cleanly when an operator asks; mid-phase cancel is deliberately not supported.

Alongside the compute job, `cmd/cron` runs the retention prune (`internal/retention`) on `CODOHUE_RETENTION_INTERVAL`, bounding `batch_run_logs` and `catalog_backlog_samples`.

### 6.1 Why full recompute?

Every cron tick rebuilds every vector from scratch. Reasons:
- Avoids race conditions inherent to get→merge→upsert flows.
- Item2Vec lacks a stable incremental online variant; incremental Word2Vec causes **catastrophic forgetting**. Full retraining keeps embedding quality consistent.

Trade-off accepted: the batch runs at `CODOHUE_BATCH_INTERVAL_MINUTES` (default 5 min), so sparse/dense freshness is bounded by the tick interval.

## 7. Catalog auto-embedding (`cmd/embedder`)

Lets callers submit raw content instead of computing embeddings themselves. The worker turns content into a dense vector and upserts `{ns}_objects_dense`. It is enabled by selecting `dense_source = "catalog"` — there is no separate boolean, so a second producer writing the same collection is structurally impossible.

### 7.1 Pipeline

```
POST /v1/namespaces/{ns}/catalog (+/batch)      XADD codohue:catalog
        │   (internal/catalog)                        │ (cmd/api catalog worker
        │                                             │  → adapter → internal/catalog)
        ▼                                             ▼
   catalog_items.insert(state=pending)
        │   XADD catalog:embed:{ns}
        ▼
┌────────────────────────────────────────────┐
│ cmd/embedder consumer (per replica)        │
│   load item                                │
│   state → embedding                        │
│   embedstrategy.Strategy.Embed(content)    │
│   qdrant.Upsert({ns}_objects_dense)        │
│   state → embedded                         │
└────────────────────────────────────────────┘
```

An optional `author_subject_id` on the ingest body is **not** stored on `catalog_items`; it is written through to the `objects` table via an interface injected in `cmd/api`, so `internal/catalog` never imports the peer domain. Omitting it on a re-ingest means "unspecified" and leaves existing attribution alone.

### 7.2 Retry, dead-letter, recovery

- Transient errors retry up to `catalog_max_attempts` (namespace override, else `CODOHUE_EMBED_MAX_ATTEMPTS`, default 5) before moving to dead-letter.
- Admin can redrive a single item (`POST /catalog/items/{id}/redrive`) or in bulk (`POST /catalog/items/redrive-deadletter`).
- The **recovery sweeper** re-publishes rows whose stream entry was lost, covering the gap between a Postgres write and a Redis failure.
- The **backlog sampler** records one `catalog_backlog_samples` row per namespace per 30s tick (skipping unchanged values) and feeds the backlog SSE snapshot.
- An unwired strategy registry in the running build causes catalog endpoints to return **503** (forward-compat seam).

### 7.3 BYOE ↔ catalog interaction

- When `dense_source = "catalog"`, `PUT /v1/namespaces/{ns}/objects/{id}/embedding` returns **409 Conflict** — the catalog pipeline is the source of truth for object vectors.
- `PUT /v1/namespaces/{ns}/subjects/{id}/embedding` is **not** guarded. Subject vectors have no single owner: `cmd/cron` phase 2 mean-pools them every tick, and this endpoint lets a client overwrite one between ticks to cut staleness.

### 7.4 Re-embed batch

`POST /api/admin/v1/namespaces/{ns}/catalog/re-embed` opens a fresh `batch_run_logs` row with `trigger_source = admin_reembed` and attached `target_strategy_id` + `target_strategy_version`, then re-publishes catalog items. With no body it re-drives only rows at a different `strategy_version`; an explicit `{"only_state":"all|embedded|failed"}` re-drives those rows regardless of version (the "rebuild after Qdrant loss" path). The re-embed completion watcher inside `cmd/embedder` closes the run when the namespace's backlog drains to 0. Returns **409** if another run is already in progress.

## 8. Recommendation pipeline

### 8.1 Subject state → source

| Subject interactions  | Source                                 |
| --------------------- | -------------------------------------- |
| 0                     | Redis trending ZSET (falls back to DB popular when Redis is empty) |
| 1 ≤ N < 5             | Hybrid cold: **70%** trending + **30%** CF |
| N ≥ 5                 | CF (sparse) or hybrid dense+sparse blend when enabled |

### 8.2 Hybrid blend

Active when `0 < alpha < 1.0` and `dense_source ∉ {"", "disabled"}`:

```
score_final = alpha · saturate(score_sparse) + (1 - alpha) · bound(score_dense)
```

Normalization is **batch-independent** (one shared helper, `blendHybridScores`, used by both recommendations and rankings): sparse dot products go through the fixed saturating curve `x/(x+k)` (`k`=5, a single global constant — a per-namespace or per-request value would reintroduce cross-request incomparability), and dense scores are bounded to [0, 1] (cosine clamped; dot-distance namespaces saturate like sparse). This replaced per-request min-max, whose scores anchored to whatever else was in the batch. Sparse search runs against `{ns}_objects`, dense against `{ns}_objects_dense`; recommendations over-fetch 5×/3× the page.

`POST /rankings` takes the **same blend** over the caller's candidate set (a `HasID` filter instead of top-K search, no paging): namespace `alpha` decides the balance, a missing side degrades to the surviving side at full weight (dense-only replaces the old whole-list zero fallback), and the same eligibility exclusions apply (§8.4). Every candidate comes back with a `scored` boolean — `false` means "returned unscored" (never indexed, or excluded), not "irrelevant" — and a subject with no vector at all gets the whole list back unscored with `source: "no_subject_vector"`. Because normalization is batch-independent, scores are comparable across calls: chunked rankings requests merge into the same ordering as one call over the union. Background: [specs/006-darkvoid-alignment/design.md](specs/006-darkvoid-alignment/design.md); cap measurement: [specs/006-darkvoid-alignment/benchmarks.md](specs/006-darkvoid-alignment/benchmarks.md).

### 8.3 Time decay & freshness rerank

- **Build time** (cron): each event is multiplied by `e^(-λ × days_since)` (λ = `namespace_configs.lambda`). Events older than 90 days are dropped.
- **Rerank time** (recommend): the final score is multiplied by the object's γ-based freshness (`gamma`, default 0.02/day) — favors newer objects when scores are close.

### 8.4 Exclusion filters

Two exclusions are merged into the same Qdrant `MustNot`, so one filter covers the sparse and dense collections alike:

- **Seen items** — objects the subject interacted with in the last `seen_items_days` (default 30). Read directly from `events`, no cache.
- **Authored objects** (`exclude_authored`, default off) — objects whose `objects.author_subject_id` is the requesting subject. Materialised as point IDs rather than a payload filter, because `cmd/cron` writes the sparse points and knows nothing about authorship; a payload filter would silently reach only the dense collection. Capped at 5000 ids with a warning log; a query failure degrades to unfiltered results rather than failing the request.

The trending and popular fallbacks cannot push the filter into the store, so they over-fetch by the exclusion size and drop authored ids **before** paging.

`POST /rankings` applies the **same** exclusion set unconditionally (same `excludedObjectIDs` path, merged as `MustNot` onto its candidate filter), so one code path defines "eligible object" for both read surfaces; excluded candidates return `scored: false` rather than being dropped. Exclusion-lookup failures degrade to unfiltered scoring on both surfaces.

### 8.5 Cache

Responses are cached in Redis for 5 minutes per `(namespace, subject_id, limit, offset)`. BYOE PUT / object delete do **not** invalidate the cache — changes appear after the TTL expires or after the next cron tick.

### 8.6 Returned `source` field

- `collaborative_filtering`
- `hybrid` — sparse + dense blend
- `hybrid_cold` — trending + CF blend (cold start)
- `hybrid_rank` — `/rankings` endpoint, subject was scored
- `no_subject_vector` — `/rankings` whole-response fallback: the subject has neither a sparse nor a dense vector; every item is `scored: false` in request order
- `fallback_popular`

## 9. Authentication

A **two-tier** model.

| Plane            | Auth                                                                              | Token storage |
| ---------------- | --------------------------------------------------------------------------------- | ------------- |
| Admin (`cmd/admin`) | Session cookie `codohue_admin_session` (login = `POST /api/v1/auth/sessions` with `CODOHUE_ADMIN_API_KEY`) **or** `Authorization: Bearer <CODOHUE_ADMIN_API_KEY>` directly on `/api/admin/v1/*` — the automation path (`sdk/go/admin`), no cookie dance. A bearer header, when present, is authoritative and the cookie is ignored | Sessions: HMAC-signed JWT carrying a random `jti`; logout revokes the `jti` server-side until its natural expiry. Bearer: the static key itself |
| Data (`cmd/api`) | `Authorization: Bearer <namespace-key>` — bcrypt-hashed in `namespace_configs.api_key_hash` | Plaintext returned **once**, on creation or rotation |

`CODOHUE_ADMIN_API_KEY` is accepted for **every** namespace on the data plane, via a DB-free constant-time compare checked before the hash lookup. The admin server needs to reach all namespaces through it, and it already grants full control via the admin-plane login — restricting its data-plane reach bought little while breaking the admin panel. Namespace **configuration mutation** still lives only on the admin plane.

Hardening in the request path:

- All plain-string credential compares are constant-time.
- The public admin login endpoint **and** the admin bearer path are per-IP rate-limited on **failed** attempts only; a correct key is never throttled. An empty configured admin key disables the bearer path entirely rather than matching empty tokens.
- Repeated bad data-plane tokens hit a 30s negative cache keyed on `(token, namespace)` — only definitive rejections are cached, never infra blips — so a brute-force loop does not cost a bcrypt compare per attempt.
- The session signing secret comes from `CODOHUE_ADMIN_SESSION_SECRET`, or fresh random material each boot (a restart then logs everyone out).
- A namespace key is rotated via `POST /api/admin/v1/namespaces/{ns}/api-key`; the old key stops working immediately.

## 10. HTTP API

### 10.1 Data plane — `cmd/api` (port 2001)

Every business capability has **exactly one canonical path**. Legacy duplicate paths have been removed → 404.

**Infra/ops**

| Method | Path                    | Authentication       | Description |
| ------ | ----------------------- | -------------------- | ----------- |
| GET    | `/ping`                 | none                 | Liveness |
| GET    | `/healthz`              | none                 | Sanitized aggregate health without raw dependency errors |
| GET    | `/healthz?details=true` | observability bearer | Per-component diagnostics; route absent when `CODOHUE_OBSERVABILITY_TOKEN` is unset |
| GET    | `/metrics`              | observability bearer | Prometheus metrics; route absent when `CODOHUE_OBSERVABILITY_TOKEN` is unset |

**Namespace-scoped (Bearer)**

| Method | Path                                                 | Description |
| ------ | ---------------------------------------------------- | ----------- |
| POST   | `/v1/namespaces/{ns}/events`                         | Ingest event (202 + `{"event_id":N}`; `namespace` in body is ignored). Also fans onto `codohue:events-tail:{ns}` |
| POST   | `/v1/namespaces/{ns}/catalog`                        | Ingest raw content (202; only when `dense_source="catalog"`). Optional `author_subject_id` is written through to `objects` |
| POST   | `/v1/namespaces/{ns}/catalog/batch`                  | Batch ingest, ≤100 items, validated independently with per-item results (202) |
| GET    | `/v1/namespaces/{ns}/catalog/objects`                | Reconciliation read (`?changed_since=&limit=&cursor=`): opaque keyset cursor over `(updated_at,id)`; legacy offset remains for one compatibility window and cannot be combined with a cursor |
| GET    | `/v1/namespaces/{ns}/subjects/{id}/recommendations`  | CF recommendations (`?limit=&offset=`) |
| POST   | `/v1/namespaces/{ns}/rankings`                       | Score + rank up to 500 candidates with the hybrid blend + shared exclusions (200); per-item `scored` flag, `no_subject_vector` whole-response fallback, chunk-comparable scores (§8.2) |
| GET    | `/v1/namespaces/{ns}/trending`                       | Trending (`?limit=&offset=`); `window_hours` in the response reports the namespace's configured window |
| PUT    | `/v1/namespaces/{ns}/objects/{id}`                   | Per-object metadata — currently `author_subject_id` (idempotent 200; accepted under every `dense_source`; empty value clears) |
| PUT    | `/v1/namespaces/{ns}/objects/{id}/embedding`         | BYOE object vector (204; **409** when `dense_source="catalog"`) |
| PUT    | `/v1/namespaces/{ns}/subjects/{id}/embedding`        | BYOE subject vector (204; not catalog-guarded) |
| DELETE | `/v1/namespaces/{ns}/objects/{id}`                   | Remove from every Qdrant collection **and** drop the `objects` row (idempotent 204) |

### 10.2 Admin plane — `cmd/admin` (port 2002, session cookie or bearer)

Sessions are modeled as a resource: login = create, logout = delete current. The API is shaped for a monitoring UI rather than plain REST CRUD: **aggregate** endpoints (one payload per view), **SSE** streams (`text/event-stream`, `event: <kind>` frames, `event: ping` heartbeat, `X-Accel-Buffering: no`), and **lifecycle** endpoints for batch runs. SSE rows are marked **(SSE)**.

| Method | Path                                                              | Description |
| ------ | ----------------------------------------------------------------- | ----------- |
| POST   | `/api/v1/auth/sessions`                                           | Validate admin key, set cookie (201 + `expires_at`) |
| DELETE | `/api/v1/auth/sessions/current`                                   | Clear cookie (204) |
| GET    | `/api/admin/v1/health`                                            | Proxy `/healthz` from `cmd/api` |
| GET    | `/api/admin/v1/ping/stream`                                       | **(SSE)** Smoke-test stream for the SSE pipeline; not a production endpoint |
| GET    | `/api/admin/v1/overview`                                          | Fleet aggregate: health + cron/embedder heartbeat + alerts + per-namespace summary |
| GET    | `/api/admin/v1/metrics/summary`                                   | Curated rolling-window metrics: ingest events/sec (1m/5m) per ns + cron batch lag |
| GET    | `/api/admin/v1/stream`                                            | **(SSE)** Global ops bus: `batch_run.*`, `catalog.dead_letter_grew`, `catalog.reembed_progress` |
| GET    | `/api/admin/v1/namespaces`                                        | List configs |
| GET    | `/api/admin/v1/namespaces/{ns}`                                   | Get config |
| PUT    | `/api/admin/v1/namespaces/{ns}`                                   | Create/update (200/201). **PATCH semantics** — an omitted field leaves that column untouched. `dense_source="catalog"` is accepted when `catalog_strategy_id`/`_version` accompany it (same dim validation as the catalog endpoint — one-request core-mode provisioning); without them → 422 naming the missing fields |
| DELETE | `/api/admin/v1/namespaces/{ns}`                                   | Wipe namespace + all its data (200 summary; 404 when missing) |
| POST   | `/api/admin/v1/namespaces/{ns}/api-key`                           | Rotate the namespace data-plane key (plaintext returned once) |
| GET    | `/api/admin/v1/namespaces/{ns}/dashboard`                         | Per-namespace aggregate: config + last 12 runs + backlog + events + qdrant counts + trending TTL + author coverage |
| POST   | `/api/admin/v1/reset`                                             | App-wide reset; body `{"confirm":"RESET"}` |
| GET    | `/api/admin/v1/catalog/strategies`                                | Namespace-free embed strategy registry; `?dim=` filters by embedding dimension |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog`                           | Catalog config + strategies + backlog + lag + failures + throughput |
| PUT    | `/api/admin/v1/namespaces/{ns}/catalog`                           | Enable/update/disable catalog (400 on dim mismatch; 503 unwired) |
| POST   | `/api/admin/v1/namespaces/{ns}/catalog/re-embed`                  | Trigger re-embed (202 + `Location`; 409 if one is running) |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog/backlog-history`           | Backlog time-series (`?window=&bucket=`) |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog/failures-summary`          | Top `last_error` reasons in window (`?window=`) |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog/stream`                    | **(SSE)** `item_state_changed`, `backlog_snapshot`, `dead_letter_grew`, `reembed_progress` |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog/items`                     | Browse (`?state=&limit=&offset=&object_id=&author=`) |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}`                | Full item (with `content`, `metadata`) |
| POST   | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive`        | Redrive a single item |
| POST   | `/api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter`  | Bulk redrive dead-letter |
| DELETE | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}`                | Hard-delete (Postgres + Qdrant point) |
| GET    | `/api/admin/v1/batch-runs`                                        | Recent runs as `BatchRunSummary` (`?namespace=&status=&kind=&limit=&offset=`) |
| GET    | `/api/admin/v1/batch-runs/stats`                                  | Fleet-wide time-series (`?window=&bucket=`) |
| GET    | `/api/admin/v1/batch-runs/{id}`                                   | Full `BatchRunDetail` (phases + `log_lines` + target strategy) |
| GET    | `/api/admin/v1/batch-runs/{id}/stream`                            | **(SSE)** `phase_started/completed`, `log_line`, `run_completed`, `cancelled`; 204 when already terminal. Covers cron runs via the pub/sub bridge |
| POST   | `/api/admin/v1/batch-runs/{id}/cancel`                            | Request cancel between phases (200; 409 when terminal) |
| POST   | `/api/admin/v1/batch-runs/{id}/retry`                             | Re-run with the same namespace/kind/target (202 + `Location`) |
| GET    | `/api/admin/v1/namespaces/{ns}/batch-runs`                        | Batch runs scoped to a namespace |
| POST   | `/api/admin/v1/namespaces/{ns}/batch-runs`                        | Create a new batch run (202 + `Location`) |
| GET    | `/api/admin/v1/namespaces/{ns}/qdrant`                            | Point counts across the four collections |
| GET    | `/api/admin/v1/namespaces/{ns}/subjects`                          | Browse subjects aggregated from `events` (`?q=&sort=&limit=&offset=`) |
| GET    | `/api/admin/v1/namespaces/{ns}/subjects/{id}/profile`             | Interaction count, seen items, sparse NNZ |
| GET    | `/api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations`     | Recommendations with `?debug=` |
| GET    | `/api/admin/v1/namespaces/{ns}/trending`                          | Trending + Redis TTL |
| GET    | `/api/admin/v1/namespaces/{ns}/events`                            | Recent events (`?limit=&offset=&subject_id=`) |
| GET    | `/api/admin/v1/namespaces/{ns}/events/stream`                     | **(SSE)** Live tail (`?action=&subject_id=`): `event`, `dropped` on backpressure |
| GET    | `/api/admin/v1/namespaces/{ns}/events/summary`                    | Server-side aggregation (`?window=1m\|5m\|1h`) |
| POST   | `/api/admin/v1/namespaces/{ns}/events`                            | Inject a test event (proxied to `cmd/api`) |
| POST   | `/api/admin/v1/demo-data`                                         | Seed demo dataset (202) |
| DELETE | `/api/admin/v1/demo-data`                                         | Clear demo dataset (204) |

### 10.3 Error envelope

```json
{
  "error": {
    "code": "invalid_request",
    "message": "invalid request body"
  }
}
```

Lifecycle and validation failures use stable codes across data-plane handlers:

| Status | Code | Meaning |
| ------ | ---- | ------- |
| 404 | `namespace_not_found` | The namespace does not exist |
| 409 | `namespace_not_active` | Delete/reset lifecycle work blocks the namespace |
| 503 | `namespace_config_unavailable` | Lifecycle or configuration storage could not be read safely |
| 400 | `invalid_object_created_at` | The supplied creation time is more than five minutes in the future |

### 10.4 Strict request decoding

Client-facing mutation endpoints (`events`, `rankings`, `catalog`, `objects`, embedding) decode their JSON body with `httpapi.DecodeStrict`: unknown fields and trailing data are rejected with `400 invalid_request` rather than silently dropped. A body that carries a field not declared on the wire type (including a redundant `namespace` on `rankings`/`catalog`, where the path is authoritative) is refused. `events` still accepts a body `namespace` because `EventPayload` declares it (the Redis-stream transport needs it); it is overwritten by the path on the HTTP route. The Redis-stream ingest path and the admin plane keep lenient decoding.

### 10.5 Wire contract

The client-facing JSON types live once in [pkg/codohuetypes](pkg/codohuetypes) and are re-exported into the server domains via type aliases, so the server marshals the exact struct the SDK unmarshals. The marshaled shape of every wire type is pinned by golden snapshots in `pkg/codohuetypes/testdata/`; after a deliberate contract change, regenerate and commit them:

```bash
go test ./pkg/codohuetypes/... -run Golden -update
```

## 11. Event ingestion

Two transports, same worker, same `events` table.

### 11.1 HTTP

```
POST /v1/namespaces/{ns}/events
Authorization: Bearer <namespace-key>
Content-Type: application/json
```

The `namespace` field in the body is ignored (the URL wins). Use RFC3339 for `occurred_at`.

### 11.2 Redis Streams

Publish to `codohue:events`; each message must carry a `payload` field with a JSON document including the namespace (the stream has no URL):

```json
{
  "namespace": "demo",
  "subject_id": "user-123",
  "object_id": "item-456",
  "action": "VIEW",
  "timestamp": "2026-04-19T10:00:00Z",
  "object_created_at": "2026-04-18T08:00:00Z"
}
```

### 11.3 Actions

Built-in: `VIEW`, `LIKE`, `COMMENT`, `SHARE`, `SKIP` (with default weights). Custom actions are accepted when `namespace_configs.action_weights` has a matching entry; otherwise ingest returns an `unknown action` error. Weights may be negative (a skip is a negative signal); only NaN/Inf are rejected.

## 12. Observability

- **Prometheus** — collectors in `internal/infra/metrics`, exposed at `GET /metrics` from both `cmd/api` (2001) and `cmd/embedder` (2003).
- **Batch run history** — `batch_run_logs` records every cron tick and admin re-embed; the `log_lines` JSONB column captures the run's slog output, surfaced through the admin API and streamed live over SSE.
- **Liveness** — `cmd/embedder` writes `codohue:embedder:heartbeat` (TTL 90s); cron liveness is derived from the most recent `batch_run_logs` row. Both feed the admin overview's alert rules.
- **Dense-downgrade alert** — the overview flags any namespace configured for hybrid (`alpha < 1`, dense on) whose `{ns}_subjects_dense` is empty: the config says hybrid while requests silently serve sparse-only (the standing state of `byoe` namespaces that never push subject vectors). The serving path logs the per-request warning.
- **Catalog stream rejects** — stream-delivered catalog items that are permanently rejected before any `catalog_items` row exists are counted in `codohue_catalog_stream_rejects_total` (by namespace + reason) and warned in the log; rejections that do reach a row surface through the item's failure state instead.
- **Rolling metrics** — `internal/admin/metricsroll` maintains in-process 1m/5m windows behind `/api/admin/v1/metrics/summary`.
- **Backlog timeline** — `catalog_backlog_samples`, written by the embedder's sampler, backs `/catalog/backlog-history`.
- **slog format** — `CODOHUE_LOG_FORMAT=text` (default) or `json` (the prod compose defaults to `json`).
- **Healthcheck** — unauthenticated `GET /healthz` exposes sanitized aggregate status. A valid observability bearer can request `/healthz?details=true` for component diagnostics; admin proxies the sanitized endpoint at `/api/admin/v1/health`.
- **Retention** — `internal/retention` prunes `batch_run_logs` and `catalog_backlog_samples`; setting either `*_RETENTION_DAYS` to 0 disables that prune.
- **Stream retention** — producers never trim streams. A periodic exact `XTRIM MINID` pass derives the safe frontier from every consumer group and never trims pending work.

## 13. Key design decisions

| Decision | Reason |
| -------- | ------ |
| Full recompute every cron tick | Avoids race conditions in get→merge→upsert; item2vec retraining avoids catastrophic forgetting |
| ID mapping via DB (BIGSERIAL), keyed `(namespace, entity_type, string_id)` | Avoids hash collisions for Qdrant point IDs; the composite key stops two namespaces (or a subject and an object) from sharing one row |
| Sparse + dense as separate collections | Different distance/algorithm (Dot vs Cosine); search runs independently before blending |
| Batch-independent normalization (`x/(x+k)` sparse, bounded dense) | Per-request min-max anchored scores to the batch, so chunked rankings calls could not be merged; a fixed map keeps scores comparable across requests. `k` is one global constant on purpose |
| Rankings share Recommend's blend and eligibility | One helper, one exclusion path — the same namespace config cannot mean two different things depending on which endpoint is asked |
| Every rankings candidate returns, with a `scored` flag | "No vector", "not indexed" and "zero overlap" were indistinguishable `score: 0`; the flag + `no_subject_vector` source let callers compute coverage and skip unknown subjects |
| Streams are never producer-trimmed | Producers cannot know the slowest consumer-group frontier. Periodic exact retention trims only completed history below every group frontier, preserving pending work |
| Admin bearer auth reuses the admin key, failed-only rate limit | The key already grants full control via login, so bearer widens no privilege — it removes the cookie handshake automation had to fake. Acceptable while there is one internal consumer |
| One `dense_source` enum, not `dense_strategy` + `catalog_enabled` | Two independent fields could describe a contradictory state (two producers writing `{ns}_objects_dense`); one enum makes it unrepresentable and deletes the cross-field validation |
| `byoe` / `disabled` skip phase 2; `catalog` does not | Phase 2 also fills `{ns}_subjects_dense`, which the embedder never writes — skipping it under `catalog` would leave subject vectors empty and silently degrade every request to sparse CF |
| `dense_source="catalog"` ⇒ BYOE object PUT returns 409 | One source of truth for the object vector avoids ping-pong overwrites |
| Subject BYOE not catalog-guarded | Subject vectors have no single owner — cron mean-pools them each tick and the endpoint cuts staleness in between |
| Author lives in `objects`, not `catalog_items` | `catalog_items` only exists under `dense_source="catalog"`; attribution had no home under the other sources. Moved, not copied — two stores for one fact drift apart |
| Authored exclusion as point IDs, not a payload filter | A payload filter would reach only the dense collection; cron writes the sparse points and knows nothing about authorship |
| Namespace config writes are PATCH | The admin UI submits only edited fields; `INSERT … ON CONFLICT DO UPDATE` must name every column, which would reset the rest to Go zero values |
| Two-tier auth, admin key valid on every namespace | Per-tenant keys isolate blast radius on leak; the admin key already grants full control via the admin plane, and the admin server must reach every namespace |
| Namespace lifecycle generations fence every writer | Delete/recreate increments the generation; generation 2+ qualifies Redis and Qdrant physical names so stale work from an earlier incarnation cannot become visible |
| Embed strategy registry as a seam | Forward-compat: an unwired build still boots and catalog endpoints return 503 instead of panicking |
| No peer-domain imports | Enforced by test; any domain can be split into a microservice without untangling coupling |
| Single `docs.go` per package | No package docs scattered across files; one canonical place for the description |
| Wire types pinned by golden snapshots | Any rename, retype, or json-tag change on the public contract fails a test instead of breaking clients silently |

## 14. Directory layout

```text
cmd/api                          HTTP data-plane + ingest/catalog workers + tail publisher (2001)
cmd/cron                         Batch recompute + retention + lifecycle janitor
cmd/admin                        Admin server + SPA + pub/sub bridges + operator CLIs (2002)
cmd/embedder                     Catalog worker + sampler + sweeper + heartbeat (2003)

internal/ingest                  HTTP + Redis Streams ingest (events + catalog), events tail
internal/compute                 Sparse + dense recompute, trending
internal/recommend               CF, hybrid, rank, BYOE, object delete
internal/nsconfig                Namespace configuration CRUD
internal/objects                 Per-object metadata (author attribution)
internal/admin                   Admin handlers / services / repos
internal/admin/{eventbus,sse,metricsroll}
internal/catalog                 Catalog ingest (HTTP + stream publish)
internal/embedder                Embed pipeline, re-embed watcher, sampler, sweeper, heartbeat
internal/retention               Observability-table prune job
internal/auth                    Bearer-token validation + negative cache
internal/config                  Env loader
internal/core/embedstrategy      Strategy interface + registry
internal/core/namespace          Shared namespace.Config
internal/core/idmap              String ID → numeric Qdrant point ID
internal/core/nslifecycle        Namespace state, generations, leases, physical names
internal/core/httpapi            JSON helpers, DecodeStrict, middleware
internal/core/batchrun           Shared batch-run logging types
internal/architecture            Repository-wide import-rule tests
internal/infra/{postgres,redis,qdrant,metrics}

migrations/                      SQL migrations (001 … 027)
e2e/                             End-to-end tests (build tag `e2e`)
specs/                           Feature specs and design docs
pkg/codohuetypes                 Shared wire types module
sdk/go                           Public Go SDK (+ sdk/go/admin: bearer admin client)
sdk/go/redistream                Redis Streams producer SDK (events + catalog)
web/admin                        Vite + React 19 + Tailwind v4 SPA
docker/                          Auxiliary Dockerfiles
```

## 15. Related docs

- [README.md](README.md) — overview + quickstart.
- [AGENTS.md](AGENTS.md) — contributor / agent conventions.
- [CLAUDE.md](CLAUDE.md) — thin Claude Code layer that imports the shared `AGENTS.md` instructions.
- [sdk/go/README.md](sdk/go/README.md) — Go SDK + Redis Streams transport.
- [specs/](specs/) — per-feature specs and design docs.
- [internal/architecture/imports_test.go](internal/architecture/imports_test.go) — import rule enforcement.
