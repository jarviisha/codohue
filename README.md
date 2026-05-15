# Codohue

Codohue is a hybrid recommendation service for behavioral personalization.

It ingests events from Redis Streams, stores raw events in PostgreSQL, computes sparse and dense signals, and serves recommendations through an HTTP API backed by Qdrant.

## What It Does

- Consumes behavioral events from the Redis Stream `codohue:events`
- Persists normalized events in PostgreSQL
- Computes sparse collaborative filtering signals
- Computes dense vectors with `svd`, `item2vec`, or accepts external embeddings with `byoe`
- Stores vectors in Qdrant collections per namespace
- Serves recommendations, ranking, trending, and embedding management over HTTP

## Binaries

The project ships two executables:

- `cmd/api`: HTTP API on port `2001` and the Redis Streams ingest worker
- `cmd/cron`: scheduled batch recompute job for vectors and trending data

## Modules

This repository contains the server application plus Go modules used by
external clients:

- root module `github.com/jarviisha/codohue`: the server application and
  internal packages
- `github.com/jarviisha/codohue/pkg/codohuetypes`: shared wire types for HTTP
  and Redis Streams payloads
- `github.com/jarviisha/codohue/sdk/go`: HTTP client SDK for the data-plane API
- `github.com/jarviisha/codohue/sdk/go/redistream`: Redis Streams producer SDK

See [sdk/go/README.md](./sdk/go/README.md) for the HTTP client and
`sdk/go/redistream` usage.

## Architecture

Core runtime components:

- PostgreSQL: durable storage for events, namespace config, and ID mappings
- Redis: event ingestion via Streams and cache/trending storage
- Qdrant: sparse and dense vector collections

Per namespace, the system uses Qdrant collections such as:

- `{ns}_subjects`
- `{ns}_objects`
- `{ns}_subjects_dense`
- `{ns}_objects_dense`

Main internal packages:

- `internal/ingest`: Redis Streams consumer and event persistence
- `internal/compute`: sparse recompute, dense vectors, and trending jobs
- `internal/recommend`: recommendation, ranking, hybrid merge, fallback, and BYOE endpoints
- `internal/nsconfig`: namespace configuration CRUD
- `internal/core/idmap`: string ID to numeric point ID mapping
- `internal/infra/{postgres,redis,qdrant,metrics}`: infrastructure adapters and observability

## Requirements

- Go `1.26.1`
- Docker and Docker Compose for local infrastructure
- `golangci-lint` if you want to run lint or format targets
- `air` if you want to use `make dev`
- `migrate` if you want to run database migrations from the host

SDK note:

- the server application tracks Go `1.26.1`
- the SDK-related modules (`pkg/codohuetypes`, `sdk/go`,
  `sdk/go/redistream`) currently target Go `1.24.13` for broader downstream
  adoption

## Configuration

Codohue reads configuration from environment variables and also loads `.env` automatically when present.

Required:

- `DATABASE_URL`
- `CODOHUE_ADMIN_API_KEY`

Common variables:

