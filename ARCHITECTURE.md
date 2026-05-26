# Codohue — Architecture

This document describes Codohue's current architecture: the processes, communication boundaries, storage layers, data flow, and important design decisions. The README only carries the overview + quickstart; everything architectural lives here.

## 1. Overview

Codohue is a **hybrid sparse + dense collaborative filtering** recommendation service for multi-tenant behavioral personalization. The system is organized around four small binaries that run independently, a four-module Go workspace, and three infrastructure components (PostgreSQL, Redis, Qdrant).

- **Sparse CF** is built from the event log (click, like, share, …) by a cron job and stored in Qdrant `{ns}_subjects` / `{ns}_objects` collections (sparse dot product).
- **Dense vectors** can come from three sources — locally trained (`item2vec` / `svd`), bring-your-own-embedding (`byoe`), or auto-embedded from raw catalog content. Stored in `{ns}_subjects_dense` / `{ns}_objects_dense` (cosine).
- **Hybrid blending** mixes sparse and dense scores at serve time, while applying time-decay and γ-freshness at rerank.
- **Multi-tenant** by `namespace`; each namespace owns its config, Qdrant collections, Redis streams, and API key.

```
┌──────────────┐    HTTP / Redis Streams       ┌──────────────────────┐
│ Main Backend │ ─────────────────────────────▶│ cmd/api  (port 2001) │
└──────────────┘                               │  + ingest worker     │
       │                                       └──────────┬───────────┘
       │  PUT /catalog                                    │ writes
       │                                                  ▼
       │                                          ┌────────────────┐
       │                                          │  PostgreSQL    │
       │                                          │  events,       │
       │                                          │  catalog_items │
       │                                          │  configs, …    │
       │                                          └───┬─────────┬──┘
       │                                              │ reads   │
       │                                              ▼         ▼
       │                                       ┌─────────┐ ┌────────────┐
       │                                       │cmd/cron │ │cmd/embedder│
       │                                       │(batch)  │ │(stream     │
       │                                       │         │ │ consumer)  │
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
                    └──────────────────────┘
```

## 2. Processes (Binaries)

All four binaries are built from the same repo, each with a clearly scoped role. Inter-binary communication goes through PostgreSQL, Redis, and Qdrant — there is **no** direct RPC between binaries (except `cmd/admin` proxying `/healthz` and injecting test events into `cmd/api`).

| Binary          | Port | Role |
| --------------- | ---- | ---- |
| [cmd/api](cmd/api)           | 2001 | HTTP data-plane (events, recommendations, rankings, trending, BYOE) plus a goroutine `ingest` worker consuming the Redis Stream `codohue:events` |
| [cmd/cron](cmd/cron)         | —    | Batch daemon driven by `CODOHUE_BATCH_INTERVAL_MINUTES` (default 5 min). Each tick runs three phases per namespace |
| [cmd/admin](cmd/admin)       | 2002 | Admin server: session-cookie auth, `/api/admin/v1/*`, embeds the `web/admin` SPA (Vite + React 19 + Tailwind v4) via the `embedui` build tag |
| [cmd/embedder](cmd/embedder) | 2003 | Per-item worker: consumes `catalog:embed:{ns}` streams, embeds via `embedstrategy.Strategy`, upserts the dense vector. Also runs the re-embed completion watcher that closes admin-triggered batch runs when a namespace's backlog drains |

### 2.1 Process ↔ storage matrix

| Storage             | api | cron | admin | embedder |
| ------------------- | :-: | :--: | :---: | :------: |
| PostgreSQL          | RW  | RW   | RW    | RW       |
| Redis (stream)      | R (consume `codohue:events`), W (reco cache)        | W (trending ZSET) | R (trending, qdrant counts) | RW (consume `catalog:embed:{ns}`) |
| Qdrant              | R (search), W (BYOE upsert, object delete) | W (sparse + dense upsert) | R (counts) | W (`{ns}_objects_dense` upsert) |

## 3. Go Workspace

The repo is a Go workspace ([go.work](go.work)) with **four modules**. `make lint`/`test`/`coverage` iterate over each module.

