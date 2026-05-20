# Codohue

Codohue is a hybrid recommendation service for behavioral personalization.

It ingests events over HTTP and Redis Streams, persists them in PostgreSQL, recomputes sparse and dense vectors on a schedule, optionally auto-embeds raw catalog content, and serves recommendations through HTTP APIs backed by Qdrant.

## What It Does

- Ingests behavioral events from either `POST /v1/namespaces/{ns}/events` or the Redis Stream `codohue:events`
- Persists normalized events in PostgreSQL (`events` table)
- Recomputes sparse collaborative filtering vectors on every cron tick
- Trains dense vectors with `item2vec` or `svd`, or accepts external embeddings with `byoe` / `disabled`
- (Optionally) auto-embeds raw catalog content per namespace into `{ns}_objects_dense`
- Caches a time-decayed trending ZSET in Redis
- Serves recommendations, ranking, trending, and embedding management over HTTP
- Ships an embedded operational SPA (web/admin) on port `2002` with session-cookie auth

## Binaries

The project ships four executables, all built from this repository:

| Binary          | Default port | Purpose                                                                                              |
| --------------- | ------------ | ---------------------------------------------------------------------------------------------------- |
| `cmd/api`       | `2001`       | Data-plane HTTP API + Redis Streams ingest worker goroutine                                          |
| `cmd/cron`      | —            | Batch daemon that recomputes sparse + dense vectors and trending data on a configurable interval     |
| `cmd/admin`     | `2002`       | Admin server: session-cookie auth, `/api/admin/v1/*` operational endpoints, embedded web/admin SPA   |
| `cmd/embedder`  | `2003`       | Catalog auto-embedding worker (consumes `catalog:embed:{ns}` streams); exposes `/healthz` + `/metrics` |

## Modules

This repository is a Go workspace ([go.work](go.work)) with four modules — lint, test, and coverage targets iterate over all of them:

| Module                                       | Purpose                                                                              |
| -------------------------------------------- | ------------------------------------------------------------------------------------ |
| `github.com/jarviisha/codohue`               | Server application — all four binaries, `internal/` domains, e2e suite               |
| `github.com/jarviisha/codohue/pkg/codohuetypes` | Shared wire types so SDK consumers do not pull in pgx/qdrant/prometheus deps      |
| `github.com/jarviisha/codohue/sdk/go`        | Public Go SDK for clients embedding Codohue                                          |
| `github.com/jarviisha/codohue/sdk/go/redistream` | Redis Streams transport helper for the SDK                                       |

See [sdk/go/README.md](sdk/go/README.md) for the HTTP client and `sdk/go/redistream` usage.

## Architecture

### Runtime components

- **PostgreSQL** — durable storage for `events`, `namespace_configs`, `id_mappings`, `batch_run_logs`, `catalog_items`
- **Redis** — event ingest stream, per-namespace catalog embed streams, trending ZSET, recommendation cache
- **Qdrant** — sparse and dense vector collections, per namespace

Per namespace Qdrant collections:

| Collection                | Written by   | Purpose                                                              |
| ------------------------- | ------------ | -------------------------------------------------------------------- |
| `{ns}_subjects`           | `cmd/cron`   | Sparse CF vectors for subjects (dot product similarity)              |
| `{ns}_objects`            | `cmd/cron`   | Sparse CF vectors for objects                                        |
| `{ns}_subjects_dense`     | `cmd/cron`   | Dense subject vectors (mean-pooled from object vectors)              |
| `{ns}_objects_dense`      | `cmd/cron` or `cmd/embedder` | Dense object vectors — from item2vec/svd, BYOE, or catalog auto-embedding |

### Data flow

