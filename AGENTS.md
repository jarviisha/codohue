# Repository Guidelines

## Project Overview
Codohue is a hybrid recommendation service for behavioral personalization. It ingests events from Redis Streams, stores raw events in PostgreSQL, computes sparse and dense vectors, and serves recommendations through an HTTP API backed by Qdrant.

There are two binaries:

- `cmd/api`: HTTP server on port `2001` and Redis Streams ingest worker.
- `cmd/cron`: batch recompute job that rebuilds vectors on a schedule.

## Project Structure & Module Organization
The repository follows a strict Go service layout:

- `cmd/api`, `cmd/cron`: executable entrypoints.
- `internal/ingest`: consume events and persist them.
- `internal/compute`: recompute CF sparse vectors, dense vectors, and trending data.
- `internal/recommend`: recommendation, ranking, hybrid merge, trending fallback, BYOE embedding endpoints.
- `internal/nsconfig`: per-namespace config CRUD.
- `internal/core/idmap`: map string IDs to numeric Qdrant point IDs.
- `internal/auth`: bearer-token authentication.
- `internal/infra/{postgres,redis,qdrant,metrics}`: external clients and observability.
- `migrations/`: SQL migrations as `NNN_name.up.sql` and `NNN_name.down.sql`.
- `e2e/`: end-to-end tests with the `e2e` build tag.
- `docs/`: architecture and client integration references.

Keep feature logic inside its domain package. The default package shape is `docs.go`, `types.go`, `repository.go`, `service.go`, `handler.go`, plus matching tests.

## Architecture Rules
- Domain packages may import `core`, `infra`, and `config`; they should not depend on each other directly.
- Current exception: `internal/recommend` may use `internal/nsconfig` for namespace config lookup.
- Every package must have a `docs.go` file containing the package doc comment and package declaration only.
- Use PostgreSQL for durable events/config, Redis for streams/cache/trending, and Qdrant for vector collections such as `{ns}_subjects`, `{ns}_objects`, `{ns}_subjects_dense`, and `{ns}_objects_dense`.

## Build, Test, and Development Commands
Use the `Makefile` as the source of truth:

- `make build`: build both binaries into `tmp/`.
- `make build-api`, `make build-cron`: build a single binary.
- `make run`: run the API directly.
- `make run-cron`: execute one cron cycle manually.
- `make dev`: run live reload with `air`.
- `make up-infra`: start PostgreSQL, Redis, and Qdrant only.
- `make up-d`: start the full Docker stack.
- `make test`: run all unit and package tests.
- `make test-pkg PKG=./internal/ingest/...`: test one package tree.
- `make test-race`: run race detection and coverage.
- `make test-e2e`: run end-to-end tests after `make up-infra && make migrate-up`.
- `make migrate-up`, `make migrate-down`, `make migrate-version`, `make migrate-create NAME=add_indexes`: manage schema changes.

## Coding Style & Naming Conventions
- Target Go `1.26.1` as defined in `go.mod`.
- Format code with standard Go formatting; lint with `golangci-lint run ./...`.
- Keep package names lowercase and concise.
- Export identifiers only when they are needed outside the package.
- Write all code comments, doc comments, and TODOs in English.
- Prefer explicit, domain-based naming such as `NamespaceConfig`, `GetPopularItems`, `CountInteractions`.

## Testing Guidelines
- Every business-logic file such as `service.go`, `repository.go`, `job.go`, or `worker.go` should have a corresponding `_test.go`.
- `handler_test.go` should cover request parsing, auth, and response contracts.
- `types.go` and `docs.go` do not require dedicated tests.
- Prefer deterministic unit tests with fakes and small fixtures.
- Run `make test` and `make test-race` before submitting changes.
- Run `make test-e2e` whenever you touch API behavior, migrations, Redis/Qdrant integration, or cron/compute flows.

## API, Data, and Config Notes
- Namespace config is created via `PUT /v1/config/namespaces/{namespace}` using `RECOMMENDER_API_KEY`.
- All non-admin routes use the namespace key returned once at namespace creation.
- Recommendation sources include `collaborative_filtering`, `hybrid`, `hybrid_cold`, `fallback_popular`, and `hybrid_rank`.
- Local development expects `.env` values such as `DATABASE_URL`, `REDIS_URL`, `QDRANT_HOST`, `QDRANT_PORT`, `RECOMMENDER_API_KEY`, and `BATCH_INTERVAL_MINUTES`.
- Do not commit secrets or plaintext namespace keys.

## Commit & Pull Request Guidelines
Use Conventional Commits for all new commits:

- Format: `type(scope): summary`
- Write the summary in imperative mood and keep it concise.
- Prefer lowercase in `type`, `scope`, and summary except for proper nouns, API names, and versions.
- Pick a single primary scope that best describes the changed area.
- Keep commits focused so the message describes one logical change.

Recommended commit types:

- `feat`: new user-facing or developer-facing behavior
- `fix`: bug fixes and regression fixes
- `refactor`: internal restructuring without behavior change
- `test`: test-only additions or adjustments
- `docs`: documentation-only changes
- `ci`: CI workflow, build pipeline, or automation changes
- `chore`: maintenance changes that do not fit the categories above

Recommended scopes for this repo:

- `api`, `cron`
- `ingest`, `compute`, `recommend`, `nsconfig`, `auth`
- `idmap`, `qdrant`, `redis`, `postgres`, `metrics`
- `e2e`, `docs`, `ci`

Examples:

- `feat(api): add namespace-scoped client routes`
- `fix(recommend): return stable JSON error responses`
- `test(e2e): cover HTTP ingest and canonical client routes`
- `docs(readme): document namespace-path API endpoints`
- `ci(e2e): build cron binary before integration tests`

Avoid mixing older styles such as `scope: summary` with Conventional Commits in new history. In pull requests:

- describe the behavior change and affected modules,
- call out migration or config changes explicitly,
- include sample requests or responses for API changes,
- note required rollout steps if Redis, Qdrant, or cron behavior changes.
