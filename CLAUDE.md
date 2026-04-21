# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Codohue** is a collaborative filtering recommendation engine for the DarkVoid project. It ingests behavioral events (clicks, likes, comments, shares, skips), builds sparse vectors, and serves personalized recommendations via Qdrant vector search.

## Commands

```bash
make build          # build both binaries (./tmp/api and ./tmp/cron)
make build-api      # build API only
make build-cron     # build cron job only

make up             # start full stack (docker compose up, foreground)
make up-d           # start full stack (docker compose up, detached)
make up-infra       # start only postgres + redis + qdrant
make down           # stop stack
make logs           # tail api container logs

make dev            # live-reload API using air
make run            # run API directly (requires infra already up)
make run-cron       # run cron job once (requires infra already up)

make test           # run all tests
make test-pkg PKG=./internal/ingest/...   # test a specific package
make test-verbose   # test with verbose output

make lint           # run golangci-lint
make fmt            # auto-format imports

make migrate-up     # run all pending migrations
make migrate-down   # roll back 1 migration
make migrate-version  # show current migration version
make migrate-create NAME=add_indexes  # create new migration files

make clean          # delete ./tmp/
```

Live reload in development uses `air` (configured in `.air.toml`), auto-rebuilds `cmd/api` on Go file changes.

## Architecture

### Two Binaries

- **`cmd/api`** — HTTP API server (port **2001**) + Redis Streams ingest worker goroutine
- **`cmd/cron`** — Batch job daemon that recomputes sparse vectors on a configurable interval (default: 5 min)

### Data Flow

```
Main Backend → Redis Streams → Ingest Worker → PostgreSQL (events table)
                                                       ↓ (every N min)
                                               Compute Job (cron binary)
                                                       ↓
                                               Qdrant Collections (sparse vectors)
                                                       ↓
                                               Recommend Service → Main Backend
```

### Domain Organization

Each feature domain lives in `internal/<domain>/` with a consistent `handler.go`, `service.go`, `repository.go`, `types.go` structure:

| Domain       | Responsibility                                                                           |
| ------------ | ---------------------------------------------------------------------------------------- |
| `ingest`     | Consumes Redis Streams events, validates, stores to `events` table                       |
| `compute`    | Batch recomputes sparse vectors with time decay, upserts to Qdrant                       |
| `recommend`  | Serves CF recommendations, hybrid dense/sparse, trending, BYOE embeddings                |
| `nsconfig`   | CRUD for per-namespace configuration (action weights, decay params, dense hybrid config) |
| `core/idmap` | Maps string IDs → numeric Qdrant point IDs via `id_mappings` table                       |
| `auth`       | Bearer token validation — global admin key and per-namespace bcrypt-hashed keys          |

**Import rule:** domains import `core/`, `infra/`, and `config/` — never each other. Exception: `recommend` imports `nsconfig` for config lookups.

### Infrastructure Clients (`internal/infra/`)

- `postgres/db.go` — pgxpool connection
- `redis/client.go` — go-redis client (Streams + trending ZSET)
- `qdrant/client.go` — Qdrant gRPC client
- `metrics/metrics.go` — Prometheus counters/histograms; exposed at `GET /metrics`

### Batch Job Phases

The cron binary runs three phases per namespace on each tick:

| Phase | Name     | Description                                                                           |
| ----- | -------- | ------------------------------------------------------------------------------------- |
| 1     | Sparse   | Recomputes CF sparse vectors and upserts to `{ns}_subjects` / `{ns}_objects` Qdrant collections |
| 2     | Dense    | Trains item embeddings via `item2vec` or `svd` strategy; derives user embeddings via mean pooling; upserts to `{ns}_subjects_dense` / `{ns}_objects_dense`. Skipped when `dense_strategy` is `"byoe"` or `"disabled"` |
| 3     | Trending | Computes time-decayed trending scores from recent events and caches them in Redis ZSET. Skipped when Redis is unavailable |

### Key Design Decisions