```
Main Backend ─POST /v1/namespaces/{ns}/events─┐
                                              │
Main Backend ─XADD codohue:events─────────────┤
                                              ▼
                                       Ingest Worker ─▶ PostgreSQL (events)
                                                              │ every N min
                                                              ▼
                                                       Compute Job (cmd/cron)
                                                              │
                                                              ▼
                                                      Qdrant (sparse + dense)
                                                              │
                                              Recommend Service ─▶ Main Backend

Main Backend ─POST /v1/namespaces/{ns}/catalog─▶ catalog_items
                                                       │ XADD catalog:embed:{ns}
                                                       ▼
                                              Embedder Worker (cmd/embedder)
                                                       │ embed + upsert
                                                       ▼
                                              Qdrant {ns}_objects_dense
```

### Batch job phases

`cmd/cron` runs three phases per namespace on each tick:

| Phase | Name     | Description                                                                                                   |
| ----- | -------- | ------------------------------------------------------------------------------------------------------------- |
| 1     | Sparse   | Recomputes CF sparse vectors → `{ns}_subjects` / `{ns}_objects`                                               |
| 2     | Dense    | Trains item embeddings (`item2vec` or `svd`), derives user embeddings via mean pooling → `*_dense`. Skipped when `dense_strategy` is `byoe` or `disabled` |
| 3     | Trending | Computes time-decayed trending scores from recent events into a Redis ZSET. Skipped when Redis is unavailable |

### Internal package layout

Each feature domain lives in `internal/<domain>/` with a consistent `handler.go`, `service.go`, `repository.go`, `types.go` structure (plus a mandatory `docs.go`).

| Package                          | Responsibility                                                                                            |
| -------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `internal/ingest`                | HTTP + Redis Streams event ingestion                                                                      |
| `internal/compute`               | Batch recompute of sparse + dense vectors and trending                                                    |
| `internal/recommend`             | CF, hybrid dense/sparse, rank, trending, BYOE embeddings, object deletion                                 |
| `internal/nsconfig`              | Per-namespace configuration CRUD                                                                          |
| `internal/admin`                 | Handlers, services, repositories for the `cmd/admin` operational dashboard                                |
| `internal/catalog`               | Data-plane catalog ingest; persists `catalog_items` and publishes to `catalog:embed:{ns}`                 |
| `internal/embedder`              | Per-item embed pipeline (load → embed → upsert → mark embedded) + re-embed completion watcher             |
| `internal/auth`                  | Bearer token validation — global admin key and per-namespace bcrypt-hashed keys                           |
| `internal/config`                | Loads + validates application configuration from environment variables                                    |
| `internal/core/embedstrategy`    | Forward-compat seam for embedding strategies (`Strategy` interface + registry)                            |
| `internal/core/namespace`        | Shared `namespace.Config` contracts consumed by every domain                                              |
| `internal/core/idmap`            | String IDs → numeric Qdrant point IDs via `id_mappings`                                                   |
| `internal/core/httpapi`          | Shared JSON HTTP response helpers and middleware                                                          |
| `internal/core/batchrun`         | Shared batch-run logging types                                                                            |
| `internal/architecture`          | Repository architecture tests — enforces the import rule below                                            |
| `internal/infra/{postgres,redis,qdrant,metrics}` | Infrastructure clients and Prometheus metrics                                             |

**Import rule** (enforced by [internal/architecture/imports_test.go](internal/architecture/imports_test.go)): packages under `internal/` may only import `internal/config`, `internal/core/...`, and `internal/infra/...`. Peer-domain imports are forbidden. Cross-domain coordination happens through `cmd/api` and `cmd/admin` wiring.

### Key design decisions

- **Full recompute strategy** — cron recalculates every vector from scratch each run; no incremental get→merge→upsert. Item2Vec retrains fully each run (incremental online Word2Vec causes catastrophic forgetting).
- **ID mapping via DB** — string IDs map to BIGSERIAL numeric IDs through `id_mappings`, avoiding hash collisions.
- **Dense hybrid** — when `alpha < 1.0` and `dense_strategy != "disabled"`, recommendations blend sparse CF (`alpha`) with dense (`1 - alpha`) using min-max normalization.
- **Time decay** — events older than 90 days excluded; freshness multiplier `e^(-λ × days)` at build time; γ-based object freshness at rerank time.
- **Cold start** — 0 interactions → Redis trending (fallback to DB popular); <5 interactions → 70 % trending + 30 % CF.
- **Recommendation cache** — 5-minute Redis cache per `(namespace, subject_id, limit)`.
- **Two-tier auth** — global `CODOHUE_ADMIN_API_KEY` is used by `cmd/admin` for session creation and is **not** accepted for data-plane mutations. Per-namespace bcrypt-hashed keys (in `namespace_configs.api_key_hash`) authenticate data-plane requests; the global key is accepted as fallback only when a namespace has no key provisioned.

