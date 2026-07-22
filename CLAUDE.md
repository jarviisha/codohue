# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Codohue** is a collaborative filtering recommendation engine for the DarkVoid project. It ingests behavioral events (clicks, likes, comments, shares, skips), builds sparse vectors, and serves personalized recommendations via Qdrant vector search.

## Commands

```bash
make build          # build all four binaries (./tmp/api, ./tmp/cron, ./tmp/admin, ./tmp/embedder)
make build-api      # build API only
make build-cron     # build cron job only
make build-admin    # build admin server only
make build-embedder # build catalog embedder worker only

make up             # start full stack (docker compose up, foreground)
make up-d           # start full stack (docker compose up, detached)
make up-infra       # start only postgres + redis + qdrant
make up-app         # start only api + cron + admin + embedder (uses docker-compose.app.yml)
make down           # stop stack
make logs           # tail api container logs
make logs-cron      # tail cron container logs
make logs-admin     # tail admin container logs
make logs-embedder  # tail embedder container logs

make dev            # live-reload API using air
make dev-admin      # run web/admin frontend (Vite dev server)
make dev-embedder   # run embedder worker directly (requires infra already up)
make dev-all        # run api (air) + admin server + web/admin frontend together
make run            # run API directly (requires infra already up)
make run-cron       # run cron job once (requires infra already up)
make run-admin      # run admin server directly (requires infra already up)
make run-embedder   # run embedder worker directly (requires infra already up)

make test                                  # run all tests across go.work modules
make test-pkg PKG=./internal/ingest/...    # test a specific package
make test-verbose                          # test with verbose output
make test-race                             # run tests with -race detector

make test-e2e         # build binaries and run e2e suite (./e2e/..., -tags=e2e)
make test-e2e-api     # e2e subset that exercises only cmd/api
make test-e2e-heavy   # e2e subset for ingest + cron + recompute + catalog + admin-plane flows

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

### Four Binaries

- **`cmd/api`** — HTTP API server (port **2001**) + Redis Streams ingest worker goroutine
- **`cmd/cron`** — Batch job daemon that recomputes sparse and dense vectors plus trending data on a configurable interval (default: 5 min)
- **`cmd/admin`** — Admin server (port **2002**) that serves the embedded `web/admin` SPA, the session-cookie auth API, and the `/api/admin/v1/*` operational dashboard endpoints
- **`cmd/embedder`** — Catalog auto-embedding worker (port **2003** for `/healthz` + `/metrics`); consumes raw catalog content from per-namespace `catalog:embed:{ns}` Redis streams, embeds it via the configured `embedstrategy.Strategy`, and upserts dense vectors into `{ns}_objects_dense`. Also runs the re-embed completion watcher that closes admin-triggered batch runs when the namespace's catalog backlog drains.

### Go Workspace

The repo is a Go workspace ([go.work](go.work)) with four modules; lint/test/coverage targets iterate over every module:

| Module                   | Purpose                                                                              |
| ------------------------ | ------------------------------------------------------------------------------------ |
| `.`                      | Server application — all four binaries, all `internal/` domains, e2e suite           |
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
                                                       Qdrant Collections (sparse + dense vectors)
                                                               ↓
                                               Recommend Service → Main Backend

Main Backend → HTTP POST /v1/namespaces/{ns}/catalog ──→ catalog_items (PostgreSQL)
                                                               ↓ (XADD catalog:embed:{ns})
                                                       Embedder Worker (embedder binary)
                                                               ↓ (per-item embed → upsert)
                                                       Qdrant {ns}_objects_dense
                                                               ↓
                                               Recommend Service (hybrid dense path)
```

### Domain Organization

Each feature domain lives in `internal/<domain>/` with a consistent `handler.go`, `service.go`, `repository.go`, `types.go` structure:

| Domain            | Responsibility                                                                                     |
| ----------------- | -------------------------------------------------------------------------------------------------- |
| `ingest`               | HTTP and Redis Streams event ingestion — validates events, stores to `events` table                              |
| `compute`              | Batch recomputes sparse + dense vectors with time decay, upserts to Qdrant; logs to `batch_run_logs`              |
| `recommend`            | Serves CF recommendations, hybrid dense/sparse, trending, rank, BYOE embeddings, object deletion                  |
| `nsconfig`             | CRUD for per-namespace configuration (action weights, decay params, dense hybrid config, catalog config)          |
| `admin`                | Handlers, services, and repositories for the `cmd/admin` operational dashboard                                    |
| `catalog`              | Data-plane HTTP ingest path for raw catalog content; persists `catalog_items` and publishes to embed streams      |
| `objects`              | Per-object metadata independent of embedding (currently author attribution); the `objects` table, usable under every `dense_source` |
| `embedder`             | Per-item embed pipeline (load → embed → upsert dense vector → mark embedded), worker, re-embed completion watcher |
| `auth`                 | Bearer token validation — global admin key and per-namespace bcrypt-hashed keys                                   |
| `config`               | Loads and validates application configuration from environment variables                                          |
| `core/embedstrategy`   | Forward-compat seam for embedding strategies — `Strategy` interface + registry; both `catalog` and `embedder` depend on it (and never on each other) |
| `core/namespace`       | Shared namespace configuration contracts (`namespace.Config`) consumed by every domain                            |
| `core/idmap`           | Maps string IDs → numeric Qdrant point IDs via `id_mappings` table                                                |
| `core/httpapi`         | Shared JSON HTTP response helpers and middleware                                                                  |
| `architecture`         | Repository architecture tests — enforces the import rule below                                                    |

**Import rule** (enforced by [internal/architecture/imports_test.go](internal/architecture/imports_test.go)): packages under `internal/` may only import `internal/config`, `internal/core/...`, `internal/infra/...`, and their own subpackages (e.g. `internal/admin` may import `internal/admin/sse`). Peer-domain imports are forbidden. Cross-domain coordination happens through `cmd/api` and `cmd/admin` wiring (e.g. [cmd/admin/nsconfig_adapter.go](cmd/admin/nsconfig_adapter.go)).

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
| 2     | Dense    | Derives user embeddings by mean-pooling the dense vectors of each subject's interacted items and upserts them to `{ns}_subjects_dense`. Where the item vectors come from depends on `dense_source`: `"item2vec"` / `"svd"` train them here and also upsert to `{ns}_objects_dense`; `"catalog"` reads them back from `{ns}_objects_dense` (owned by `cmd/embedder`) and writes **only** subject vectors. Skipped for `"byoe"` and `"disabled"` |
| 3     | Trending | Computes time-decayed trending scores from recent events and caches them in Redis ZSET. Skipped when Redis is unavailable |

### Key Design Decisions

- **Full recompute strategy**: The cron job recalculates all vectors from scratch each run to avoid incremental update race conditions. No get→merge→upsert pattern. Item2Vec retrains fully each run — incremental online Word2Vec is deliberately avoided because it causes catastrophic forgetting.
- **ID mapping via DB**: String IDs map to numeric Qdrant point IDs through the `id_mappings` table (BIGSERIAL), avoiding hash collisions.
- **Sparse vectors**: `{ns}_subjects` and `{ns}_objects` Qdrant collections hold sparse interaction vectors (dot product similarity).
- **Dense source (unified)**: a single `dense_source` enum names the producer of object dense vectors — `disabled` | `item2vec` | `svd` | `byoe` | `catalog`. It replaced the old `dense_strategy` + `catalog_enabled` pair (added in 016 with a dual-write window; the legacy columns were dropped in 017). `{ns}_subjects_dense` / `{ns}_objects_dense` hold the dense embeddings. When `alpha < 1.0` and `dense_source != "disabled"`, the recommend service blends sparse CF scores (weight=`alpha`) with dense scores (weight=`1-alpha`) using min-max normalization before merging. Selecting `catalog` is how catalog auto-embedding is enabled — there is no separate boolean, so the old dense_strategy↔catalog conflict is structurally impossible.
- **`dense_source` names the producer of *object* vectors only**: `{ns}_subjects_dense` is a separate question. Nothing but `cmd/cron` phase 2 mean-pooling fills it (BYOE aside), which is why phase 2 runs under `catalog` too — otherwise the embedder would fill `{ns}_objects_dense` while `{ns}_subjects_dense` stayed empty, `fetchSubjectDenseVector` would return nil, and every request would silently fall back to pure sparse CF with the catalog embeddings never read. Under `catalog` the phase loads only the item vectors for objects that actually appear in events (`interactedObjectIDs` → `FetchItemDenseVectors`), since only those can contribute to a subject's mean.
- **Author is ownership metadata, never a behavioural link**: `objects.author_subject_id` records which subject *created* an object. It shares the id space of `events.subject_id` but has no foreign key, and the only thing that ever reads it is the opt-in `exclude_authored` filter below — never the embedder, never `cmd/cron`, and never as a scoring signal. The subject↔object relation that drives CF lives solely in `events` and is **many-to-many**: the whole signal is that many subjects touch the same object. Giving an object one owning subject would not model that relation, it would destroy it.
- **`objects` vs `catalog_items`**: `catalog_items` is embedding input and its lifecycle (content, hash, state machine, strategy). `objects` is facts about the object itself. Author started on `catalog_items` (019) and moved out in 021 because that table only exists for `dense_source="catalog"` — under `item2vec`/`svd`/`byoe` an object has no row anywhere, so attribution had no home and `exclude_authored` silently did nothing. The column was **moved, not copied**: two stores for one fact drift apart, the failure mode 016/017 removed when `dense_strategy` and `catalog_enabled` could disagree. Catalog ingest still accepts `author_subject_id` and writes it through to `objects` via an interface injected in `cmd/api`, so `internal/catalog` never imports the peer domain. Absence on a catalog re-ingest means "unspecified" and leaves the attribution alone; an explicit empty value on `PUT /objects/{id}` clears it.
- **Excluding a subject's own objects** (`exclude_authored`, default off): the exclusion is materialised as point ids and merged into the same Qdrant `MustNot` that already carries seen-items, so one filter covers the sparse (`{ns}_objects`) and dense (`{ns}_objects_dense`) collections alike. Filtering on a Qdrant payload field instead would only reach the dense collection — `cmd/cron` writes the sparse points and knows nothing about authorship — so the feature would silently do nothing in the most common configuration. The trending and popular fallbacks cannot push the filter into the store, so they over-fetch by the exclusion size and drop authored ids *before* paging; filtering after paging would make the offset count rows that are about to disappear. Cost of the id approach: the filter grows with the number of objects the requesting subject authored, capped at `authoredObjectsCap` (5000) with a warning log when it bites. A query failure degrades to unfiltered results rather than failing the request.
- **Namespace config writes are PATCH, not replace**: every field on `nsconfig.UpsertRequest` is a pointer, and nil travels all the way to SQL where `COALESCE($n, column)` keeps the current value. This is why the write is two statements (`INSERT ... ON CONFLICT DO NOTHING` to seed schema defaults for a new row, then `UPDATE ... COALESCE`) rather than one `INSERT ... ON CONFLICT DO UPDATE`: the conflict form must supply a value for every column, so a request naming one field reset all the others to their Go zero value — and the admin UI submits only the fields the operator edited. `dense_source` is named explicitly on the INSERT because the app default for a new namespace (`disabled`) differs from the schema default (`item2vec`).
- **Time decay**: Events older than 90 days excluded. Freshness multiplier `e^(-λ × days_since)` applied during vector build; γ-based object freshness applied at rerank time.
- **Cold start**: 0 interactions → Redis trending ZSET (fallback to DB popular); <5 interactions → 70% trending + 30% CF hybrid.
- **Recommendation cache**: Results cached in Redis for 5 minutes per `(namespace, subject_id, limit)` key.
- **Two-tier auth**: Global `CODOHUE_ADMIN_API_KEY` is used by the admin server (`cmd/admin`) for session creation and is **not** accepted by the data plane for mutations — namespace configuration lives only on the admin plane. Per-namespace bcrypt-hashed keys (stored in `namespace_configs.api_key_hash`) authenticate data-plane requests, with fallback to the global key when a namespace has no key provisioned.
- **Locked client wire contract**: The client-facing JSON types live once in `pkg/codohuetypes` and are re-exported into the server domains via type aliases (e.g. `type Response = codohuetypes.Response`), so the server marshals the exact struct the SDK unmarshals. Request bodies are decoded with `httpapi.DecodeStrict` (rejects unknown fields + trailing data → 400), so client typos fail loudly instead of being silently dropped — a redundant `namespace` in the rankings/catalog body is rejected (the URL path is authoritative; `events` still carries `namespace` because the Redis-stream transport needs it). The marshaled shape of every wire type is pinned by golden snapshots in `pkg/codohuetypes/testdata/` (see the wire-contract convention below).

### REST API — `cmd/api` (port 2001)

**Infra / ops (no auth, unversioned)**

| Method  | Path       | Description                              |
| ------- | ---------- | ---------------------------------------- |
| `GET`   | `/ping`    | Liveness probe                           |
| `GET`   | `/healthz` | Health check for postgres, redis, qdrant |
| `GET`   | `/metrics` | Prometheus metrics                       |

**Client-facing — per-namespace Bearer token (falls back to `CODOHUE_ADMIN_API_KEY`)**

Every business capability is reachable from exactly one canonical path. Legacy duplicate paths have been removed and return 404.

| Method   | Path                                                                    | Description                                                            |
| -------- | ----------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| `POST`   | `/v1/namespaces/{ns}/events`                                            | Ingest a behavioral event (202 Accepted + `{"event_id":N}`; namespace in body is ignored). Also fans the event onto the `codohue:events-tail:{ns}` pub/sub channel for the admin live tail. |
| `POST`   | `/v1/namespaces/{ns}/catalog`                                           | Ingest raw content for catalog auto-embedding (202; only when `dense_source="catalog"`; embedder consumer asynchronously upserts the dense vector). Optional `author_subject_id` is written through to the `objects` table (not stored on `catalog_items`); omitting it leaves any existing attribution alone |
| `GET`    | `/v1/namespaces/{ns}/subjects/{id}/recommendations`                     | CF recommendations for a subject (`?limit=&offset=`)                   |
| `POST`   | `/v1/namespaces/{ns}/rankings`                                          | Score and rank up to 500 candidate items for a subject (200, computed) |
| `GET`    | `/v1/namespaces/{ns}/trending`                                          | Trending items from Redis ZSET (`?limit=&offset=&window_hours=`)       |
| `PUT`    | `/v1/namespaces/{ns}/objects/{id}`                                      | Store per-object metadata — currently `author_subject_id` (idempotent, 200). Accepted for **every** `dense_source`; an empty author clears the attribution |
| `PUT`    | `/v1/namespaces/{ns}/objects/{id}/embedding`                            | Store/replace BYOE dense vector for an object (idempotent, 204). Returns **409 Conflict** when the namespace has `dense_source="catalog"` (catalog auto-embedding is the source of truth in that mode). |
| `PUT`    | `/v1/namespaces/{ns}/subjects/{id}/embedding`                           | Store/replace BYOE dense vector for a subject (idempotent, 204). NOT guarded by catalog mode — unlike object vectors, subject vectors have no single owner: `cmd/cron` phase 2 mean-pools them on every tick, and this endpoint lets a client overwrite one between ticks to cut staleness. |
| `DELETE` | `/v1/namespaces/{ns}/objects/{id}`                                      | Remove an object from all Qdrant collections **and drop its `objects` metadata row** (idempotent, 204). Deleting a *catalog item* from the admin plane does not — that removes embedding input, not the object |

> **Ingest transport options:** Events can be sent via the HTTP endpoint above **or** published directly to Redis Streams — the `ingest` worker consumes both paths and writes to the same `events` table. The Redis Streams transport carries `namespace` inside the payload because it has no URL path.

### REST API — `cmd/admin` (port 2002, session-cookie auth via `codohue_admin_session`)

Authentication models sessions as a resource. Login = create session; logout = delete current session.

The admin API is designed for a monitoring UI, not plain REST CRUD: it exposes **aggregate** endpoints (one payload per view), **SSE streams** (`text/event-stream`, `event: <kind>` frames, `event: ping` heartbeat, `X-Accel-Buffering: no`), and **lifecycle** endpoints for batch runs. SSE rows are marked **(SSE)** below.

| Method   | Path                                                                | Auth          | Description                                                             |
| -------- | ------------------------------------------------------------------- | ------------- | ----------------------------------------------------------------------- |
| `POST`   | `/api/v1/auth/sessions`                                             | none (public) | Validate `CODOHUE_ADMIN_API_KEY`; set session cookie (201 + `expires_at`) |
| `DELETE` | `/api/v1/auth/sessions/current`                                     | session       | Clear session cookie (204)                                              |
| `GET`    | `/api/admin/v1/health`                                              | session       | Proxy `GET /healthz` from `cmd/api`                                     |
| `GET`    | `/api/admin/v1/overview`                                            | session       | Fleet aggregate: health + cron/embedder heartbeat + alerts + per-namespace summary (events 24h, events/min, catalog, qdrant). Replaces the old `?include=overview`. |
| `GET`    | `/api/admin/v1/metrics/summary`                                     | session       | Curated rolling-window metrics: ingest events/sec (1m/5m) per ns + cron batch lag. (Recommend/embedder metrics are scraped from Prometheus, not proxied.) |
| `GET`    | `/api/admin/v1/stream`                                             | session       | **(SSE)** Global ops bus: `batch_run.*`, `catalog.dead_letter_grew`, `catalog.reembed_progress`; drives sidebar badges + toasts |
| `GET`    | `/api/admin/v1/namespaces`                                          | session       | List all namespace configs from PostgreSQL                              |
| `GET`    | `/api/admin/v1/namespaces/{ns}`                                     | session       | Get single namespace config                                             |
| `PUT`    | `/api/admin/v1/namespaces/{ns}`                                     | session       | Create/update namespace; calls `nsconfig.Service` directly (200 / 201). **PATCH semantics** — an omitted field leaves that column untouched; see the config-write note below |
| `DELETE` | `/api/admin/v1/namespaces/{ns}`                                     | session       | Wipe namespace + every trace of its data across postgres, redis, qdrant (200 with summary; 404 when missing) |
| `GET`    | `/api/admin/v1/namespaces/{ns}/dashboard`                          | session       | Per-namespace aggregate: config + last 12 runs (phase strip) + catalog backlog + events 24h/rate + qdrant counts + trending TTL + author coverage (attributed/total catalog items) |
| `POST`   | `/api/admin/v1/reset`                                               | session       | App-wide reset — drops every namespace; requires body `{"confirm":"RESET"}` (400 otherwise) |
| `GET`    | `/api/admin/v1/namespaces/{ns}/catalog`                             | session       | Catalog config + available strategies + backlog + consumer lag + failures + throughput |
| `PUT`    | `/api/admin/v1/namespaces/{ns}/catalog`                             | session       | Enable / update / disable catalog auto-embedding for a namespace (400 on dim mismatch with both dims in body; 503 when feature unwired) |
| `POST`   | `/api/admin/v1/namespaces/{ns}/catalog/re-embed`                    | session       | Trigger a namespace-wide re-embed (202 + `Location`); body `{"only_state":"all\|embedded\|failed"}`; 409 when one is already running; 503 when feature unwired |
| `GET`    | `/api/admin/v1/namespaces/{ns}/catalog/backlog-history`            | session       | Backlog time-series from `catalog_backlog_samples` (`?window=&bucket=`) |
| `GET`    | `/api/admin/v1/namespaces/{ns}/catalog/failures-summary`          | session       | Top `last_error` reasons in window (`?window=`) with a sample object id |
| `GET`    | `/api/admin/v1/namespaces/{ns}/catalog/stream`                    | session       | **(SSE)** Live catalog signals: `item_state_changed`, `backlog_snapshot`, `dead_letter_grew`, `reembed_progress` |
| `GET`    | `/api/admin/v1/namespaces/{ns}/catalog/items`                       | session       | Paginated browse of catalog items (`?state=&limit=&offset=&object_id=&author=&include_summary=&sort=`); `author` is an exact `author_subject_id` match; excludes `content` |
| `GET`    | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}`                  | session       | Full catalog item including `content` and `metadata`                    |
| `POST`   | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive`          | session       | Re-drive a single failed / dead-letter item (202; 404 when not redrivable) |
| `POST`   | `/api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter`    | session       | Bulk re-drive every dead-letter item in the namespace (200 with count)  |
| `DELETE` | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}`                  | session       | Hard-delete a catalog item (Postgres + Qdrant point removal, idempotent 204) |
| `GET`    | `/api/admin/v1/batch-runs`                                          | session       | Recent batch runs as lightweight `BatchRunSummary` (`?namespace=&status=&kind=&limit=&offset=`) |
| `GET`    | `/api/admin/v1/batch-runs/stats`                                   | session       | Batch-run time-series for the fleet (`?window=&bucket=`)                |
| `GET`    | `/api/admin/v1/batch-runs/{id}`                                    | session       | Full `BatchRunDetail` (phases + `log_lines` + target strategy)          |
| `GET`    | `/api/admin/v1/batch-runs/{id}/stream`                            | session       | **(SSE)** Live run: `phase_started/completed`, `log_line`, `run_completed`, `cancelled`. 204 when the run is already terminal (client falls back to the snapshot) |
| `POST`   | `/api/admin/v1/batch-runs/{id}/cancel`                            | session       | Request operator cancel between phases (200; 409 when terminal)         |
| `POST`   | `/api/admin/v1/batch-runs/{id}/retry`                             | session       | Re-run with the same namespace/kind/target (202 + `Location`; 409/422/404 per reject rules) |
| `GET`    | `/api/admin/v1/namespaces/{ns}/batch-runs`                          | session       | Batch run history scoped to one namespace                               |
| `POST`   | `/api/admin/v1/namespaces/{ns}/batch-runs`                          | session       | Create a new batch run (202 Accepted + `Location` header)               |
| `GET`    | `/api/admin/v1/namespaces/{ns}/qdrant`                              | session       | Points count for `{ns}_subjects/objects/subjects_dense/objects_dense`   |
| `GET`    | `/api/admin/v1/namespaces/{ns}/trending`                            | session       | Trending items + Redis TTL (`?limit=&offset=&window_hours=`)            |
| `GET`    | `/api/admin/v1/namespaces/{ns}/events`                              | session       | Paginated recent events (`?limit=&offset=&subject_id=`)                 |
| `GET`    | `/api/admin/v1/namespaces/{ns}/events/stream`                      | session       | **(SSE)** Live event tail (`?action=&subject_id=`): `event` per ingested event, `dropped` on backpressure. Fed by the `codohue:events-tail:*` pub/sub bridge, so it captures HTTP + Redis-stream + injected events |
| `GET`    | `/api/admin/v1/namespaces/{ns}/events/summary`                    | session       | Server-side ingest aggregation (`?window=1m\|5m\|1h`): total + rate + by-action + time-bucketed series |
| `POST`   | `/api/admin/v1/namespaces/{ns}/events`                              | session       | Inject a test event (proxied to `cmd/api`, 202 + `{"ok":true,"event_id":N}`) |
| `GET`    | `/api/admin/v1/namespaces/{ns}/subjects`                            | session       | Browse subjects aggregated from `events` (`?q=` subject_id prefix, `?sort=last_seen\|interactions\|subject_id`, `?limit=&offset=`) |
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
- **010** — `catalog_items` table (raw content store consumed by `cmd/embedder`)
- **011** — `catalog_enabled`, `catalog_strategy_id`, `catalog_strategy_version`, `catalog_strategy_params` on `namespace_configs`
- **012** — pre-prod hardening of `batch_run_logs`: `trigger_source` CHECK-constrained to `('cron', 'manual', 'admin_reembed')`; adds `target_strategy_id` / `target_strategy_version` for catalog re-embed runs; renames `subjects_processed` → `entities_processed` (column now carries CF subject counts during cron runs and catalog item counts during re-embed runs)
- **013** — `cancel_requested` on `batch_run_logs` + partial index `idx_batch_run_logs_running_cancel` for operator-initiated cancel between phases
- **014** — `catalog_backlog_samples` table for the persisted backlog timeline (one row per ns per 30s sampler tick, skip-on-unchanged)
- **015** — indexes on `batch_run_logs.started_at` + `catalog_backlog_samples.sampled_at` to support the cron retention prune (`CODOHUE_BATCH_RUN_RETENTION_DAYS` / `CODOHUE_BACKLOG_SAMPLES_RETENTION_DAYS`)
- **016** — `dense_source` on namespace_configs (single producer enum: `disabled`/`item2vec`/`svd`/`byoe`/`catalog`), backfilled from `catalog_enabled`/`dense_strategy` + CHECK constraint; legacy columns kept for the dual-write window
- **017** — drops the legacy `dense_strategy` + `catalog_enabled` columns; `dense_source` is the single source of truth (down recreates + backfills them, mapping `catalog` → `disabled` + `catalog_enabled=TRUE`)
- **018** — `idx_events_ns_subject_occurred` on `events (namespace, subject_id, occurred_at DESC)` so the admin subject browser aggregates via index-only scan
- **019** — `author_subject_id` on `catalog_items` + partial index `idx_catalog_items_ns_author` (attributed rows only); nullable, no FK
- **020** — `exclude_authored` on namespace_configs (default FALSE) — opt-in filter dropping a subject's own authored objects from their recommendations
- **021** — `objects` table (`namespace`, `object_id`, `author_subject_id`) + partial index; **moves** `author_subject_id` off `catalog_items` and drops that column, so attribution works under every `dense_source`
- **022** — re-keys `id_mappings` on `PRIMARY KEY (namespace, entity_type, string_id)` (was a global `string_id` PK that let two namespaces — or a subject and an object — share one row); run a full recompute per namespace after deploying so Qdrant points use the newly minted numeric ids

## Environment Variables

Copy `.env.example` to `.env`:

```
DATABASE_URL=postgres://codohue:secret@localhost:5432/codohue?sslmode=disable
REDIS_URL=redis://localhost:6379
QDRANT_HOST=localhost
QDRANT_PORT=6334
CODOHUE_ADMIN_API_KEY=dev-secret-key
CODOHUE_BATCH_INTERVAL_MINUTES=5
CODOHUE_LOG_FORMAT=text   # "text" (default) | "json"
CODOHUE_API_PORT=2001     # cmd/api listen port
CODOHUE_ADMIN_PORT=2002   # cmd/admin listen port
CODOHUE_API_URL=http://localhost:2001  # used by cmd/admin to proxy /healthz and inject test events

# Catalog auto-embedding (cmd/embedder) — feature 004
CODOHUE_CATALOG_MAX_CONTENT_BYTES=32768  # default per-namespace cap; can be overridden per-ns via admin API
CODOHUE_EMBED_MAX_ATTEMPTS=5             # transient retries before dead-lettering
CODOHUE_EMBEDDER_HEALTH_PORT=2003        # /healthz + /metrics
CODOHUE_EMBEDDER_REPLICA_NAME=           # consumer name for XREADGROUP; defaults to hostname
CODOHUE_EMBEDDER_POLL_INTERVAL=30s       # how often the worker rescans for newly-enabled namespaces

# Retention (cmd/cron) — keeps observability tables bounded. Set days to 0 to disable a given prune.
CODOHUE_BATCH_RUN_RETENTION_DAYS=30       # batch_run_logs older than this are deleted
CODOHUE_BACKLOG_SAMPLES_RETENTION_DAYS=7  # catalog_backlog_samples older than this are deleted
CODOHUE_RETENTION_INTERVAL=1h             # how often the prune fires
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

### Wire contract (`pkg/codohuetypes`)

The client-facing JSON types are the public contract — treat changes to them as breaking. Any rename, removal, retype, or json-tag change is caught by the golden snapshots in [pkg/codohuetypes/golden_test.go](pkg/codohuetypes/golden_test.go) (one `testdata/*.golden.json` per type). After a **deliberate** contract change, regenerate and commit the snapshots so the diff is reviewed:

```bash
go test ./pkg/codohuetypes/... -run Golden -update
```

When adding a new client-facing wire type, add a case to that test (the orphan guard fails if a snapshot has no matching case). New request fields on existing types must be added to the struct in `codohuetypes` — `httpapi.DecodeStrict` rejects anything not declared there.

### Commit messages

Commit messages describe **what changed and what was added** in repo terms (files, functions, behaviour). Format:

```
type(scope): subject (≤72 chars)

Brief body (2–4 lines max). What the change does, not narrative.

Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>
```

Rules:

- Subject line follows Conventional Commits (`feat`, `fix`, `refactor`, `style`, `chore`, `docs`, `test`).
- **Never reference internal/transient concepts** like "Phase 5", "Tier 1", "Step 2", `T012`, plan/spec task numbers. Reviewers reading `git log` six months later don't have that context — describe the code change in code terms.
- Skip per-file enumeration — the diff already has it.
- Skip trailing "Verified npm run build…" stanzas unless a test result is genuinely surprising and worth recording.
- One paragraph of context maximum. "X had problem Y; this does Z." in one line is usually enough.

<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan
at specs/005-dense-source-unification/plan.md
<!-- SPECKIT END -->