| Module path                                          | Role |
| ---------------------------------------------------- | ---- |
| `github.com/jarviisha/codohue` (`.`)                 | Server application — four binaries, every `internal/` domain, the e2e suite |
| `github.com/jarviisha/codohue/pkg/codohuetypes`      | Shared wire types so the SDK doesn't pull pgx/qdrant/prometheus deps |
| `github.com/jarviisha/codohue/sdk/go`                | Public Go SDK for clients embedding Codohue |
| `github.com/jarviisha/codohue/sdk/go/redistream`     | Redis Streams transport helper for the SDK |

The server module currently targets Go `1.26.1`. The SDK modules (`pkg/codohuetypes`, `sdk/go`, `sdk/go/redistream`) deliberately stay on Go `1.24.13` for broader downstream adoption.

## 4. Internal layering

Each feature domain lives at `internal/<domain>/` with a consistent file set: `handler.go`, `service.go`, `repository.go`, `types.go`, plus a mandatory `docs.go` as the single canonical place for the package doc.

| Package                              | Responsibility |
| ------------------------------------ | -------------- |
| [internal/ingest](internal/ingest)             | Accepts events via HTTP and Redis Streams, validates, persists to `events` |
| [internal/compute](internal/compute)           | Batch: sparse + dense recompute, trending |
| [internal/recommend](internal/recommend)       | CF, hybrid dense/sparse, rank, trending, BYOE embeddings, object delete |
| [internal/nsconfig](internal/nsconfig)         | CRUD for per-namespace config (weights, decay, dense hybrid, catalog) |
| [internal/admin](internal/admin)               | Handlers/services/repos for `cmd/admin` |
| [internal/catalog](internal/catalog)           | Data-plane HTTP content ingest; persists `catalog_items`, publishes `catalog:embed:{ns}` |
| [internal/embedder](internal/embedder)         | Per-item pipeline (load → embed → upsert → mark embedded) + re-embed completion watcher |
| [internal/auth](internal/auth)                 | Bearer-token validation: admin key + per-namespace bcrypt key |
| [internal/config](internal/config)             | Env-var loader |
| [internal/core/embedstrategy](internal/core/embedstrategy) | Forward-compat seam: `Strategy` interface + registry (both `catalog` and `embedder` depend on the seam, never on each other) |
| [internal/core/namespace](internal/core/namespace)         | Shared `namespace.Config` contract |
| [internal/core/idmap](internal/core/idmap)                 | String ID → BIGSERIAL numeric ID through `id_mappings` |
| [internal/core/httpapi](internal/core/httpapi)             | JSON response helpers + middleware |
| [internal/core/batchrun](internal/core/batchrun)           | Shared batch-run logging types |
| [internal/architecture](internal/architecture)             | Repo-wide import-rule enforcement test |
| [internal/infra/{postgres,redis,qdrant,metrics}](internal/infra) | pgxpool, go-redis, Qdrant gRPC, Prometheus collectors |

### 4.1 Import rule (hard)

Enforced by [internal/architecture/imports_test.go](internal/architecture/imports_test.go):

- Packages under `internal/` may import only `internal/config`, `internal/core/...`, and `internal/infra/...`.
- Peer-domain imports are **forbidden** (for example, `recommend` may not import `ingest`).
- Cross-domain coordination happens at the wiring layer in `cmd/api` and `cmd/admin` (see [cmd/admin/nsconfig_adapter.go](cmd/admin/nsconfig_adapter.go)).

This shape lets any domain be split into a separate microservice later without untangling coupling.

## 5. Data model

### 5.1 PostgreSQL

Migrations live under [migrations/](migrations/) as `NNN_name.up.sql` / `NNN_name.down.sql`.

| Table                   | Role |
| ----------------------- | ---- |
| `namespace_configs`     | Per-namespace config: `action_weights`, `lambda`, `gamma`, `max_results`, `seen_items_days`, `alpha`, `dense_strategy`, `embedding_dim`, `dense_distance`, `trending_*`, `api_key_hash`, `catalog_enabled`, `catalog_strategy_id`, `catalog_strategy_version`, `catalog_strategy_params` |
| `events`                | Behavioral events: `namespace`, `subject_id`, `object_id`, `action`, `occurred_at`, `object_created_at`. Indexed on `(namespace, subject_id)` and `occurred_at` |
| `id_mappings`           | String ID → BIGSERIAL numeric, scoped by `(namespace, entity_type)`. Used as the Qdrant point ID to avoid hash collisions |
| `batch_run_logs`        | History of every cron tick / admin re-embed: `trigger_source ∈ {cron, manual, admin_reembed}`, phase{1,2,3} ok/duration/entities/objects/error, `log_lines` JSONB |
| `catalog_items`         | Raw content per object: state machine `pending → embedding → embedded` (plus `failed` / `dead_letter`); `content`, `metadata` |

