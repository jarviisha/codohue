# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Codohue** is a collaborative filtering recommendation engine for the DarkVoid project. It ingests behavioral events (clicks, likes, comments, shares, skips), builds sparse vectors, and serves personalized recommendations via Qdrant vector search.

## Commands

```bash
make build          # build all three binaries (./tmp/api, ./tmp/cron, ./tmp/admin)
make build-api      # build API only
make build-cron     # build cron job only
make build-admin    # build admin server only

make up             # start full stack (docker compose up, foreground)
make up-d           # start full stack (docker compose up, detached)
make up-infra       # start only postgres + redis + qdrant
make up-app         # start only api + cron + admin (uses docker-compose.app.yml)
make down           # stop stack
make logs           # tail api container logs
make logs-cron      # tail cron container logs
make logs-admin     # tail admin container logs

make dev            # live-reload API using air
make dev-admin      # run web/admin frontend (Vite dev server)
make dev-all        # run api (air) + admin server + web/admin frontend together
make run            # run API directly (requires infra already up)
make run-cron       # run cron job once (requires infra already up)
make run-admin      # run admin server directly (requires infra already up)

make test                                  # run all tests across go.work modules
make test-pkg PKG=./internal/ingest/...    # test a specific package
make test-verbose                          # test with verbose output
make test-race                             # run tests with -race detector

make test-e2e         # build binaries and run e2e suite (./e2e/..., -tags=e2e)
make test-e2e-api     # e2e subset that exercises only cmd/api
make test-e2e-heavy   # e2e subset for ingest + cron + recompute flows

make coverage             # generate ./tmp/coverage/unit.out and print total
make coverage-html        # open HTML coverage report
make coverage-check-all   # enforce per-package and total coverage minimums (used by CI)

make lint           # run golangci-lint over every go.work module
make fmt            # auto-format imports across every go.work module

make migrate-up     # run all pending migrations
make migrate-down   # roll back 1 migration
make migrate-version  # show current migration version
make migrate-create NAME=add_indexes  # create new migration files

make clean          # delete ./tmp/
```

Live reload in development uses `air` (configured in `.air.toml`), auto-rebuilds `cmd/api` on Go file changes. The admin frontend lives at [web/admin/](web/admin/) (Vite + React 19 + Tailwind v4) and is embedded into the `cmd/admin` binary at build time.

## Architecture

### Three Binaries

- **`cmd/api`** — HTTP API server (port **2001**) + Redis Streams ingest worker goroutine
- **`cmd/cron`** — Batch job daemon that recomputes sparse and dense vectors plus trending data on a configurable interval (default: 5 min)
- **`cmd/admin`** — Admin server (port **2002**) that serves the embedded `web/admin` SPA, the session-cookie auth API, and the `/api/admin/v1/*` operational dashboard endpoints

### Go Workspace

The repo is a Go workspace ([go.work](go.work)) with four modules; lint/test/coverage targets iterate over every module:

| Module                   | Purpose                                                                              |
| ------------------------ | ------------------------------------------------------------------------------------ |
| `.`                      | Server application — all three binaries, all `internal/` domains, e2e suite          |
| `./pkg/codohuetypes`     | Shared wire types so SDK consumers do not pull in pgx/qdrant/prometheus dependencies |
| `./sdk/go`               | Public Go SDK for clients embedding Codohue                                          |
| `./sdk/go/redistream`    | Redis Streams transport helper for the SDK                                           |

### Data Flow

```
Main Backend → HTTP POST /v1/namespaces/{ns}/events ──┐
                                                       │
Main Backend → Redis Streams ──────────────────────────┤
                                                       ▼
                                               Ingest Worker → PostgreSQL (events table)
                                                               ↓ (every N min)
                                                       Compute Job (cron binary)
                                                               ↓
                                                       Qdrant Collections (sparse vectors)
                                                               ↓
                                               Recommend Service → Main Backend
```

### Domain Organization

Each feature domain lives in `internal/<domain>/` with a consistent `handler.go`, `service.go`, `repository.go`, `types.go` structure:

| Domain            | Responsibility                                                                                     |
| ----------------- | -------------------------------------------------------------------------------------------------- |
| `ingest`          | HTTP and Redis Streams event ingestion — validates events, stores to `events` table                |
| `compute`         | Batch recomputes sparse + dense vectors with time decay, upserts to Qdrant; logs to `batch_run_logs` |
| `recommend`       | Serves CF recommendations, hybrid dense/sparse, trending, rank, BYOE embeddings, object deletion   |
| `nsconfig`        | CRUD for per-namespace configuration (action weights, decay params, dense hybrid config)           |
| `admin`           | Handlers, services, and repositories for the `cmd/admin` operational dashboard                     |
| `auth`            | Bearer token validation — global admin key and per-namespace bcrypt-hashed keys                    |
| `config`          | Loads and validates application configuration from environment variables                           |
| `core/namespace`  | Shared namespace configuration contracts (`namespace.Config`) consumed by every domain             |
| `core/idmap`      | Maps string IDs → numeric Qdrant point IDs via `id_mappings` table                                 |
| `core/httpapi`    | Shared JSON HTTP response helpers and middleware                                                   |
| `architecture`    | Repository architecture tests — enforces the import rule below                                     |

**Import rule** (enforced by [internal/architecture/imports_test.go](internal/architecture/imports_test.go)): packages under `internal/` may only import `internal/config`, `internal/core/...`, and `internal/infra/...`. Peer-domain imports are forbidden. Cross-domain coordination happens through `cmd/api` and `cmd/admin` wiring (e.g. [cmd/admin/nsconfig_adapter.go](cmd/admin/nsconfig_adapter.go)).

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
- **Two-tier auth**: Global `RECOMMENDER_API_KEY` is used by the admin server (`cmd/admin`) for session creation and is **not** accepted by the data plane for mutations — namespace configuration lives only on the admin plane. Per-namespace bcrypt-hashed keys (stored in `namespace_configs.api_key_hash`) authenticate data-plane requests, with fallback to the global key when a namespace has no key provisioned.

### REST API — `cmd/api` (port 2001)

**Infra / ops (no auth, unversioned)**

| Method  | Path       | Description                              |
| ------- | ---------- | ---------------------------------------- |
| `GET`   | `/ping`    | Liveness probe                           |
| `GET`   | `/healthz` | Health check for postgres, redis, qdrant |
| `GET`   | `/metrics` | Prometheus metrics                       |

**Client-facing — per-namespace Bearer token (falls back to `RECOMMENDER_API_KEY`)**

Every business capability is reachable from exactly one canonical path. Legacy duplicate paths have been removed and return 404.

| Method   | Path                                                                    | Description                                                            |
| -------- | ----------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `POST`   | `/v1/namespaces/{ns}/events`                                            | Ingest a behavioral event (202 Accepted; namespace in body is ignored) |
| `POST`   | `/v1/namespaces/{ns}/catalog`                                           | Ingest raw content for catalog auto-embedding (202; only when `catalog_enabled`; embedder consumer asynchronously upserts the dense vector) |
| `GET`    | `/v1/namespaces/{ns}/subjects/{id}/recommendations`                     | CF recommendations for a subject (`?limit=&offset=`)                   |
| `POST`   | `/v1/namespaces/{ns}/rankings`                                          | Score and rank up to 500 candidate items for a subject (200, computed) |
| `GET`    | `/v1/namespaces/{ns}/trending`                                          | Trending items from Redis ZSET (`?limit=&offset=&window_hours=`)       |
| `PUT`    | `/v1/namespaces/{ns}/objects/{id}/embedding`                            | Store/replace BYOE dense vector for an object (idempotent, 204). Returns **409 Conflict** when the namespace has `catalog_enabled=true` (catalog auto-embedding is the source of truth in that mode). |
| `PUT`    | `/v1/namespaces/{ns}/subjects/{id}/embedding`                           | Store/replace BYOE dense vector for a subject (idempotent, 204). NOT guarded by catalog mode — subject vectors continue through `cmd/cron` mean-pooling regardless. |
| `DELETE` | `/v1/namespaces/{ns}/objects/{id}`                                      | Remove an object from all Qdrant collections (idempotent, 204)         |

> **Ingest transport options:** Events can be sent via the HTTP endpoint above **or** published directly to Redis Streams — the `ingest` worker consumes both paths and writes to the same `events` table. The Redis Streams transport carries `namespace` inside the payload because it has no URL path.

### REST API — `cmd/admin` (port 2002, session-cookie auth via `codohue_admin_session`)

Authentication models sessions as a resource. Login = create session; logout = delete current session.

