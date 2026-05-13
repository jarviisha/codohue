# Phase 0 Research: Catalog Auto-Embedding Service

**Feature**: 004-catalog-embedder
**Date**: 2026-05-09
**Inputs**: [spec.md](./spec.md) clarifications session 2026-05-09 (5 questions, all resolved); existing codebase patterns at [internal/ingest/](../../internal/ingest/), [internal/compute/](../../internal/compute/), [cmd/cron/](../../cmd/cron/); migrations 001–009.

This document resolves every plan-phase NEEDS CLARIFICATION carried over from the spec and consolidates the design decisions that anchor the rest of the plan. There are no remaining NEEDS CLARIFICATION markers after this phase.

---

## R1. Embedding algorithm — concrete shape of the V1 deterministic strategy

**Decision**: Feature hashing trick directly to the target dimension, with sign trick for collision-bias reduction. No two-step "hash to high dim then project down" pipeline. Tokenizer emits whitespace tokens plus character n-grams (n=3..5) per the spec; each token is hashed (xxhash64) to a slot in `[0, dim)` and an independent sign hash maps it to `±1`. The output vector is the L2-normalised sum of `sign(token) * weight(token)` across all tokens, where `weight(token)` is TF (term-frequency, raw count) for V1.

**Rationale**:
- The spec's Q1 answer mandates "deterministic, training-free, in-process Go". Feature hashing is the textbook deterministic embedding (used by `vowpal_wabbit`, `scikit-learn` `HashingVectorizer`) and avoids the IDF-corpus-snapshot problem that would otherwise creep in via TF-IDF.
- The spec's Q3 answer mandates whitespace + character n-grams (n=3..5). This stage runs before hashing.
- Direct hashing to target dim is simpler than hash → projection. Random projection adds a fixed random matrix that must be seeded and persisted, with no quality benefit over direct hashing in the deterministic regime.
- Sign trick (Weinberger et al., 2009) cancels expected collision bias to zero, raising effective recall on small dims (e.g. dim=128) for free.
- L2 normalisation makes cosine similarity equivalent to dot product, so the existing recommend service's hybrid blend formula keeps working.

**Alternatives considered**:
- *TF-IDF + SVD (LSA)*: rejected because SVD requires a corpus snapshot, which violates "training-free" and re-introduces an artifact-versioning concern Q1 explicitly stepped around.
- *Hashing then random projection*: rejected as needless extra step; random projection on top of a random hash adds no quality.
- *Plain TF without sign trick*: rejected because uncorrected hash collisions inflate magnitudes for common tokens, distorting cosine similarity.
- *BM25 weighting*: rejected because BM25 needs corpus statistics (avgdl, df), reintroducing the same artifact problem.

**Implications**:
- Strategy parameters: `hash_dim` (= `namespace.embedding_dim`), `ngram_min=3`, `ngram_max=5`, `lowercase=true`, `strip_punct=true`. All fixed by strategy version, none operator-tuneable in V1.
- Determinism: identical input + identical strategy version → bitwise-identical output, no PRNG state.
- Empty content edge case: if all tokens are filtered out, returns the zero vector before normalisation; embedder rejects zero vectors at write time and moves the item to dead-letter (caught by FR-009 dimension-mismatch sibling check expanded to also cover zero-norm).

---

## R2. Tokenizer implementation

**Decision**: Pure-Go tokenizer with the steps: Unicode NFC-normalise → lowercase via `strings.ToLower` → split on Unicode whitespace via `unicode.IsSpace` → drop tokens of length < 1 → strip Unicode-category punctuation → emit each surviving token AND its character n-grams for n in `[3,4,5]`. No external library, no shipped dictionary. Hashtags (`#foo`) and mentions (`@foo`) are kept as single tokens (no special handling). URLs are dropped via a simple prefix check (`http://`, `https://`).

**Rationale**:
- The spec declares language-agnostic and explicitly excludes Vietnamese segmenters from V1.
- Character n-grams cover the Vietnamese multi-syllable case without a dictionary.
- URL drop prevents one common social-media noise source from dominating the hash space.
- All operations are stdlib-only (`strings`, `unicode`, `unicode/norm` from `golang.org/x/text/unicode/norm`); no new third-party dependency. `golang.org/x/text` is already an indirect dependency.

