# Codohue

Codohue is a hybrid (sparse + dense) collaborative-filtering recommendation service for behavioral personalization.

It ingests events over HTTP and Redis Streams, persists them in PostgreSQL, recomputes sparse/dense vectors on a schedule, optionally auto-embeds raw catalog content, and serves recommendations through HTTP APIs backed by Qdrant.

> **Architecture details, data model, API surface, design decisions:** see [ARCHITECTURE.md](ARCHITECTURE.md).
> **Contributor conventions:** see [AGENTS.md](AGENTS.md).
> **Go SDK:** see [sdk/go/README.md](sdk/go/README.md).

## Highlights

- Ingest events via `POST /v1/namespaces/{ns}/events` or Redis Stream `codohue:events`
- Sparse CF + dense (`item2vec` / `svd` / `byoe` / `disabled`) blended at serve time
- Optional auto-embedding of raw catalog content per namespace
- Time-decayed trending ZSET in Redis; 5-minute recommendation cache
- Multi-tenant: each namespace owns its config, Qdrant collections, Redis streams, and API key
- Operational SPA on port `2002` (session-cookie auth)

## Binaries

| Binary           | Port  | Purpose |
| ---------------- | ----- | ------- |
| `cmd/api`        | 2001  | Data-plane HTTP API + Redis Streams ingest worker |
| `cmd/cron`       | —     | Batch daemon: sparse + dense + trending recompute |
| `cmd/admin`      | 2002  | Admin server: `/api/admin/v1/*` + embedded SPA |
| `cmd/embedder`   | 2003  | Catalog auto-embedding worker |

## Requirements

- Go `1.26.1` (server). SDK modules (`pkg/codohuetypes`, `sdk/go`, `sdk/go/redistream`) target Go `1.24.13`.
- Docker + Docker Compose for local infra
- `golangci-lint` (for `make lint`, `make fmt`)
- `air` (for `make dev`)
- `migrate` (for host-side `make migrate-*`)
- Node.js 20+ and `npm` (for `web/admin`)

## Quickstart — full stack with Docker Compose

```bash
cp .env.example .env
make up-d
```

This starts postgres + redis + qdrant + migrations + the four app containers. Defaults in `.env.example` match what `docker-compose.yml` injects, so no edits are needed for a first run.

Verify:

```bash
curl http://localhost:2001/healthz
open  http://localhost:2002       # admin SPA login (uses CODOHUE_ADMIN_API_KEY)
```

Other compose layouts:

| File | Purpose |
| ---- | ------- |
| [docker-compose.yml](docker-compose.yml)         | Dev — builds from source, infra included, auto-migrate |
| [docker-compose.app.yml](docker-compose.app.yml) | App-only — builds from source, host networking to external infra |
| [docker-compose.prod.yml](docker-compose.prod.yml) | Prod — prebuilt GHCR images; supply external DB via `CODOHUE_DATABASE_URL` + `CODOHUE_ADMIN_API_KEY` |

## Quickstart — binaries against Docker infra

For fast Go iteration without rebuilding images:

```bash
make up-infra        # postgres + redis + qdrant only
make migrate-up      # apply migrations from host
make run             # cmd/api (or run-cron / run-admin / run-embedder)
make dev             # cmd/api with live reload (air)
make dev-all         # api (air) + admin + web/admin Vite together
```

## Configuration

Codohue loads `.env` automatically when present. Required: `DATABASE_URL`, `CODOHUE_ADMIN_API_KEY`.

| Variable                            | Default                  | Used by |
| ----------------------------------- | ------------------------ | ------- |
| `REDIS_URL`                         | `redis://localhost:6379` | all |
| `QDRANT_HOST` / `QDRANT_PORT`       | `localhost` / `6334`     | all |
| `CODOHUE_API_PORT`                  | `2001`                   | `cmd/api` |
| `CODOHUE_ADMIN_PORT`                | `2002`                   | `cmd/admin` |
| `CODOHUE_API_URL`                   | `http://localhost:2001`  | `cmd/admin` (proxy `/healthz`, inject events) |
| `CODOHUE_BATCH_INTERVAL_MINUTES`    | `5`                      | `cmd/cron` |
| `CODOHUE_LOG_FORMAT`                | `text`                   | all (`text` or `json`) |
| `CODOHUE_CATALOG_MAX_CONTENT_BYTES` | `32768`                  | `cmd/embedder` (override per-ns via admin API) |
| `CODOHUE_EMBED_MAX_ATTEMPTS`        | `5`                      | `cmd/embedder` retries before dead-letter |
| `CODOHUE_EMBEDDER_HEALTH_PORT`      | `2003`                   | `cmd/embedder` |
| `CODOHUE_EMBEDDER_REPLICA_NAME`     | hostname                 | `cmd/embedder` consumer name |
| `CODOHUE_EMBEDDER_POLL_INTERVAL`    | `30s`                    | `cmd/embedder` rescan cadence |