| Method   | Path                                                                | Auth          | Description                                                             |
| -------- | ------------------------------------------------------------------- | ------------- | ----------------------------------------------------------------------- |
| `POST`   | `/api/v1/auth/sessions`                                             | none (public) | Validate `RECOMMENDER_API_KEY`; set session cookie (201 + `expires_at`) |
| `DELETE` | `/api/v1/auth/sessions/current`                                     | session       | Clear session cookie (204)                                              |
| `GET`    | `/api/admin/v1/health`                                              | session       | Proxy `GET /healthz` from `cmd/api`                                     |
| `GET`    | `/api/admin/v1/namespaces`                                          | session       | List all namespace configs from PostgreSQL (`?include=overview`)        |
| `GET`    | `/api/admin/v1/namespaces/{ns}`                                     | session       | Get single namespace config                                             |
| `PUT`    | `/api/admin/v1/namespaces/{ns}`                                     | session       | Create/update namespace; calls `nsconfig.Service` directly (200 / 201)  |
| `GET`    | `/api/admin/v1/namespaces/{ns}/catalog`                             | session       | Get catalog auto-embedding config + available strategies + backlog snapshot |
| `PUT`    | `/api/admin/v1/namespaces/{ns}/catalog`                             | session       | Enable / update / disable catalog auto-embedding for a namespace (400 on dim mismatch with both dims in body; 503 when feature unwired) |
| `GET`    | `/api/admin/v1/batch-runs`                                          | session       | Recent batch run history (`?namespace=&status=&limit=&offset=`)         |
| `GET`    | `/api/admin/v1/namespaces/{ns}/batch-runs`                          | session       | Batch run history scoped to one namespace                               |
| `POST`   | `/api/admin/v1/namespaces/{ns}/batch-runs`                          | session       | Create a new batch run (202 Accepted + `Location` header)               |
| `GET`    | `/api/admin/v1/namespaces/{ns}/qdrant`                              | session       | Points count for `{ns}_subjects/objects/subjects_dense/objects_dense`   |
| `GET`    | `/api/admin/v1/namespaces/{ns}/trending`                            | session       | Trending items + Redis TTL (`?limit=&offset=&window_hours=`)            |
| `GET`    | `/api/admin/v1/namespaces/{ns}/events`                              | session       | Paginated recent events (`?limit=&offset=&subject_id=`)                 |
| `POST`   | `/api/admin/v1/namespaces/{ns}/events`                              | session       | Inject a test event (proxied to `cmd/api`, 202)                         |
| `GET`    | `/api/admin/v1/namespaces/{ns}/subjects/{id}/profile`               | session       | Subject profile: interaction count, seen items, sparse vector NNZ       |
| `GET`    | `/api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations`       | session       | Recommendations for a subject (`?limit=&offset=&debug=`)                |
| `POST`   | `/api/admin/v1/demo-data`                                           | session       | Seed the bundled demo dataset (202)                                     |
| `DELETE` | `/api/admin/v1/demo-data`                                           | session       | Clear the bundled demo dataset (204)                                    |

### Database Schema

Migrations live in `migrations/` as `NNN_name.up.sql` / `NNN_name.down.sql` pairs. Core tables from `001_initial.up.sql`:

- `namespace_configs` — per-namespace config (action weights, decay params, dense hybrid settings, `api_key_hash`)
- `events` — raw behavioral events; indexed on `(namespace, subject_id)` and `occurred_at`
- `id_mappings` — string ID → BIGSERIAL numeric ID, scoped by `(namespace, entity_type)`

Key columns added by later migrations:

- **002** — `gamma` on namespace_configs (γ-based object freshness rerank)
- **003** — `seen_items_days` on namespace_configs (recency window for the seen-items filter)
- **004** — `object_created_at` on events
- **005** — `api_key_hash`, `alpha`, `dense_strategy`, `embedding_dim`, `trending_*` on namespace_configs
- **006** — `batch_run_logs` table (per-run history written by `cmd/cron` and surfaced by the admin API)
- **007** — phase breakdown columns on `batch_run_logs` (`phase{1,2,3}_ok` / `_duration_ms` / `_subjects` / `_objects` / `_error`)
- **008** — `trigger_source` on `batch_run_logs` (e.g. `cron`, `admin`)
- **009** — `log_lines` JSONB on `batch_run_logs` (captured slog output for run inspection)

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
API_PORT=2001     # cmd/api listen port
ADMIN_PORT=2002   # cmd/admin listen port
API_URL=http://localhost:2001  # used by cmd/admin to proxy /healthz and inject test events
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

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan
at specs/004-catalog-embedder/plan.md
<!-- SPECKIT END -->