**Alternatives considered**:
- *Keep punctuation*: rejected because punctuation tokens cluster at high frequency and hash to the same dominant slots, swamping content tokens.
- *No URL drop*: rejected — URLs shared across many posts collide in the hash space and create false similarity signals.
- *Word-boundary regex (\b)*: rejected because Go's `regexp` is much slower than `unicode.IsSpace` walks for the per-item embed call; we want this fast (target: <5 ms per typical post).

---

## R3. Strategy abstraction (forward-compat seam for V2 LLMs)

**Decision**: Define the strategy abstraction in a new shared package `internal/core/embedstrategy/` so both `internal/embedder` and `internal/admin` can depend on it without violating the cross-domain import rule.

```go
// internal/core/embedstrategy/types.go
type Strategy interface {
    ID() string                                       // e.g. "internal-hashing-ngrams"
    Version() string                                  // e.g. "v1"
    Dim() int                                         // produced vector dimension
    MaxInputBytes() int                               // hard input cap; >0 → reject at ingest
    Embed(ctx context.Context, content string) ([]float32, error)
}

type Params map[string]any  // extension slot — V1 ignores; V2 LLMs read credentials/model_id

type Factory func(p Params) (Strategy, error)

type Registry struct { /* ... */ }
func (r *Registry) Register(id, version string, f Factory)
func (r *Registry) Build(id, version string, p Params) (Strategy, error)
```

The V1 hashing strategy lives in `internal/embedder/hashing.go` and registers itself via `init()` against a default registry. A V2 OpenAI strategy would live in `internal/embedder/openai.go`, register under a different `(id, version)` pair, and read credentials from `Params` — no changes to `internal/catalog`, no changes to the catalog HTTP contract, no changes to the `catalog_items` table, no changes to the admin contract beyond presenting the new `(id, version)` option in dropdowns. This realises FR-021 and SC-009.

**Rationale**:
- The constitution allows domains to import `core/`, so placing the interface there is the canonical way to share contracts without violating the no-cross-domain-imports rule.
- The `Params` map is the FR-007 "extension slot" — a future credentials reference is just a new key (e.g. `Params["api_key_secret_ref"]`).
- Self-registration via `init()` lets new strategies be added by adding a new file under `internal/embedder/` without touching wiring code.

**Alternatives considered**:
- *Interface in `internal/embedder/types.go`*: rejected — `internal/admin` would have to import `internal/embedder` to validate strategy ids, breaking the cross-domain rule.
- *String-only registry with no interface*: rejected — gives up compile-time type safety on the `Embed` call, which becomes critical when V2 strategies have very different latency/error profiles.
- *Plugin (.so) loading at runtime*: rejected — Go plugins are notoriously fragile across builds and run on a single OS only; static registration with `init()` covers V1+V2 needs with zero plugin tooling.

---

## R4. Queue between catalog ingest (cmd/api) and embedder (cmd/embedder)

**Decision**: Redis Streams with consumer group, consistent with the existing `ingest` worker pattern. Stream name `catalog:embed:{ns}` per namespace; consumer group `embedder`. Catalog ingest in `cmd/api` does (a) `INSERT INTO catalog_items ... RETURNING id` then (b) `XADD catalog:embed:{ns}` with `{catalog_item_id, namespace, strategy_id, strategy_version}`. Embedder workers `XREADGROUP > BLOCK 5s`, process, then `XACK`. Failed claims are reaped via `XAUTOCLAIM` with a 60-second min-idle threshold so a crashed worker does not strand items.

**Rationale**:
- Identical pattern to the existing ingest worker → operators already understand consumer-group semantics, dead-letter via `XPENDING`, and how to scale with multiple replicas.
- Per-namespace streams give fair scheduling and let operators inspect per-namespace backlog with `XLEN catalog:embed:{ns}`.
- Postgres remains the source of truth (catalog_items row is committed before XADD); the stream is a transient queue, so a stream loss only forces a re-scan of `catalog_items WHERE state IN ('pending','in_flight')`, which is also how the operator-triggered re-embed works.