- `REDIS_URL` default: `redis://localhost:6379`
- `QDRANT_HOST` default: `localhost`
- `QDRANT_PORT` default: `6334`
- `CODOHUE_API_PORT` default: `2001`
- `CODOHUE_BATCH_INTERVAL_MINUTES` default: `5`
- `CODOHUE_LOG_FORMAT` default: `text`

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
```

## Quick Start

The project ships two Docker Compose files:

- `docker-compose.yml` — local development; builds images from source and runs the API binary
- `docker-compose.app.yml` — app-only development; builds API and cron, connects to externally managed Postgres, Redis, and Qdrant
- `docker-compose.prod.yml` — production; pulls prebuilt images from GHCR, no source mount

### Option 1: Full stack with Docker Compose (development)

**1. Copy and review environment variables**

```bash
cp .env.example .env
```

The default values in `.env.example` match what `docker-compose.yml` already injects, so no edits are required for a local first run.

**2. Start the full stack in the background**

```bash
make up-d
```

This starts:

| Container          | Image                    | Exposed ports                    |
| ------------------ | ------------------------ | -------------------------------- |
| `codohue-api`      | built from source        | `2001` (HTTP API)                |
| `codohue-postgres` | `postgres:16-alpine`     | `5432`                           |
| `codohue-redis`    | `redis:7-alpine`         | `6379`                           |
| `codohue-qdrant`   | `qdrant/qdrant:v1.17.1`  | `6333` (HTTP), `6334` (gRPC)     |

**3. Apply database migrations**

```bash
make migrate-up
```

Migrations must be applied manually on first run (and after any schema updates). The `postgres` container starts healthy before the API, so migrations can be run immediately after `make up-d`.

**4. Verify the stack**

```bash
curl http://localhost:2001/healthz
```

**Useful commands**

```bash
make logs          # tail API logs
make logs-cron     # tail cron logs
make down          # stop and remove containers
make down-v        # stop and remove containers + all local volumes (full reset)
```

---

### Option 2: Run the API locally against Docker infra

Use this when you want fast iteration on the Go source without rebuilding the Docker image.

**1. Start only the infrastructure services**

```bash
make up-infra
```

**2. Apply migrations**

```bash
make migrate-up
```

**3. Run the API or cron**

```bash
make run           # start the API
make run-cron      # run one cron cycle
make dev           # start the API with live reload (requires air)
```

---

### Option 3: API and cron only with external infrastructure

Use this when Postgres, Redis, and Qdrant are managed outside this compose stack. The app-only compose file reads the same `.env` variables as the application and uses host networking, so `localhost` means the Docker host.

```env
DATABASE_URL=postgres://user:password@localhost:5432/codohue?sslmode=disable
REDIS_URL=redis://localhost:6379
QDRANT_HOST=localhost
QDRANT_PORT=6334
CODOHUE_ADMIN_API_KEY=dev-secret-key
CODOHUE_BATCH_INTERVAL_MINUTES=5
CODOHUE_LOG_FORMAT=text
CODOHUE_API_PORT=2001
```

Start only `api` and `cron`:

```bash
make up-app-d
```

Stop them:

```bash
make down-app
```

If your infrastructure is on another host or network, set `DATABASE_URL`, `REDIS_URL`, and `QDRANT_HOST` to those reachable addresses instead of `localhost`.

---

### Option 4: Production deployment with Docker Compose

`docker-compose.prod.yml` pulls prebuilt images from GHCR (`ghcr.io/jarviisha/codohue/api:latest` and `ghcr.io/jarviisha/codohue/cron:latest`). It does **not** include a Postgres container — you must supply an external database.

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

- `api` and `cron` containers use `restart: unless-stopped`
- Qdrant and Redis gRPC/HTTP ports are bound to `127.0.0.1` only (not exposed to the network)
- Redis uses `maxmemory 256mb` with `allkeys-lru` eviction
- Log format defaults to `json`

Run migrations against the production database before starting (or as a pre-deploy step):

```bash
migrate -path migrations -database "$DATABASE_URL" up
```

## Namespace Setup

Before calling namespace-scoped endpoints, create a namespace configuration through the admin server with the admin API key from `CODOHUE_ADMIN_API_KEY`.

Create an admin session:

```bash
curl -c cookies.txt -X POST http://localhost:2002/api/v1/auth/sessions \
  -H "Content-Type: application/json" \
  -d '{"api_key":"dev-secret-key"}'
