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
- `RECOMMENDER_API_KEY`

Common variables:

- `REDIS_URL` default: `redis://localhost:6379`
- `QDRANT_HOST` default: `localhost`
- `QDRANT_PORT` default: `6334`
- `API_PORT` default: `2001`
- `BATCH_INTERVAL_MINUTES` default: `5`
- `LOG_FORMAT` default: `text`

Example `.env`:

```env
DATABASE_URL=postgres://codohue:secret@localhost:5432/codohue?sslmode=disable
REDIS_URL=redis://localhost:6379
QDRANT_HOST=localhost
QDRANT_PORT=6334
RECOMMENDER_API_KEY=dev-secret-key
BATCH_INTERVAL_MINUTES=5
LOG_FORMAT=text
API_PORT=2001
```

## Quick Start

### Option 1: Run with Docker Compose

Start the full stack:

```bash
make up-d
```

The API becomes available at `http://localhost:2001`.

Useful follow-up commands:

```bash
make logs
make logs-cron
make down
```

To reset local volumes:

```bash
make down-v
```

### Option 2: Run the API locally against Docker infra

Start only infrastructure:

```bash
make up-infra
```

Apply migrations:

```bash
make migrate-up
```

Run the API:

```bash
make run
```

Run one cron cycle manually:

```bash
make run-cron
```

For live reload:

```bash
make dev
```

## Namespace Setup

Before calling namespace-scoped endpoints, create a namespace configuration with the admin API key from `RECOMMENDER_API_KEY`.

```bash
curl -X PUT http://localhost:2001/v1/config/namespaces/demo \
  -H "Authorization: Bearer dev-secret-key" \
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

with the same JSON payload shape.

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

Admin route:

- `PUT /v1/config/namespaces/{namespace}`

Namespace routes:

- `GET /v1/recommendations?subject_id=...&namespace=...`
- `POST /v1/rank`
- `GET /v1/trending/{ns}`
- `POST /v1/objects/{ns}/{id}/embedding`
- `POST /v1/subjects/{ns}/{id}/embedding`
- `DELETE /v1/objects/{ns}/{id}`

Canonical namespace-path routes for new clients:

- `POST /v1/namespaces/{ns}/events`
- `GET /v1/namespaces/{ns}/recommendations?subject_id=...`
- `POST /v1/namespaces/{ns}/rank`
- `GET /v1/namespaces/{ns}/trending`
- `POST /v1/namespaces/{ns}/objects/{id}/embedding`
- `POST /v1/namespaces/{ns}/subjects/{id}/embedding`
- `DELETE /v1/namespaces/{ns}/objects/{id}`

Authentication model:

- `PUT /v1/config/namespaces/{namespace}` uses the global admin key from `RECOMMENDER_API_KEY`
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
- The production compose file expects `DATABASE_URL` and `RECOMMENDER_API_KEY` from the environment

## Notes

- Namespace keys are only returned in plaintext on first namespace creation
- Do not commit secrets, `.env` files, or plaintext namespace API keys
- Redis and Qdrant state can influence local behavior, so use `make down-v` when you need a full reset
