# Copilot Instructions for Codohue

## Build, test, lint, and run commands

Use the Makefile targets unless you need a one-off `go test` invocation:

```bash
# Build
make build
make build-api
make build-cron
make build-admin
make build-embedder

# Run
make up-infra      # postgres + redis + qdrant only
make run           # run API directly
make run-cron      # run cron batch worker once
make dev           # API live reload via air

# Quality
make lint
make fmt
make test
make test-verbose
make coverage-unit
make coverage-check-all
make test-pkg PKG=./internal/ingest/...

# Single test (by name)
go test ./internal/recommend -run TestService_Recommend -v
```

Migrations are SQL files in `migrations/`; CI applies `001_initial.up.sql` through `004_object_created_at.up.sql` before tests.

## High-level architecture

Codohue is a hybrid recommendation service with four binaries:

1. `cmd/api`: serves the data-plane HTTP API and runs event and catalog Redis Streams ingest workers.
2. `cmd/cron`: runs scheduled sparse, dense, and trending recompute jobs.
3. `cmd/admin`: serves the admin API and embedded `web/admin` application.
4. `cmd/embedder`: consumes catalog embed streams and upserts catalog-owned dense vectors.

Data flow:

1. Main backend publishes events to Redis stream `codohue:events` (field `payload` JSON).
2. Ingest worker validates/parses events and stores them in PostgreSQL `events`.
3. Compute job reads active namespaces and performs full recompute for recent data (no incremental merge strategy), ensuring `{namespace}_subjects` and `{namespace}_objects` collections exist in Qdrant.
4. Recommendation service chooses strategy by interaction count:
   - `0` interactions: `fallback_popular`
   - `< 5` interactions: `hybrid_cold` (blend popular + CF)
   - otherwise: `collaborative_filtering`
5. CF path fetches subject sparse vector from Qdrant, excludes recently seen items, queries object vectors, then re-ranks with gamma freshness decay.
6. Recommendation responses are cached in Redis for 5 minutes (`rec:{namespace}:{subject_id}:limit={limit}`).

Core storage model:

- `id_mappings` maps string IDs to numeric Qdrant point IDs (do not use ad-hoc hashing for point IDs).
- `namespace_configs` controls behavior weights and ranking knobs (`lambda`, `gamma`, `max_results`, `seen_items_days`).

## Key repository conventions

1. Domain packages follow `handler.go`, `service.go`, `repository.go`, `types.go` layout under `internal/<domain>/`.
2. Domain import boundary is strict: domains may import `core/`, `infra/`, and `config/`, but not other domains directly.
3. Every package must include `docs.go` with the package doc comment and `package` declaration only.
4. All comments in Go files must be English.
5. Business-logic files (`service.go`, `repository.go`, `job.go`, `worker.go`) require matching `_test.go` files; handler tests belong in `handler_test.go`.
6. Application routes use global-admin/session or per-namespace authority as documented; protected operational monitoring uses a distinct observability bearer credential.