```

Then upsert the namespace:

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

On initial creation, the response returns a plaintext namespace API key once. Keep it safe and do not commit it.

## Event Ingestion

Codohue ingests events from the Redis Stream `codohue:events`. Each stream message must contain a `payload` field with a JSON document.

Example payload:

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

Example publish:

```bash
redis-cli XADD codohue:events * payload '{"namespace":"demo","subject_id":"user-123","object_id":"item-456","action":"VIEW","timestamp":"2026-04-19T10:00:00Z"}'
```

For client integrations that do not want direct Redis access, Codohue also exposes:

```bash
POST /v1/namespaces/{ns}/events
```

with `occurred_at` instead of the Redis payload's `timestamp`, and without `namespace`; the namespace is supplied by the URL path.

Default built-in actions in the ingest layer:

- `VIEW`
- `LIKE`
- `COMMENT`
- `SHARE`
- `SKIP`

These are the built-in fallback actions with hardcoded default weights. The system is not limited to only these values: custom action strings are also accepted when the namespace config defines a matching entry in `action_weights`. If an action is neither configured in `action_weights` nor present in the built-in defaults, ingest returns an `unknown action` error.

## HTTP API

Health and diagnostics:

- `GET /ping`
- `GET /healthz`
- `GET /metrics`

Admin routes:

- `POST /api/v1/auth/sessions`
- `DELETE /api/v1/auth/sessions/current`
- `GET /api/admin/v1/health`
- `GET /api/admin/v1/namespaces`
- `GET /api/admin/v1/namespaces/{ns}`
- `PUT /api/admin/v1/namespaces/{ns}`
- `GET /api/admin/v1/batch-runs`
- `GET /api/admin/v1/namespaces/{ns}/batch-runs`
- `POST /api/admin/v1/namespaces/{ns}/batch-runs`
- `GET /api/admin/v1/namespaces/{ns}/qdrant`
- `GET /api/admin/v1/namespaces/{ns}/trending`
- `GET /api/admin/v1/namespaces/{ns}/events`
- `POST /api/admin/v1/namespaces/{ns}/events`
- `GET /api/admin/v1/namespaces/{ns}/subjects/{id}/profile`
- `GET /api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations`
- `POST /api/admin/v1/demo-data`
- `DELETE /api/admin/v1/demo-data`

Namespace routes:

- `POST /v1/namespaces/{ns}/events`
- `GET /v1/namespaces/{ns}/subjects/{id}/recommendations`
- `POST /v1/namespaces/{ns}/rankings`
- `GET /v1/namespaces/{ns}/trending`
- `PUT /v1/namespaces/{ns}/objects/{id}/embedding`
- `PUT /v1/namespaces/{ns}/subjects/{id}/embedding`
- `DELETE /v1/namespaces/{ns}/objects/{id}`

Authentication model:

- admin routes use a session cookie created with `CODOHUE_ADMIN_API_KEY`
- namespace routes use the per-namespace API key returned when the namespace is created
- `GET /ping` and `GET /healthz` are public

Error responses use a stable JSON shape:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "invalid request body"
  }
}
```

Recommendation sources returned by the service include:

- `collaborative_filtering`
- `hybrid`
- `hybrid_cold`
- `fallback_popular`
- `hybrid_rank`

## Development Commands

The `Makefile` is the source of truth:

```bash
make build
make build-api
make build-cron
make run
make run-cron
make dev
make lint
make fmt
make test
make test-race
make test-e2e
make migrate-up
make migrate-down
make migrate-version
make migrate-create NAME=add_indexes
```

Examples:

```bash
make test-pkg PKG=./internal/ingest/...
make coverage-unit
make coverage-check-all
```

Build outputs are written to `tmp/`.

## Testing

Available test layers:

- Unit and package tests for isolated package behavior
- API E2E tests for HTTP contracts and auth flows
- Integration-heavy E2E tests for Redis Streams ingest, cron recompute, and hybrid/computed recommendation paths

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
```

The E2E suite launches the API binary on port `12001`. The full suite also executes the cron binary and validates recommendation, ranking, trending, health, config, embedding, Redis Streams ingest, recompute, and hybrid flows.

## Project Layout

```text
cmd/api                  HTTP API + ingest worker
cmd/cron                 batch recompute job
internal/ingest          Redis Streams consumer and persistence
internal/compute         recompute pipeline, dense vectors, trending
internal/recommend       recommendation and ranking API
internal/nsconfig        namespace configuration CRUD
internal/core/idmap      string ID <-> numeric Qdrant point mapping
internal/infra/*         external service clients and metrics
migrations/              SQL migrations
e2e/                     end-to-end tests
```

## Deployment Notes

- `docker-compose.yml` is aimed at local development
- `docker-compose.prod.yml` runs the prebuilt `api` and `cron` images
- The production compose file expects `CODOHUE_DATABASE_URL` and `CODOHUE_ADMIN_API_KEY` from the environment

## Notes

- Namespace keys are only returned in plaintext on first namespace creation
- Do not commit secrets, `.env` files, or plaintext namespace API keys
- Redis and Qdrant state can influence local behavior, so use `make down-v` when you need a full reset