## Requirements

- Go `1.26.1` (server application)
- Docker and Docker Compose for local infrastructure
- `golangci-lint` if you want to run lint or format targets
- `air` if you want to use `make dev`
- `migrate` if you want to run database migrations from the host
- Node.js 20+ and `npm` if you work on the web/admin SPA

SDK note: the SDK-related modules (`pkg/codohuetypes`, `sdk/go`, `sdk/go/redistream`) currently target Go `1.24.13` for broader downstream adoption.

## Configuration

Codohue reads configuration from environment variables and also loads `.env` automatically when present.

Required:

- `DATABASE_URL`
- `CODOHUE_ADMIN_API_KEY`

Common variables:

| Variable                            | Default                  | Used by             |
| ----------------------------------- | ------------------------ | ------------------- |
| `REDIS_URL`                         | `redis://localhost:6379` | all                 |
| `QDRANT_HOST`                       | `localhost`              | all                 |
| `QDRANT_PORT`                       | `6334`                   | all                 |
| `CODOHUE_API_PORT`                  | `2001`                   | `cmd/api`           |
| `CODOHUE_ADMIN_PORT`                | `2002`                   | `cmd/admin`         |
| `CODOHUE_API_URL`                   | `http://localhost:2001`  | `cmd/admin` (proxies `/healthz`, event injection) |
| `CODOHUE_BATCH_INTERVAL_MINUTES`    | `5`                      | `cmd/cron`          |
| `CODOHUE_LOG_FORMAT`                | `text`                   | all (`text` or `json`) |

Catalog auto-embedding (`cmd/embedder`):

| Variable                            | Default | Description                                                            |
| ----------------------------------- | ------- | ---------------------------------------------------------------------- |
| `CODOHUE_CATALOG_MAX_CONTENT_BYTES` | `32768` | Default per-namespace cap on raw catalog content (override via admin API) |
| `CODOHUE_EMBED_MAX_ATTEMPTS`        | `5`     | Transient retries before dead-lettering                                |
| `CODOHUE_EMBEDDER_HEALTH_PORT`      | `2003`  | `/healthz` + `/metrics` port                                           |
| `CODOHUE_EMBEDDER_REPLICA_NAME`     | hostname | Consumer name for `XREADGROUP`                                         |
| `CODOHUE_EMBEDDER_POLL_INTERVAL`    | `30s`   | How often the worker rescans for newly-enabled namespaces              |

Example `.env`:

```env
DATABASE_URL=postgres://codohue:secret@localhost:5432/codohue?sslmode=disable
REDIS_URL=redis://localhost:6379
QDRANT_HOST=localhost
QDRANT_PORT=6334
CODOHUE_ADMIN_API_KEY=dev-secret-key
CODOHUE_BATCH_INTERVAL_MINUTES=5
CODOHUE_LOG_FORMAT=text
CODOHUE_API_PORT=2001
CODOHUE_ADMIN_PORT=2002
CODOHUE_API_URL=http://localhost:2001

CODOHUE_CATALOG_MAX_CONTENT_BYTES=32768
CODOHUE_EMBED_MAX_ATTEMPTS=5
CODOHUE_EMBEDDER_HEALTH_PORT=2003
CODOHUE_EMBEDDER_REPLICA_NAME=
CODOHUE_EMBEDDER_POLL_INTERVAL=30s
```

## Quick Start

The project ships three Docker Compose files:

- [docker-compose.yml](docker-compose.yml) — local development; builds all four binaries from source plus `postgres`, `redis`, `qdrant`, and a one-shot `migrate` service
- [docker-compose.app.yml](docker-compose.app.yml) — app-only; builds the four binaries from source and uses host networking to talk to externally managed infra
- [docker-compose.prod.yml](docker-compose.prod.yml) — production; pulls prebuilt images from GHCR, no source mount

### Option 1: Full stack with Docker Compose (development)

**1. Copy and review environment variables**

```bash
cp .env.example .env
```

Defaults in [.env.example](.env.example) match what [docker-compose.yml](docker-compose.yml) injects, so no edits are required for a local first run.

**2. Start the full stack in the background**

```bash
make up-d
```

This starts (migrations run automatically as the `migrate` service before `api` / `cron` / `admin` / `embedder` boot):

| Container            | Image                       | Exposed ports                |
| -------------------- | --------------------------- | ---------------------------- |
| `codohue-migrate`    | built from source           | —                            |
| `codohue-api`        | built from source           | `2001`                       |
| `codohue-cron`       | built from source           | —                            |
| `codohue-admin`      | built from source           | `2002`                       |
| `codohue-embedder`   | built from source           | `2003`                       |
| `codohue-postgres`   | `postgres:16-alpine`        | `5432`                       |
| `codohue-redis`      | `redis:7-alpine`            | `6379`                       |
| `codohue-qdrant`     | `qdrant/qdrant:v1.17.1`     | `6333` (HTTP), `6334` (gRPC) |

**3. Verify the stack**

```bash
curl http://localhost:2001/healthz
curl http://localhost:2001/ping
open  http://localhost:2002      # admin SPA login
```

**Useful commands**

```bash
make logs            # tail API logs
make logs-cron       # tail cron logs
make logs-admin      # tail admin logs
make logs-embedder   # tail embedder logs
make down            # stop and remove containers
make down-v          # stop and remove containers + all local volumes (full reset)
```

---

### Option 2: Run binaries locally against Docker infra

Use this when you want fast iteration on the Go source without rebuilding Docker images.

**1. Start only the infrastructure**

```bash
make up-infra
```

**2. Apply migrations from the host**

```bash
make migrate-up
```

**3. Run any binary**

```bash
make run             # cmd/api
make run-cron        # one cron cycle
make run-admin       # cmd/admin
make run-embedder    # cmd/embedder

make dev             # cmd/api with live reload (requires air)
make dev-admin       # web/admin Vite dev server
make dev-all         # api (air) + admin binary + web/admin Vite, all together
```

---

### Option 3: Apps only with external infrastructure

Use this when Postgres, Redis, and Qdrant are managed outside this stack. [docker-compose.app.yml](docker-compose.app.yml) builds `api`, `cron`, `admin`, and `embedder` from source, uses host networking, and reads `.env` (plus an optional `.env.app`).

```env
DATABASE_URL=postgres://user:password@localhost:5432/codohue?sslmode=disable
REDIS_URL=redis://localhost:6379
QDRANT_HOST=localhost
QDRANT_PORT=6334
CODOHUE_ADMIN_API_KEY=dev-secret-key
CODOHUE_API_URL=http://localhost:2001
```

Start / stop:

```bash
make up-app-d
make down-app
make logs-app
```

If your infra is on another host, point `DATABASE_URL`, `REDIS_URL`, and `QDRANT_HOST` at the reachable addresses instead of `localhost`.

---

### Option 4: Production deployment with Docker Compose

[docker-compose.prod.yml](docker-compose.prod.yml) pulls prebuilt images from GHCR (`api`, `cron`, `admin`, `embedder`, `migrate`). It does **not** include a Postgres container — supply an external database.

**Required environment variables**

```bash
export CODOHUE_DATABASE_URL="postgres://user:password@host:5432/dbname?sslmode=require"
export CODOHUE_ADMIN_API_KEY="your-secret-key"
```

**Start the stack**

```bash
docker compose -f docker-compose.prod.yml up -d
```

**Notable differences from the dev compose**