**Alternatives considered**:
- *Postgres `LISTEN/NOTIFY`*: rejected — does not survive consumer disconnect; misses notifications fired while the consumer is reconnecting; does not provide consumer-group semantics.
- *Postgres polling only*: rejected — adds DB load proportional to embedder count; defeats the multi-replica scaling story.
- *Single global stream `catalog:embed`*: rejected — one noisy namespace would head-of-line-block all others.
- *Kafka*: rejected — adds a new infrastructure dependency for no V1 benefit; can be reconsidered if scale exceeds Redis Streams capacity.

**Implications**:
- The XADD must fire AFTER the Postgres row commits to avoid the embedder picking up an invisible row. Use `INSERT ... RETURNING id` then `XADD` outside the txn.
- Idempotency: if `XADD` fails after commit, a recovery sweep finds rows with state=`pending` whose claim count is zero and re-publishes them. This is the only place the embedder polls Postgres in V1.

---

## R5. Embedder run trigger model and concurrency

**Decision**: Continuous worker loop. `cmd/embedder` runs N goroutines (default `GOMAXPROCS`), each blocked on `XREADGROUP > BLOCK 5s` against all enabled namespaces' streams (one per goroutine, fan-in via reflect.SelectCase OR per-stream goroutine). Multi-replica deployment is supported via the consumer group: each replica claims a disjoint subset of pending entries. A single coordinator goroutine per replica runs the recovery sweep + `XAUTOCLAIM` every 60 seconds.

**Rationale**:
- Continuous loop minimises p95 freshness latency (SC-002: 60s for 95% of items) — no batch-tick wait.
- Consumer group + `XAUTOCLAIM` is the standard Redis Streams reliability pattern; it is exactly how the existing `ingest` worker handles ingest crash recovery.
- One goroutine per stream avoids head-of-line blocking across namespaces while keeping the per-stream sequential processing that makes per-item ordering deterministic.

**Alternatives considered**:
- *Periodic batch ticks (cron-style)*: rejected — adds tail latency equal to the tick interval and provides no operational benefit since the V1 strategy is CPU-cheap.
- *Single-instance worker with leader election*: rejected — caps embedding throughput at one CPU; multi-replica via consumer group is strictly more capable with the same code complexity.

**Implications**:
- The plan adds `cmd/embedder/main.go` with the same shape as `cmd/cron/main.go` — config load, infra clients, signal handling, run loop.
- Default replica count for production deployment: 1 (catch up by simply scaling the deployment to 2+ when backlog accumulates). The embedder is stateless beyond Redis stream offsets.

---

## R6. Re-embed trigger mechanism (operator-initiated, namespace-wide)

**Decision**: Reuse `batch_run_logs` (migration 006/007/008/009). The admin endpoint `POST /api/admin/v1/namespaces/{ns}/catalog/re-embed` (a) inserts a `batch_run_logs` row with `phase=embed_reembed`, `trigger_source=admin`, status=`running`; (b) `SELECT id FROM catalog_items WHERE namespace=$1 AND strategy_version <> $2` to enumerate stale items; (c) bulk-XADD them to `catalog:embed:{ns}`. The embedder workers naturally drain the resulting backlog. A separate goroutine in `cmd/embedder` watches `XLEN` and updates the same `batch_run_logs` row's `subjects_processed` and `success` columns when the count reaches zero.

**Rationale**:
- Reuses operator mental model and admin UI surface (existing batch-runs view at `cmd/admin`).
- One re-embed per namespace at a time: enforced by checking the namespace has no `batch_run_logs` row with `phase LIKE 'embed%'` and `status='running'` before accepting the trigger.
- Concurrent re-embeds across different namespaces are allowed (different `batch_run_logs` rows, different streams).

**Alternatives considered**:
- *New table `embed_runs`*: rejected — duplicates `batch_run_logs` fields; the only difference is phase semantics, which the existing `phase{1,2,3}_*` columns already accommodate as a sentinel value.
- *In-memory state*: rejected — does not survive embedder restart; operators cannot resume a half-completed re-embed.

**Implications**:
- Migration 010 will add `phase=embed_initial | embed_reembed` as documented enum sentinel values (no schema change, the column is already TEXT).
- Admin endpoints exposed:
  - `POST /api/admin/v1/namespaces/{ns}/catalog/re-embed` — 202 Accepted + Location header to the batch-run resource.
  - `GET /api/admin/v1/namespaces/{ns}/catalog` — status, backlog size (XLEN + DB pending count), in-flight count, dead-letter count, active strategy id+version.

