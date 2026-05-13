# Implementation Plan: Catalog Auto-Embedding Service

**Branch**: `feat/recommend-catalog-embedder` | **Date**: 2026-05-09 | **Spec**: [./spec.md](./spec.md)
**Input**: Feature specification from `/specs/004-catalog-embedder/spec.md`

## Summary

Codohue currently requires every client to compute and push their own dense vectors (BYOE). This feature adds an opt-in per-namespace path where clients send raw textual `content` and Codohue produces the dense vector itself. V1 ships a deterministic, training-free, in-process Go embedding strategy (feature hashing trick + character n-grams, L2-normalised) and a new `cmd/embedder` binary that drains a per-namespace Redis Streams queue. The strategy abstraction lives in a new shared package `internal/core/embedstrategy/` so a follow-up phase can register external-LLM strategies (OpenAI-, Cohere-style) as a purely additive change with no migration of existing namespaces or catalog records ([R3](./research.md#r3-strategy-abstraction-forward-compat-seam-for-v2-llms), [strategy-interface contract](./contracts/strategy-interface.md)).

The recommend service requires no change: catalog-produced vectors land in the same `{ns}_objects_dense` Qdrant collection as today's BYOE writes, with two additional payload keys (`strategy_id`, `strategy_version`) for operational inspection. Subject-side embeddings continue to be derived by `cmd/cron`'s existing mean-pooling step.

## Technical Context

**Language/Version**: Go 1.26.1 (matching `go.mod`)
**Primary Dependencies**: `chi/v5` (HTTP), `pgx/v5` + `pgxpool` (Postgres), `redis/go-redis/v9` (Redis Streams + state), `qdrant/go-client/v1.17` (vector store), `prometheus/client_golang` (metrics), `golang.org/x/text/unicode/norm` (Unicode NFC for tokenizer — already an indirect dep), stdlib `hash` for xxhash-style integer hashing OR `cespare/xxhash/v2` (already indirect via go-redis). No new third-party dependencies.
**Storage**: PostgreSQL (catalog_items table — migration 010; namespace_configs additions — migration 011); Redis Streams (transient embed queue, one stream per namespace); Qdrant (`{ns}_objects_dense` collection — existing).
**Testing**: `go test` via `make test` and `make test-pkg`; `_test.go` files for every `service.go` / `repository.go` / `worker.go` / `job.go` (Constitution II); e2e suite extension `make test-e2e-heavy` for ingest→embed→recommend cycle.
**Target Platform**: Linux server (Docker Compose for local dev; identical for production).
**Project Type**: Server-side Go workspace project. Adds one new binary (`cmd/embedder`) and two new internal domains (`internal/catalog`, `internal/embedder`) plus one new shared core package (`internal/core/embedstrategy`).
**Performance Goals**: Per-item embed p95 < 5 ms for V1 hashing strategy on 1 KiB content (validated in benchmarks). Sustains 1k items/sec ingest per namespace. p95 freshness latency (ingest → discoverable in dense recommendations) < 60 s under nominal load (SC-002).
**Constraints**: Catalog ingest endpoint MUST NOT degrade `/recommendations` p99 latency by more than 5% (SC-004). The data plane (`cmd/api`) MUST remain stateless w.r.t. catalog work — all queue + state persistence is in Redis + Postgres. The embedder binary MUST be optional: a deployment that runs no `cmd/embedder` replicas and has every namespace at `catalog_enabled=false` MUST behave identically to the pre-feature build (SC-005).
**Scale/Scope**: Up to 10⁷ catalog rows per namespace; dozens of catalog-enabled namespaces; embedder horizontally scaled by replica count (consumer-group sharded). Strategy registry is O(10) entries lifetime.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

| Gate | Status | Notes |
|------|--------|-------|
| **I. Code Quality** — domain in `internal/<domain>/`, `docs.go` present, import boundaries respected, English-only comments | ✅ | Two new domains follow the standard layout (`docs.go`, `handler.go` where applicable, `service.go`, `repository.go`, `types.go`, `worker.go`/`job.go` where applicable, all with `_test.go`). Strategy interface placed in `internal/core/embedstrategy/` so cross-domain dependents (`internal/admin`, `internal/catalog`, `internal/embedder`) all use the allowed `core/` import path. No domain imports another domain. |
| **II. Testing Standards** — `_test.go` planned for every `service.go`, `repository.go`, `job.go`, `worker.go` | ✅ | Plan tracks: `internal/catalog/{service,repository,handler}_test.go`; `internal/embedder/{strategy,hashing,tokenizer,service,repository,worker}_test.go`; `internal/core/embedstrategy/{registry}_test.go`. e2e coverage added in `e2e/catalog_test.go` under `-tags=e2e`. |
| **III. API Consistency** — endpoints follow `/v1/<resource>`, two-tier auth, REST API table in `CLAUDE.md` updated | ✅ | New data-plane endpoint `POST /v1/namespaces/{ns}/catalog` follows the canonical sub-resource style ratified in 003. Auth uses the existing per-namespace bearer scheme (with global key fallback). Admin endpoints follow `/api/admin/v1/...` with session cookie. REST API table delta documented in `contracts/rest-api.md` and lands in `CLAUDE.md` at implementation time. The BYOE `PUT .../objects/{id}/embedding` adds a 409 conflict path when catalog is enabled — documented in the same table. |
| **IV. Performance** — Redis cache plan in place, batch phases non-blocking, cold-start fallback accounted for | ✅ | Catalog ingest is a write path — no recommendation cache impact. Embedder workers are independent of `cmd/cron` and `cmd/api` request handlers — cannot block batch phases or recommend latency. Cold-start fallback path in recommend is unchanged: a subject with 0 interactions still falls back to Redis trending; catalog only affects what populates `*_objects_dense`. The 90-day decay and γ rerank are unaffected. |

> Constitutional violation present in Architecture Constraints (binary count). See Complexity Tracking below.

### Architecture Constraints check

| Constraint | Status | Notes |
|---|---|---|
| ID mappings via `id_mappings` table (no hash-based ID generation) | ✅ | Catalog-driven Qdrant points use the same `id_mappings` flow as BYOE writes. |
| Migrations as numbered `NNN_name.up.sql` / `.down.sql` pairs | ✅ | This feature adds 010 (catalog_items) and 011 (namespace_configs catalog columns). |
| Full-recompute strategy maintained | ✅ (with rationale) | Catalog embedding is per-item incremental, but the V1 strategy is *deterministic* — there is no learned model state to forget, so the constitution's catastrophic-forgetting rationale does not apply. Future learned strategies will be required to use the operator-triggered namespace-wide re-embed mechanism (FR-013) which is the same full-recompute pattern. See [research.md R12](./research.md#r12-constitution-alignment-audit). |
| Exactly two binaries (`cmd/api`, `cmd/cron`) | ❌ Violation | Codebase already has three (`cmd/admin` shipped after constitution v1.0.0). This plan adds `cmd/embedder` for a total of four. Justified in Complexity Tracking; constitution is due a PATCH bump in a separate change set to acknowledge both. |

## Project Structure

### Documentation (this feature)

```text
specs/004-catalog-embedder/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 — design decisions resolving spec NEEDS-CLARIFICATION
├── data-model.md        # Phase 1 — entities, schema, lifecycle, transport schemas
├── quickstart.md        # Phase 1 — operator + developer ramp-up walkthrough
├── contracts/
│   ├── rest-api.md           # Phase 1 — HTTP surfaces (data plane + admin plane)
│   ├── strategy-interface.md # Phase 1 — Go contract for embedding strategies (forward-compat seam)
│   └── redis-stream.md       # Phase 1 — XADD/XREADGROUP/XAUTOCLAIM transport schema
├── checklists/
│   └── requirements.md  # Spec quality checklist (created at /speckit.specify time)
└── tasks.md             # Phase 2 — created later by /speckit.tasks
```

### Source Code (repository root)

```text
cmd/
├── api/                  # existing — gains catalog ingest handler wiring + 409 on BYOE write under catalog mode
├── admin/                # existing — gains catalog admin endpoint wiring
├── cron/                 # existing — unchanged
└── embedder/             # NEW — fourth binary (constitutional violation, see Complexity Tracking)
    ├── main.go
    └── main_test.go      # smoke test for run() bootstrap, mirroring cmd/cron/main_test.go

internal/
├── catalog/              # NEW domain — HTTP ingest, content-hash idempotency, queue publishing
│   ├── docs.go
│   ├── handler.go
│   ├── handler_test.go
│   ├── service.go        # validation, content_hash, persist, XADD
│   ├── service_test.go
│   ├── repository.go     # catalog_items CRUD
│   ├── repository_test.go
│   └── types.go
├── embedder/             # NEW domain — strategy implementations + worker + per-namespace orchestration
│   ├── docs.go
│   ├── strategy.go       # registers V1 hashing-ngrams strategy in init()
│   ├── strategy_test.go
│   ├── hashing.go        # feature-hashing + sign-trick implementation
│   ├── hashing_test.go
│   ├── tokenizer.go      # whitespace + character n-grams (n=3..5)
│   ├── tokenizer_test.go
│   ├── service.go        # per-item orchestration: load row → tokenize → embed → upsert Qdrant → mark embedded
│   ├── service_test.go
│   ├── repository.go     # reads catalog_items, writes state transitions
│   ├── repository_test.go
│   ├── worker.go         # Redis Streams XREADGROUP loop + XAUTOCLAIM reaper + recovery sweep + re-embed completion watcher
│   ├── worker_test.go
│   └── types.go
├── core/
│   ├── (existing)
│   └── embedstrategy/    # NEW — Strategy interface, Registry, sentinel errors
│       ├── docs.go
│       ├── registry.go
│       ├── registry_test.go
│       └── types.go
├── admin/                # existing — gains catalog admin handler/service/repository additions (under existing admin domain layout)
│   └── (catalog-related additions inside existing files; new files only if a clean separation makes test isolation easier)
├── recommend/            # existing — adds the FR-018 409 check at the BYOE write handler
│   └── (one-line lookup of nsconfig.CatalogEnabled before the existing BYOE write logic)
├── nsconfig/             # existing — extends config struct + repository to read/write the new catalog_* columns
│   └── (no new files; existing types.go, repository.go, service.go, handler.go gain the new fields)
├── ingest/               # existing — unchanged
└── (other existing domains unchanged)

migrations/
├── 010_catalog_items.up.sql
├── 010_catalog_items.down.sql
├── 011_namespace_configs_catalog.up.sql
└── 011_namespace_configs_catalog.down.sql
```

**Structure Decision**: Server-side Go workspace project structure, adding two new domains (`internal/catalog`, `internal/embedder`), one new shared core package (`internal/core/embedstrategy`), one new binary (`cmd/embedder`), and two new migrations (010, 011). Existing domains (`recommend`, `nsconfig`, `admin`) gain additive changes only — no rewrites, no domain renames. The `internal/core/embedstrategy` placement is the linchpin of the forward-compat seam ([R3](./research.md#r3-strategy-abstraction-forward-compat-seam-for-v2-llms)).

## Complexity Tracking

> **Filled because Constitution Check has one violation that must be justified.**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Fourth binary `cmd/embedder` (constitution allows two; codebase already has three with `cmd/admin`) | The catalog feature adds workload with **categorically different operational characteristics from anything in cmd/api or cmd/cron**: (1) variable-cost embedding work whose future external-LLM strategies will have unpredictable rate-limits, network failure modes, and per-token billing — bleeding any of these into the data plane is exactly what the user authorised separating in [Q1](./spec.md#clarifications); (2) burst load patterns during operator-triggered namespace-wide re-embeds that we explicitly do not want competing for `cmd/api` request-handler goroutines; (3) deployment scaling shaped by embedding throughput rather than recommendation QPS, motivating an independently-scaled replica set; (4) the spec's SC-005 promise that disabling the feature has *zero impact* on existing BYOE clients is directly satisfied by simply not deploying any `cmd/embedder` replica — impossible if the work lived in `cmd/api`. | **Worker goroutine in cmd/api**: rejected at spec time as Approach B; bleeds embedder failure modes into recommendation tail latency (SC-004 violation surface), couples deployment scaling. **Phase in cmd/cron**: rejected because cmd/cron is batch-tick-driven, which adds 5-minute tail latency to SC-002 (60 s p95) and serialises catalog work across all namespaces under one process. The constitution is itself due a PATCH bump in a separate change set acknowledging the existing `cmd/admin` (which predates this plan) and the new `cmd/embedder`; that constitution amendment is OUT OF SCOPE for this feature. |

No other Constitution Check items have violations. The "full-recompute strategy" clause (Architecture Constraints) is honoured in spirit because the V1 strategy is deterministic (no learned model) and FR-013 already preserves the full-recompute pattern for any future learned strategies via operator-triggered namespace-wide re-embed.