- All app containers use `restart: unless-stopped`
- Qdrant ports are bound to `127.0.0.1` only (not exposed to the network)
- Redis uses `maxmemory 256mb` with `allkeys-lru` eviction
- `CODOHUE_LOG_FORMAT` defaults to `json`
- The bundled `migrate` service runs once against `CODOHUE_DATABASE_URL` and other services wait on its successful completion

## Namespace Setup

Before calling namespace-scoped data-plane endpoints, create a namespace through `cmd/admin` using `CODOHUE_ADMIN_API_KEY`.

Create an admin session (cookie is stored in `cookies.txt`):

```bash
curl -c cookies.txt -X POST http://localhost:2002/api/v1/auth/sessions \
  -H "Content-Type: application/json" \
  -d '{"api_key":"dev-secret-key"}'
```

Upsert the namespace:

```bash
curl -b cookies.txt -X PUT http://localhost:2002/api/admin/v1/namespaces/demo \
  -H "Content-Type: application/json" \
  -d '{
    "action_weights": {"VIEW": 1, "LIKE": 5, "SHARE": 10},
    "lambda": 0.01,
    "gamma": 0.5,
    "max_results": 20,
    "seen_items_days": 14,
    "alpha": 0.7,
    "dense_strategy": "byoe",
    "embedding_dim": 4,
    "dense_distance": "cosine",
    "trending_window": 24,
    "trending_ttl": 600,
    "lambda_trending": 0.1
  }'
```

On initial creation, the response returns a **plaintext namespace API key once**. Keep it safe — only the bcrypt hash is stored (`namespace_configs.api_key_hash`). Subsequent data-plane calls must send this key as `Authorization: Bearer <key>`. The global admin key is accepted as fallback only when a namespace has no key provisioned.

## Event Ingestion

Two equivalent transports — the `ingest` worker consumes both and writes to the same `events` table.

### HTTP

```
POST /v1/namespaces/{ns}/events
Authorization: Bearer <namespace-key>
Content-Type: application/json
```

The `namespace` field in the body is ignored (URL wins). Use `occurred_at` (RFC3339).

### Redis Streams

Publish to stream `codohue:events`; each message must have a `payload` field with a JSON document carrying the namespace (no URL):

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

```bash
redis-cli XADD codohue:events '*' payload \
  '{"namespace":"demo","subject_id":"user-123","object_id":"item-456","action":"VIEW","timestamp":"2026-04-19T10:00:00Z"}'
```

### Actions

Built-in fallback actions with hardcoded default weights:

- `VIEW`, `LIKE`, `COMMENT`, `SHARE`, `SKIP`

Custom action strings are also accepted when the namespace config defines a matching entry in `action_weights`. An action that is neither configured nor present in the built-in defaults causes ingest to return an `unknown action` error.

## Catalog Auto-Embedding

For namespaces that enable it, Codohue can take raw object content (text, structured fields, etc.) and embed it asynchronously, populating `{ns}_objects_dense` without the caller having to compute vectors.

### Enable on a namespace

```bash
curl -b cookies.txt -X PUT http://localhost:2002/api/admin/v1/namespaces/demo/catalog \
  -H "Content-Type: application/json" \
  -d '{
    "enabled": true,
    "strategy_id": "<registered-strategy-id>",
    "strategy_version": "v1",
    "strategy_params": {...},
    "max_content_bytes": 32768
  }'
```

The available strategies come from `internal/core/embedstrategy`. The admin endpoint returns 400 with both dims in the body on a dimension mismatch with the namespace's `embedding_dim`, and 503 when the strategy registry is unwired in the running build.

### Ingest raw catalog content

```
POST /v1/namespaces/{ns}/catalog
Authorization: Bearer <namespace-key>
```

Persists to `catalog_items` and publishes one message to `catalog:embed:{ns}`. The embedder worker (one consumer per replica, identified by `CODOHUE_EMBEDDER_REPLICA_NAME`) pulls the message, embeds it, and upserts the dense vector into Qdrant. Transient failures retry up to `CODOHUE_EMBED_MAX_ATTEMPTS` before dead-lettering.