Columns accumulated across later migrations:
- **002** `gamma` (object freshness rerank)
- **003** `seen_items_days` (recency filter window)
- **004** `events.object_created_at`
- **005** `api_key_hash`, `alpha`, `dense_strategy`, `embedding_dim`, `trending_*`
- **006** `batch_run_logs` table
- **007** phase breakdown columns
- **008** `trigger_source`
- **009** `log_lines` JSONB
- **010** `catalog_items` table
- **011** catalog columns on `namespace_configs`
- **012** pre-prod hardening: CHECK on `trigger_source`; `target_strategy_id` / `target_strategy_version`; rename `subjects_processed` → `entities_processed`

### 5.2 Redis

| Key                                | Kind        | Producer        | Consumer / TTL |
| ---------------------------------- | ----------- | --------------- | -------------- |
| `codohue:events`                   | Stream      | Main Backend    | `cmd/api` ingest worker (consumer group) |
| `catalog:embed:{ns}`               | Stream      | `internal/catalog` (publishes on PUT catalog) | `cmd/embedder` (per-replica consumer group, `CODOHUE_EMBEDDER_REPLICA_NAME`) |
| Trending ZSET (per namespace)      | Sorted set  | `cmd/cron` phase 3 | `recommend` service; TTL = `trending_ttl` |
| Recommendation cache               | String/Hash | `recommend`       | 5 minutes per `(namespace, subject_id, limit)` |

### 5.3 Qdrant

Each namespace has **four collections**:

| Collection              | Vector kind | Distance     | Writer       | Purpose |
| ----------------------- | ----------- | ------------ | ------------ | ------- |
| `{ns}_subjects`         | Sparse      | Dot          | `cmd/cron`   | Sparse CF subject vectors |
| `{ns}_objects`          | Sparse      | Dot          | `cmd/cron`   | Sparse CF object vectors  |
| `{ns}_subjects_dense`   | Dense       | Cosine       | `cmd/cron` (mean-pool) or `cmd/api` (BYOE PUT) | Dense subject vector |
| `{ns}_objects_dense`    | Dense       | Cosine       | `cmd/cron` (item2vec/svd), `cmd/api` (BYOE), or `cmd/embedder` (catalog) | Dense object vector |

Point IDs are `int64` values from `id_mappings`. The dimension of each dense collection is taken from `namespace_configs.embedding_dim`.

## 6. Batch job (`cmd/cron`)

Each tick iterates over every namespace and runs three sequential phases; each phase can be skipped independently and is logged separately into `batch_run_logs`.

| Phase | Name      | Description |
| ----- | --------- | ----------- |
| 1     | Sparse    | Reads `events` from the last 90 days, applies `action_weights × e^(-λ × days_since)`, builds subject/object sparse vectors, upserts into `{ns}_subjects` / `{ns}_objects` |
| 2     | Dense     | Trains item embeddings via `item2vec` or `svd`; derives user embeddings by mean-pooling object vectors over history; upserts `*_dense`. **Skipped** when `dense_strategy ∈ {byoe, disabled}` (preserves vectors written by `cmd/api` or `cmd/embedder`) |
| 3     | Trending  | Computes time-decayed trending from recent events into a Redis ZSET. **Skipped** when Redis is unavailable |

### 6.1 Why full recompute?

Every cron tick rebuilds every vector from scratch. Reasons:
- Avoids race conditions inherent to get→merge→upsert flows.
- Item2Vec lacks a stable incremental online variant; incremental Word2Vec causes **catastrophic forgetting**. Full retraining keeps embedding quality consistent.

Trade-off accepted: the batch runs at `CODOHUE_BATCH_INTERVAL_MINUTES` (default 5 min), so sparse/dense freshness is bounded by the tick interval.

## 7. Catalog auto-embedding (`cmd/embedder`)