## Creating a namespace

The admin plane is the only place namespaces are configured. Login via the SPA, or with curl:

```bash
# 1. Create session cookie
curl -c cookies.txt -X POST http://localhost:2002/api/v1/auth/sessions \
  -H "Content-Type: application/json" \
  -d '{"api_key":"dev-secret-key"}'

# 2. Upsert namespace
curl -b cookies.txt -X PUT http://localhost:2002/api/admin/v1/namespaces/demo \
  -H "Content-Type: application/json" \
  -d '{
    "action_weights": {"VIEW": 1, "LIKE": 5, "SHARE": 10},
    "lambda": 0.01, "gamma": 0.5,
    "max_results": 20, "seen_items_days": 14,
    "alpha": 0.7,
    "dense_strategy": "byoe", "embedding_dim": 4, "dense_distance": "cosine",
    "trending_window": 24, "trending_ttl": 600, "lambda_trending": 0.1
  }'
```

The response returns a **plaintext namespace API key once** — only the bcrypt hash is stored. Data-plane calls send it as `Authorization: Bearer <key>`. The global admin key is accepted as fallback only when a namespace has no provisioned key.

## Sending events

```bash
# HTTP
curl -X POST http://localhost:2001/v1/namespaces/demo/events \
  -H "Authorization: Bearer <namespace-key>" \
  -H "Content-Type: application/json" \
  -d '{"subject_id":"user-123","object_id":"item-456","action":"VIEW","occurred_at":"2026-04-19T10:00:00Z"}'

# Redis Streams (no URL → namespace lives in the payload)
redis-cli XADD codohue:events '*' payload \
  '{"namespace":"demo","subject_id":"user-123","object_id":"item-456","action":"VIEW","timestamp":"2026-04-19T10:00:00Z"}'
```

Built-in actions: `VIEW`, `LIKE`, `COMMENT`, `SHARE`, `SKIP`. Custom actions are accepted when defined in `namespace_configs.action_weights`.

## Common Make targets

[Makefile](Makefile) is the source of truth. Highlights:

```bash
# Build / run
make build                     # all four binaries to ./tmp/
make build-admin-embed         # admin binary with SPA bundled
make run / run-cron / run-admin / run-embedder
make dev / dev-admin / dev-all

# Docker
make up-d / up-infra / up-app-d
make down / down-v / down-app
make logs / logs-cron / logs-admin / logs-embedder
make compose-check             # validate every compose file

# Quality
make lint
make fmt
make test
make test-race
make test-pkg PKG=./internal/ingest/...

# Coverage
make coverage
make coverage-html
make coverage-check-all        # CI gate

# E2E (build tag `e2e`, requires infra + migrations)
make test-e2e
make test-e2e-api
make test-e2e-heavy

# Migrations
make migrate-up / migrate-down / migrate-version
make migrate-create NAME=add_indexes
```

## Testing

```bash
make test           # unit + package tests across all go.work modules
make test-race      # with -race

# E2E
make up-infra && make migrate-up && make test-e2e
```

The E2E suite launches the API binary on port `12001` and exercises HTTP contracts, Redis Streams ingest, cron recompute, hybrid recommendation, and catalog auto-embedding.

## Web admin SPA

The admin console at [web/admin/](web/admin/) (Vite + React 19 + Tailwind v4) is embedded into `cmd/admin` at build time via the `embedui` build tag.

```bash
make dev-admin            # Vite dev server (standalone)
make build-admin-embed    # production admin binary with SPA
```

`make build-admin` (no `-embed`) builds an admin binary that serves only the API.

## Notes

- Namespace keys are returned in plaintext **only once**, on creation.
- Do not commit `.env`, secrets, or plaintext namespace keys.
- `make down-v` removes containers **and** volumes — full local reset.
- Catalog ingest (`POST /v1/namespaces/{ns}/catalog`) only works when `catalog_enabled = true` on the namespace; in that mode, `PUT /v1/namespaces/{ns}/objects/{id}/embedding` returns `409 Conflict` because the catalog pipeline owns object vectors.