When `catalog_enabled=true`, `PUT /v1/namespaces/{ns}/objects/{id}/embedding` returns **409 Conflict** — the catalog pipeline is the source of truth in that mode. Subject embeddings (`PUT /v1/namespaces/{ns}/subjects/{id}/embedding`) are **not** affected.

### Operations (admin plane)

- Trigger a namespace-wide re-embed: `POST /api/admin/v1/namespaces/{ns}/catalog/re-embed` (202 + `Location`; 409 if one already running)
- Browse items: `GET /api/admin/v1/namespaces/{ns}/catalog/items?state=&limit=&offset=`
- Inspect one (with content): `GET /api/admin/v1/namespaces/{ns}/catalog/items/{id}`
- Re-drive a single failed/dead-letter item: `POST /api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive`
- Bulk re-drive every dead-letter item: `POST /api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter`
- Hard-delete an item: `DELETE /api/admin/v1/namespaces/{ns}/catalog/items/{id}`

## HTTP API

### Data plane — `cmd/api` (port 2001)

**Infra / ops (no auth, unversioned)**

| Method | Path        | Description                              |
| ------ | ----------- | ---------------------------------------- |
| GET    | `/ping`     | Liveness probe                           |
| GET    | `/healthz`  | Health check (postgres, redis, qdrant)   |
| GET    | `/metrics`  | Prometheus metrics                       |

**Namespace-scoped (Bearer token; falls back to `CODOHUE_ADMIN_API_KEY` only if the namespace has no provisioned key)**