Lets callers submit raw content (text + structured fields) instead of computing embeddings themselves. The worker turns content into a dense vector and upserts `{ns}_objects_dense`.

### 7.1 Pipeline

```
POST /v1/namespaces/{ns}/catalog
        │   (internal/catalog)
        ▼
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

### 7.2 Retry & dead-letter

- Transient errors retry up to `CODOHUE_EMBED_MAX_ATTEMPTS` (default 5) before moving to dead-letter.
- Admin can redrive a single item (`POST /catalog/items/{id}/redrive`) or in bulk (`POST /catalog/items/redrive-deadletter`).
- An unwired strategy registry in the running build causes catalog endpoints to return **503** (forward-compat seam).

### 7.3 BYOE ↔ catalog interaction

- When `catalog_enabled = true`, `PUT /v1/namespaces/{ns}/objects/{id}/embedding` returns **409 Conflict** — the catalog pipeline is the source of truth for object vectors.
- `PUT /v1/namespaces/{ns}/subjects/{id}/embedding` is **not** guarded — subject vectors still flow through `cmd/cron`'s mean-pool regardless of catalog mode.

### 7.4 Re-embed batch

`POST /api/admin/v1/namespaces/{ns}/catalog/re-embed` opens a fresh `batch_run_logs` row with `trigger_source = admin_reembed` and attached `target_strategy_id` + `target_strategy_version`, then re-publishes every catalog item. The re-embed completion watcher (inside `cmd/embedder`) closes the run when the namespace's backlog drains to 0. Returns **409** if another run is already in progress.

## 8. Recommendation pipeline

### 8.1 Subject state → source

| Subject interactions  | Source                                 |
| --------------------- | -------------------------------------- |
| 0                     | Redis trending ZSET (falls back to DB popular when Redis is empty) |
| 1 ≤ N < 5             | Hybrid cold: **70%** trending + **30%** CF |
| N ≥ 5                 | CF (sparse) or hybrid dense+sparse blend when enabled |

### 8.2 Hybrid blend

When `alpha < 1.0` and `dense_strategy != "disabled"`:

```
score_final = alpha · normalize(score_sparse) + (1 - alpha) · normalize(score_dense)
```

Min-max normalization is applied over the candidate pool before merging. Sparse search runs against `{ns}_objects`, dense search against `{ns}_objects_dense` using the user's subject vector.

### 8.3 Time decay & freshness rerank

- **Build time** (cron): each event is multiplied by `e^(-λ × days_since)` (λ = `namespace_configs.lambda`). Events older than 90 days are dropped.
- **Rerank time** (recommend): the final score is multiplied by the object's γ-based freshness (`gamma` field) — favors newer objects when scores are close.

### 8.4 Seen-items filter

Subjects do not see objects they interacted with in the last `seen_items_days` (default 14). Read directly from `events`, no cache.

### 8.5 Cache

Responses are cached in Redis for 5 minutes per `(namespace, subject_id, limit)`. BYOE PUT / object delete do **not** invalidate the cache — changes appear after the TTL expires or after the next cron tick.

### 8.6 Returned `source` field

- `collaborative_filtering`
- `hybrid` — sparse + dense blend
- `hybrid_cold` — trending + CF blend (cold start)
- `hybrid_rank` — `/rankings` endpoint
- `fallback_popular`

## 9. Authentication

A **two-tier** model.

| Plane            | Auth                                                                              | Token storage |
| ---------------- | --------------------------------------------------------------------------------- | ------------- |
| Admin (`cmd/admin`) | Session cookie `codohue_admin_session`. Login = `POST /api/v1/auth/sessions` with `CODOHUE_ADMIN_API_KEY` | Server-side session |
| Data (`cmd/api`) | `Authorization: Bearer <namespace-key>` — bcrypt-hashed in `namespace_configs.api_key_hash` | Plaintext returned **once** on namespace creation |

`CODOHUE_ADMIN_API_KEY` is accepted by the data plane **only when the namespace has no provisioned key** (fallback). All namespace-config mutations must go through the admin plane.

## 10. HTTP API

### 10.1 Data plane — `cmd/api` (port 2001)

Every business capability has **exactly one canonical path**. Legacy duplicate paths have been removed → 404.

**Infra/ops (no auth)**

| Method | Path        | Description                                 |
| ------ | ----------- | ------------------------------------------- |
| GET    | `/ping`     | Liveness                                    |
| GET    | `/healthz`  | postgres + redis + qdrant                   |
| GET    | `/metrics`  | Prometheus                                  |

**Namespace-scoped (Bearer)**

| Method | Path                                                 | Description |
| ------ | ---------------------------------------------------- | ----------- |
| POST   | `/v1/namespaces/{ns}/events`                         | Ingest event (202; `namespace` in body is ignored) |
| POST   | `/v1/namespaces/{ns}/catalog`                        | Ingest raw content (202; only when `catalog_enabled`) |
| GET    | `/v1/namespaces/{ns}/subjects/{id}/recommendations`  | CF recommendations (`?limit=&offset=`) |
| POST   | `/v1/namespaces/{ns}/rankings`                       | Score + rank up to 500 candidates |
| GET    | `/v1/namespaces/{ns}/trending`                       | Trending (`?limit=&offset=&window_hours=`) |
| PUT    | `/v1/namespaces/{ns}/objects/{id}/embedding`         | BYOE object vector (204; **409** when `catalog_enabled`) |
| PUT    | `/v1/namespaces/{ns}/subjects/{id}/embedding`        | BYOE subject vector (204; not catalog-guarded) |
| DELETE | `/v1/namespaces/{ns}/objects/{id}`                   | Remove object from every Qdrant collection (idempotent 204) |

### 10.2 Admin plane — `cmd/admin` (port 2002, session cookie)

Sessions are modeled as a resource: login = create, logout = delete current.

| Method | Path                                                              | Description |
| ------ | ----------------------------------------------------------------- | ----------- |
| POST   | `/api/v1/auth/sessions`                                           | Validate admin key, set cookie (201 + `expires_at`) |
| DELETE | `/api/v1/auth/sessions/current`                                   | Clear cookie (204) |
| GET    | `/api/admin/v1/health`                                            | Proxy `/healthz` from `cmd/api` |
| GET    | `/api/admin/v1/namespaces`                                        | List configs (`?include=overview`) |
| GET    | `/api/admin/v1/namespaces/{ns}`                                   | Get config |
| PUT    | `/api/admin/v1/namespaces/{ns}`                                   | Create/update (200/201) |
| DELETE | `/api/admin/v1/namespaces/{ns}`                                   | Wipe namespace + all its data (200 summary; 404 when missing) |
| POST   | `/api/admin/v1/reset`                                             | App-wide reset; body `{"confirm":"RESET"}` |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog`                           | Catalog config + strategies + backlog |
| PUT    | `/api/admin/v1/namespaces/{ns}/catalog`                           | Enable/update/disable catalog (400 on dim mismatch; 503 unwired) |
| POST   | `/api/admin/v1/namespaces/{ns}/catalog/re-embed`                  | Trigger re-embed (202 + `Location`; 409 if one is running) |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog/items`                     | Browse (`?state=&limit=&offset=&object_id=`) |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}`                | Full item (with `content`, `metadata`) |
| POST   | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive`        | Redrive a single item |
| POST   | `/api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter`  | Bulk redrive dead-letter |
| DELETE | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}`                | Hard-delete |
| GET    | `/api/admin/v1/batch-runs`                                        | Recent batch runs (`?namespace=&status=&limit=&offset=`) |
| GET    | `/api/admin/v1/namespaces/{ns}/batch-runs`                        | Batch runs scoped to a namespace |
| POST   | `/api/admin/v1/namespaces/{ns}/batch-runs`                        | Create a new batch run (202 + `Location`) |
| GET    | `/api/admin/v1/namespaces/{ns}/qdrant`                            | Point counts across the four collections |
| GET    | `/api/admin/v1/namespaces/{ns}/trending`                          | Trending + Redis TTL |
| GET    | `/api/admin/v1/namespaces/{ns}/events`                            | Recent events (`?limit=&offset=&subject_id=`) |
| POST   | `/api/admin/v1/namespaces/{ns}/events`                            | Inject a test event (proxied to `cmd/api`) |
| GET    | `/api/admin/v1/namespaces/{ns}/subjects/{id}/profile`             | Interaction count, seen items, sparse NNZ |
| GET    | `/api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations`     | Recommendations with `?debug=` |
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