---

## R7. Mixed-version detection (Edge Case "Mixed-version dense collection during transition")

**Decision**: Two layers of strategy_version tagging:
1. **Postgres**: `catalog_items.strategy_id` and `catalog_items.strategy_version` columns are updated on each successful embed. This is the authoritative source of truth.
2. **Qdrant point payload**: every dense vector point upsert includes `payload.strategy_version` (and `payload.strategy_id`). This is for inspection and debugging; the recommend service ignores it.

Re-embed completion is detected by `SELECT count(*) FROM catalog_items WHERE namespace=$1 AND strategy_version <> $2` returning zero.

**Rationale**:
- DB tag is required for the re-embed sweep query.
- Qdrant payload tag is operationally useful: an operator inspecting a recommendation result can see which strategy version produced each candidate's vector.
- Recommend service does not need to know — it just consumes vectors. This preserves the spec's promise that recommend requires no change.

**Alternatives considered**:
- *DB tag only*: rejected — Qdrant inspection is genuinely useful for debugging mixed-version transitions and recommend-quality regressions.
- *Qdrant tag only*: rejected — DB sweep query needs the tag; Qdrant `Scroll` over millions of points is far slower.

---

## R8. Source-of-truth conflict policy (FR-018, Assumption "Source-of-truth precedence")

**Decision**: When a namespace has `catalog_enabled=true`, the existing `PUT /v1/namespaces/{ns}/objects/{id}/embedding` endpoint (BYOE write) returns **409 Conflict** with the body `{"error":"namespace uses catalog auto-embedding; BYOE writes for object dense vectors are not accepted"}`. The check is placed in `internal/recommend`'s embedding-write handler (the only domain that owns that endpoint today), gated by a one-line `nsconfig` lookup.

**Rationale**:
- Matches the spec's explicit decision in the Source-of-truth precedence assumption: "rejected (not silently overwritten) for clarity".
- 409 (rather than 403) communicates "the resource state forbids this", which is semantically right: the resource (namespace) is in catalog mode.
- One-line check keeps the cross-cutting policy testable without spreading state across domains.

**Alternatives considered**:
- *403 Forbidden*: rejected — auth-style status code, misleading.
- *Silent acceptance with warning*: rejected by the spec.
- *400 Bad Request*: rejected — request shape is fine, the namespace state forbids it.

**Implications**:
- The recommend domain reads `nsconfig.GetByNamespace(ns).CatalogEnabled` on each BYOE write. This is a single Postgres lookup; an existing 5-minute TTL cache can be added if shown to matter, but is not part of V1.

---

## R9. Oversized content policy (FR-020)

**Decision**: Reject at ingest with **413 Payload Too Large** when `len(content) > MAX_CONTENT_BYTES`. Default `MAX_CONTENT_BYTES = 32768` (32 KiB), env-configurable via `CATALOG_MAX_CONTENT_BYTES`. No truncation in V1.

**Rationale**:
- The V1 strategy is fast on long content (linear in tokens), but the spec mandates an explicit policy and "reject" is operationally clearer than "truncate then maybe surprise the operator with which prefix won".
- 32 KiB covers all realistic social-media posts, articles, captions; flags accidental binary uploads.
- Future external-LLM strategies will have provider-specific length limits — the strategy interface exposes `MaxInputBytes()` so per-strategy limits can be enforced at ingest time without changing the catalog API. V1 sets `MaxInputBytes()` to whatever `CATALOG_MAX_CONTENT_BYTES` is configured to, falling back to 32 KiB.

**Alternatives considered**:
- *Truncate to limit*: rejected for V1 — leaves the operator wondering which prefix was kept; trivially added later as a per-strategy policy.
- *No limit*: rejected — opens the embedder to OOM via a single multi-megabyte ingest.

---

## R10. Observability indicators (FR-014, FR-015, SC-007)

**Decision**: Three categories of indicators, all per-namespace, surfaced through (a) Prometheus at the existing `/metrics` endpoint and (b) the admin endpoint `GET /api/admin/v1/namespaces/{ns}/catalog`:

| Category | Indicator | Source |
|---|---|---|
| Backlog | `catalog_pending_total{namespace}` | `XLEN catalog:embed:{ns}` + `SELECT count(*) FROM catalog_items WHERE state='pending'` |
| Backlog | `catalog_inflight_total{namespace}` | `XPENDING catalog:embed:{ns} embedder` |
| Backlog | `catalog_deadletter_total{namespace}` | `SELECT count(*) FROM catalog_items WHERE state='dead_letter'` |
| Throughput | `catalog_items_embedded_total{namespace,strategy_id,strategy_version}` | counter, incremented per success |
| Throughput | `catalog_embed_duration_seconds{namespace,strategy_id,strategy_version}` | histogram |
| Throughput | `catalog_embed_failures_total{namespace,strategy_id,strategy_version,reason}` | counter, reasons: `dim_mismatch`, `zero_norm`, `panic`, `oversized_at_strategy`, etc. |
| Resource | `catalog_strategy_work_volume_total{namespace,strategy_id,strategy_version,unit}` | counter — for V1 `unit=tokens_processed`; future external strategies will additionally emit `unit=billed_tokens` and `unit=billed_cost_micro_usd` through the SAME metric, satisfying the "uniform indicator surface" clause of FR-015 |

**Rationale**:
- Prometheus + existing `/metrics` endpoint keeps integration with whatever observability stack the operator already runs.
- The resource indicator deliberately uses a `unit` label so V2 LLM strategies emit cost without redesigning the metric surface (the FR-015 forward-compat clause).

**Alternatives considered**:
- *Logs-only*: rejected — defeats SC-007 ("visible within 5 minutes of accrual").
- *Per-item attribution table*: rejected — Assumptions explicitly mark per-item cost OUT OF V1 SCOPE.

---

## R11. Subject-side embeddings under catalog mode (Assumption: subject embeddings derived by cron mean-pooling)

**Decision**: No change to the existing `cmd/cron` mean-pooling step. The cron's user_dense computation reads object dense vectors from the same `{ns}_objects_dense` Qdrant collection it always has — the catalog feature only changes how those vectors arrive (catalog instead of BYOE). Subject vectors will naturally re-derive on the next cron tick after a catalog-driven update.

**Rationale**: Preserves the spec's promise that recommend requires no change; reuses the existing mean-pool path; aligns with the constitution's "full-recompute strategy" for subject vectors specifically.

**Implications**: A re-embed of a namespace will produce a transient mismatch between subject vectors (still derived from old object vectors) and newly re-embedded object vectors, until the next cron tick (default 5 min). This is acceptable because the spec already declares the dense collection is legitimately mixed-version during transition.

---

## R12. Constitution alignment audit

The constitution at [/.specify/memory/constitution.md](../../.specify/memory/constitution.md) is at version 1.0.0 and predates `cmd/admin`. Two clauses interact with this plan:

1. **Architecture Constraint: "exactly two binaries"** — already at three (`cmd/admin` shipped after the constitution was ratified). Adding `cmd/embedder` makes four. This is a constitutional violation that the user explicitly authorised at spec-time (the user's [Q1 answer](./spec.md#clarifications) chose the separate-binary architecture). Tracked under Complexity Tracking in `plan.md` with the rationale: data-plane decoupling for variable-cost embedding work whose failure modes (CPU saturation, future external-LLM rate limits, bulk re-embed bursts) must not bleed into recommend tail latency.

2. **Architecture Constraint: "full-recompute strategy must be maintained; incremental online updates explicitly forbidden because they cause catastrophic forgetting"** — the catalog embedder is per-item incremental, *but* the V1 strategy is deterministic and training-free, so there is no learned-model state that could "catastrophically forget". The clause's rationale ("catastrophic forgetting in the embedding models") does not apply to a hash function. This is **not** a violation; the plan honours the spirit of the clause. Should V2 introduce a *learned* strategy, the same constraint will require that strategy to use a full-recompute pattern via the existing namespace-wide re-embed mechanism — which is why FR-013 already mandates that mechanism.

The plan does not need a constitution amendment for V1; the cmd-count drift is documented in Complexity Tracking and should be addressed by a constitution PATCH bump in a separate change set acknowledging the existing `cmd/admin` and the new `cmd/embedder`.