| Method   | Path                                                       | Description                                                       |
| -------- | ---------------------------------------------------------- | ----------------------------------------------------------------- |
| POST     | `/v1/namespaces/{ns}/events`                               | Ingest a behavioral event (202; body's `namespace` ignored)       |
| POST     | `/v1/namespaces/{ns}/catalog`                              | Ingest raw catalog content (202; only when `catalog_enabled`)     |
| GET      | `/v1/namespaces/{ns}/subjects/{id}/recommendations`        | CF recommendations (`?limit=&offset=`)                            |
| POST     | `/v1/namespaces/{ns}/rankings`                             | Score and rank up to 500 candidates for a subject                 |
| GET      | `/v1/namespaces/{ns}/trending`                             | Trending items (`?limit=&offset=&window_hours=`)                  |
| PUT      | `/v1/namespaces/{ns}/objects/{id}/embedding`               | Store/replace BYOE object vector (204; **409** when `catalog_enabled`) |
| PUT      | `/v1/namespaces/{ns}/subjects/{id}/embedding`              | Store/replace BYOE subject vector (204)                           |
| DELETE   | `/v1/namespaces/{ns}/objects/{id}`                         | Remove object from all Qdrant collections (idempotent 204)        |

### Admin plane — `cmd/admin` (port 2002, session cookie `codohue_admin_session`)

Sessions are modeled as a resource: login = create session, logout = delete current session.

| Method | Path                                                              | Auth     | Description                                                                |
| ------ | ----------------------------------------------------------------- | -------- | -------------------------------------------------------------------------- |
| POST   | `/api/v1/auth/sessions`                                           | public   | Validate `CODOHUE_ADMIN_API_KEY`; set session cookie (201 + `expires_at`)  |
| DELETE | `/api/v1/auth/sessions/current`                                   | session  | Clear session cookie (204)                                                 |
| GET    | `/api/admin/v1/health`                                            | session  | Proxy `GET /healthz` from `cmd/api`                                        |
| GET    | `/api/admin/v1/namespaces`                                        | session  | List namespace configs (`?include=overview`)                               |
| GET    | `/api/admin/v1/namespaces/{ns}`                                   | session  | Get single namespace config                                                |
| PUT    | `/api/admin/v1/namespaces/{ns}`                                   | session  | Create or update namespace (200 / 201)                                     |
| DELETE | `/api/admin/v1/namespaces/{ns}`                                   | session  | Wipe namespace + all its data across postgres, redis, qdrant               |
| POST   | `/api/admin/v1/reset`                                             | session  | App-wide reset — drops every namespace; body must be `{"confirm":"RESET"}` |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog`                           | session  | Catalog config + strategies + backlog snapshot                             |
| PUT    | `/api/admin/v1/namespaces/{ns}/catalog`                           | session  | Enable / update / disable catalog auto-embedding                           |
| POST   | `/api/admin/v1/namespaces/{ns}/catalog/re-embed`                  | session  | Trigger a namespace-wide re-embed                                          |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog/items`                     | session  | Paginated browse (`?state=&limit=&offset=&object_id=`)                     |
| GET    | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}`                | session  | Full catalog item including `content` and `metadata`                       |
| POST   | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive`        | session  | Re-drive a single failed / dead-letter item                                |
| POST   | `/api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter`  | session  | Bulk re-drive every dead-letter item                                       |
| DELETE | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}`                | session  | Hard-delete a catalog item                                                 |
| GET    | `/api/admin/v1/batch-runs`                                        | session  | Recent batch runs (`?namespace=&status=&limit=&offset=`)                   |
| GET    | `/api/admin/v1/namespaces/{ns}/batch-runs`                        | session  | Batch runs scoped to one namespace                                         |
| POST   | `/api/admin/v1/namespaces/{ns}/batch-runs`                        | session  | Create a new batch run (202 + `Location`)                                  |
| GET    | `/api/admin/v1/namespaces/{ns}/qdrant`                            | session  | Point counts across the four `{ns}_*` collections                          |
| GET    | `/api/admin/v1/namespaces/{ns}/trending`                          | session  | Trending items + Redis TTL                                                 |
| GET    | `/api/admin/v1/namespaces/{ns}/events`                            | session  | Paginated recent events (`?limit=&offset=&subject_id=`)                    |
| POST   | `/api/admin/v1/namespaces/{ns}/events`                            | session  | Inject a test event (proxied to `cmd/api`)                                 |
| GET    | `/api/admin/v1/namespaces/{ns}/subjects/{id}/profile`             | session  | Subject profile: interaction count, seen items, sparse NNZ                 |
| GET    | `/api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations`     | session  | Recommendations with optional `?debug=` payload                            |
| POST   | `/api/admin/v1/demo-data`                                         | session  | Seed the bundled demo dataset (202)                                        |
| DELETE | `/api/admin/v1/demo-data`                                         | session  | Clear the bundled demo dataset (204)                                       |

### Error responses

```json
{
  "error": {
    "code": "invalid_request",
    "message": "invalid request body"
  }
}
```

### Recommendation sources

Returned in the `source` field by the recommend / rank endpoints:

- `collaborative_filtering`
- `hybrid` (sparse CF + dense blend)
- `hybrid_cold` (trending + CF blend, for cold-start subjects)
- `hybrid_rank` (ranking endpoint)
- `fallback_popular`

## Web Admin SPA

The admin operations console lives at [web/admin/](web/admin/) — Vite + React 19 + Tailwind v4. It's embedded into the `cmd/admin` binary at build time via the `embedui` build tag.

```bash
make dev-admin         # Vite dev server (separate from cmd/admin)
make dev-all           # api (air) + admin binary + Vite, all together
make build-admin-embed # builds cmd/admin with the SPA bundled (production layout)
```

`make build-admin` (without `-embed`) ships an admin binary that serves only the API; use `build-admin-embed` (or the Dockerfile) when you need the SPA available.

## Development Commands

The [Makefile](Makefile) is the source of truth. Highlights:

```bash
# Build
make build                # all four binaries into ./tmp/
make build-api
make build-cron
make build-admin
make build-admin-embed    # admin binary with SPA embedded
make build-embedder

# Run
make run                  # cmd/api
make run-cron
make run-admin
make run-embedder
make dev                  # cmd/api with air
make dev-admin            # web/admin Vite dev server
make dev-all              # api (air) + admin + web/admin Vite