- **Full recompute strategy**: The cron job recalculates all vectors from scratch each run to avoid incremental update race conditions. No get→merge→upsert pattern. Item2Vec retrains fully each run — incremental online Word2Vec is deliberately avoided because it causes catastrophic forgetting.
- **ID mapping via DB**: String IDs map to numeric Qdrant point IDs through the `id_mappings` table (BIGSERIAL), avoiding hash collisions.
- **Sparse vectors**: `{ns}_subjects` and `{ns}_objects` Qdrant collections hold sparse interaction vectors (dot product similarity).
- **Dense hybrid (BYOE)**: `{ns}_subjects_dense` and `{ns}_objects_dense` hold externally-provided dense embeddings. When `alpha < 1.0` and `dense_strategy != "disabled"`, the recommend service blends sparse CF scores (weight=`alpha`) with dense scores (weight=`1-alpha`) using min-max normalization before merging.
- **Time decay**: Events older than 90 days excluded. Freshness multiplier `e^(-λ × days_since)` applied during vector build; γ-based object freshness applied at rerank time.
- **Cold start**: 0 interactions → Redis trending ZSET (fallback to DB popular); <5 interactions → 70% trending + 30% CF hybrid.
- **Recommendation cache**: Results cached in Redis for 5 minutes per `(namespace, subject_id, limit)` key.
- **Two-tier auth**: Global `RECOMMENDER_API_KEY` for admin routes (namespace config upsert); per-namespace bcrypt-hashed keys for data routes (stored in `namespace_configs.api_key_hash`). Falls back to global key when a namespace has no key provisioned.

### REST API

| Method   | Path                                | Auth                    | Description                                                         |
| -------- | ----------------------------------- | ----------------------- | ------------------------------------------------------------------- |
| `GET`    | `/ping`                             | none                    | Liveness probe                                                      |
| `GET`    | `/healthz`                          | none                    | Health check for postgres, redis, qdrant                            |
| `GET`    | `/metrics`                          | none                    | Prometheus metrics                                                  |
| `PUT`    | `/v1/config/namespaces/{namespace}` | global admin key        | Upsert namespace config; returns plaintext API key on first create  |
| `GET`    | `/v1/recommendations`               | namespace key           | CF recommendations for a subject (`?subject_id=&namespace=&limit=`) |
| `POST`   | `/v1/rank`                          | namespace key (in body) | Score and rank a list of candidate items for a subject (max 500 candidates) |
| `GET`    | `/v1/trending/{ns}`                 | namespace key           | Trending items from Redis ZSET (`?limit=&offset=&window_hours=`)    |
| `POST`   | `/v1/objects/{ns}/{id}/embedding`   | namespace key           | Store BYOE dense vector for an object                               |
| `POST`   | `/v1/subjects/{ns}/{id}/embedding`  | namespace key           | Store BYOE dense vector for a subject                               |
| `DELETE` | `/v1/objects/{ns}/{id}`             | namespace key           | Remove an object from all Qdrant collections (idempotent)           |

### Database Schema

Migrations live in `migrations/` as `NNN_name.up.sql` / `NNN_name.down.sql` pairs. Core tables from `001_initial.up.sql`:

- `namespace_configs` — per-namespace config (action weights, decay params, dense hybrid settings, `api_key_hash`)
- `events` — raw behavioral events; indexed on `(namespace, subject_id)` and `occurred_at`
- `id_mappings` — string ID → BIGSERIAL numeric ID, scoped by `(namespace, entity_type)`

Key columns added by later migrations: `gamma` (002), `seen_items_days` (003), `object_created_at` on events (004), `api_key_hash`/`alpha`/`dense_strategy`/`embedding_dim`/`trending_*` on namespace_configs (005).

## Environment Variables

Copy `.env.example` to `.env`:

```
DATABASE_URL=postgres://codohue:secret@localhost:5432/codohue?sslmode=disable
REDIS_URL=redis://localhost:6379
QDRANT_HOST=localhost
QDRANT_PORT=6334
RECOMMENDER_API_KEY=dev-secret-key
BATCH_INTERVAL_MINUTES=5
LOG_FORMAT=text   # "text" (default) | "json"
```

## Conventions

### Package documentation

Every package must have a `docs.go` file containing only the `// Package <name> ...` comment and the `package` declaration. This is the canonical place to describe what the package does — do not scatter package-level explanations across other files.

```go
// Package ingest consumes behavioral events from Redis Streams,
// validates them, and persists them to the events table in PostgreSQL.
package ingest
```

### Comments language

All code comments (inline comments, doc comments, TODO notes) must be written in **English**. This applies to every `.go` file in the repository without exception.

### Test files

Every file that contains business logic (`service.go`, `repository.go`, `job.go`, `worker.go`) must have a corresponding `_test.go` file. Handler tests live in `handler_test.go`. Files that only declare types (`types.go`) or wire dependencies (`docs.go`) do not require test files.