Built-in: `VIEW`, `LIKE`, `COMMENT`, `SHARE`, `SKIP` (with default weights). Custom actions are accepted when `namespace_configs.action_weights` has a matching entry; otherwise ingest returns an `unknown action` error.

## 12. Observability

- **Prometheus** — collectors in `internal/infra/metrics`, exposed at `GET /metrics` from both `cmd/api` (2001) and `cmd/embedder` (2003).
- **Batch run history** — `batch_run_logs` records every cron tick and admin re-embed; the `log_lines` JSONB column captures the run's slog output, surfaced through the admin API.
- **slog format** — `CODOHUE_LOG_FORMAT=text` (default) or `json` (the prod compose defaults to `json`).
- **Healthcheck** — `GET /healthz` on `cmd/api` checks postgres + redis + qdrant; admin proxies it at `/api/admin/v1/health`.

## 13. Key design decisions

| Decision | Reason |
| -------- | ------ |
| Full recompute every cron tick | Avoids race conditions in get→merge→upsert; item2vec retraining avoids catastrophic forgetting |
| ID mapping via DB (BIGSERIAL)  | Avoids hash collisions for Qdrant numeric point IDs |
| Sparse + dense as separate collections | Different distance/algorithm (Dot vs Cosine); search runs independently before blending |
| Hybrid blend with min-max normalize | Sparse scores and dense cosine live on different scales; min-max over the candidate pool normalizes to [0, 1] |
| `byoe` / `disabled` skip phase 2 | When the caller already has high-quality embeddings (LLM, CV), cron should not overwrite them |
| Catalog enabled ⇒ BYOE object 409 | One source of truth for the object vector avoids ping-pong overwrites |
| Subject BYOE not catalog-guarded | Catalog auto-embed only owns object content; subject vectors always come from behavior |
| Two-tier auth (admin/namespace) | Per-tenant namespace keys isolate blast radius on leak; the admin key only opens the admin plane |
| Embed strategy registry as a seam | Forward-compat: an unwired build still boots and catalog endpoints return 503 instead of panicking |
| No peer-domain imports | Enforced by test; any domain can be split into a microservice without untangling coupling |
| Single `docs.go` per package | No package docs scattered across files; one canonical place for the description |