# Docker
make up-d                 # full stack, detached
make up-infra             # only postgres + redis + qdrant
make up-app-d             # app-only compose, detached
make down / down-v / down-app
make logs / logs-cron / logs-admin / logs-embedder
make compose-check        # validate every compose file

# Lint / test
make lint
make fmt
make test
make test-pkg PKG=./internal/ingest/...
make test-race
make test-verbose

# Coverage
make coverage             # alias for coverage-unit
make coverage-html
make coverage-check-all   # enforces per-package and total minimums (used by CI)

# E2E
make test-e2e
make test-e2e-api
make test-e2e-heavy

# Migrations
make migrate-up
make migrate-down
make migrate-version
make migrate-create NAME=add_indexes
```

Build outputs are written to `tmp/`.

## Testing

Available test layers:

- Unit and package tests for isolated package behavior
- API E2E tests for HTTP contracts and auth flows
- Integration-heavy E2E tests for Redis Streams ingest, cron recompute, hybrid recommendation, and catalog auto-embedding

Unit and package tests:

```bash
make test
make test-race
```

End-to-end tests require infrastructure and migrations:

```bash
make up-infra
make migrate-up
make test-e2e
```

Targeted E2E commands:

```bash
make test-e2e-api
make test-e2e-heavy
go test -v -tags=e2e ./e2e/... -run Ingest
go test -v -tags=e2e ./e2e/... -run Cron
go test -v -tags=e2e ./e2e/... -run 'RecommendComputed|RankComputed'
go test -v -tags=e2e ./e2e/... -run Hybrid
go test -v -tags=e2e ./e2e/... -run Catalog
```

The E2E suite launches the API binary on port `12001`. The full suite also exercises the cron and embedder binaries.

## Project Layout

```text
cmd/api                          HTTP API + ingest worker (port 2001)
cmd/cron                         Batch recompute daemon
cmd/admin                        Admin server + embedded SPA (port 2002)
cmd/embedder                     Catalog auto-embedding worker (port 2003)

internal/ingest                  HTTP + Redis Streams ingest
internal/compute                 Sparse + dense recompute, trending
internal/recommend               CF, hybrid, rank, BYOE
internal/nsconfig                Namespace configuration CRUD
internal/admin                   Admin handlers / services / repos
internal/catalog                 Catalog ingest (HTTP + stream publish)
internal/embedder                Embed pipeline + re-embed watcher
internal/auth                    Bearer-token validation (global + per-ns)
internal/config                  Environment-variable loader
internal/core/embedstrategy      Strategy interface + registry
internal/core/namespace          Shared namespace.Config contracts
internal/core/idmap              String IDs → numeric Qdrant point IDs
internal/core/httpapi            JSON helpers + middleware
internal/core/batchrun           Shared batch-run logging types
internal/architecture            Repository-wide import-rule tests
internal/infra/postgres          pgxpool client
internal/infra/redis             go-redis client
internal/infra/qdrant            Qdrant gRPC client
internal/infra/metrics           Prometheus collectors

migrations/                      SQL migrations (001 … 012)
e2e/                             End-to-end tests (build tag `e2e`)
pkg/codohuetypes                 Shared wire types module
sdk/go                           Public Go SDK
sdk/go/redistream                Redis Streams producer SDK
web/admin                        Vite + React 19 + Tailwind v4 SPA
docker/                          Auxiliary Dockerfiles (e.g. migrate)
```

## Deployment Notes

- [docker-compose.yml](docker-compose.yml) targets local development (builds from source, runs the `migrate` service automatically).
- [docker-compose.prod.yml](docker-compose.prod.yml) runs prebuilt GHCR images for `api`, `cron`, `admin`, `embedder`, and `migrate`.
- The production compose file requires `CODOHUE_DATABASE_URL` and `CODOHUE_ADMIN_API_KEY` from the environment.

## Notes

- Namespace keys are only returned in plaintext on first namespace creation — only the bcrypt hash is stored.
- Do not commit secrets, `.env` files, or plaintext namespace API keys.
- Redis and Qdrant state can influence local behavior; use `make down-v` for a full reset.