## 14. Directory layout

```text
cmd/api                          HTTP data-plane + ingest worker (2001)
cmd/cron                         Batch recompute daemon
cmd/admin                        Admin server + embedded SPA (2002)
cmd/embedder                     Catalog auto-embed worker (2003)

internal/ingest                  HTTP + Redis Streams ingest
internal/compute                 Sparse + dense recompute, trending
internal/recommend               CF, hybrid, rank, BYOE
internal/nsconfig                Namespace configuration CRUD
internal/admin                   Admin handlers / services / repos
internal/catalog                 Catalog ingest (HTTP + stream publish)
internal/embedder                Embed pipeline + re-embed watcher
internal/auth                    Bearer-token validation
internal/config                  Env loader
internal/core/embedstrategy      Strategy interface + registry
internal/core/namespace          Shared namespace.Config
internal/core/idmap              String ID → numeric Qdrant point ID
internal/core/httpapi            JSON helpers + middleware
internal/core/batchrun           Shared batch-run logging types
internal/architecture            Repository-wide import-rule tests
internal/infra/{postgres,redis,qdrant,metrics}

migrations/                      SQL migrations (001 … 012)
e2e/                             End-to-end tests (build tag `e2e`)
pkg/codohuetypes                 Shared wire types module
sdk/go                           Public Go SDK
sdk/go/redistream                Redis Streams producer SDK
web/admin                        Vite + React 19 + Tailwind v4 SPA
docker/                          Auxiliary Dockerfiles
```

## 15. Related docs

- [README.md](README.md) — overview + quickstart.
- [AGENTS.md](AGENTS.md) — contributor / agent conventions.
- [CLAUDE.md](CLAUDE.md) — detailed instructions for Claude Code working in this repo.
- [sdk/go/README.md](sdk/go/README.md) — Go SDK + Redis Streams transport.
- [internal/architecture/imports_test.go](internal/architecture/imports_test.go) — import rule enforcement.
