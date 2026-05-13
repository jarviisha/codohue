# Phase 1 Data Model: Catalog Auto-Embedding Service

**Feature**: 004-catalog-embedder
**Date**: 2026-05-09
**Inputs**: [spec.md](./spec.md), [research.md](./research.md)

This document defines all persistent and transient data shapes introduced or modified by this feature. Migrations 010 and 011 land the new schema; the strategy abstraction in `internal/core/embedstrategy/` types these contracts in Go.

---

## 1. New table: `catalog_items` (migration 010)

Authoritative store for every catalog item submitted to a namespace. Source of truth for the item's lifecycle, content, and last-applied embedding strategy.

```sql
CREATE TABLE catalog_items (
    id                  BIGSERIAL    PRIMARY KEY,
    namespace           TEXT         NOT NULL,
    object_id           TEXT         NOT NULL,
    content             TEXT         NOT NULL,
    content_hash        BYTEA        NOT NULL,                -- sha256 over content
    metadata            JSONB        NOT NULL DEFAULT '{}'::jsonb,
    state               TEXT         NOT NULL DEFAULT 'pending',
    -- 'pending' | 'in_flight' | 'embedded' | 'failed' | 'dead_letter'
    strategy_id         TEXT,                                  -- null until first embed
    strategy_version    TEXT,                                  -- null until first embed
    embedded_at         TIMESTAMPTZ,
    attempt_count       INTEGER      NOT NULL DEFAULT 0,
    last_error          TEXT,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (namespace, object_id)
);

CREATE INDEX idx_catalog_items_ns_state          ON catalog_items (namespace, state);
CREATE INDEX idx_catalog_items_ns_strategy_ver   ON catalog_items (namespace, strategy_version);
CREATE INDEX idx_catalog_items_state_attempt     ON catalog_items (state, attempt_count) WHERE state IN ('pending','failed');
CREATE INDEX idx_catalog_items_updated_at        ON catalog_items (updated_at DESC);
```

### Field semantics

| Field | Source / mutation rules |
|---|---|
| `namespace`, `object_id` | Set on first ingest; immutable. Together unique. |
| `content` | The exact text the embedder receives. Set on every ingest call. |
| `content_hash` | sha256 over `content` only (never metadata). Recomputed on every ingest. Idempotency key for FR-002. |
| `metadata` | Opaque JSON stored verbatim. Never feeds the embedder. May be updated on re-ingest. |
| `state` | Lifecycle below. |
| `strategy_id`, `strategy_version` | Set only on a successful embed. Used for re-embed sweep query and for `Qdrant` payload tagging. |
| `embedded_at` | UTC timestamp of the last successful embed. |
| `attempt_count` | Incremented on each transient failure; reset to 0 on each new ingest of changed content. |
| `last_error` | Free-form error string from the most recent failed attempt. |
| `created_at`, `updated_at` | Standard. `updated_at` maintained by the application (Go), not a trigger. |

### Lifecycle (`state` column)

```
                  ┌────────────┐
ingest ──────────▶│  pending   │
                  └─────┬──────┘
                        │ worker XREADGROUP
                        ▼
                  ┌────────────┐
                  │ in_flight  │
                  └─────┬──────┘
            success │           │ transient failure
                    ▼           ▼
              ┌──────────┐  ┌──────────┐
              │ embedded │  │  failed  │ (attempt_count++)
              └──────────┘  └─────┬────┘
                                  │ retry by re-publishing to stream
                                  ▼
                            ┌──────────┐
                            │ pending  │  (back to top)
                            └─────┬────┘
                                  │ attempt_count > MAX_ATTEMPTS
                                  ▼
                            ┌─────────────┐
                            │ dead_letter │
                            └─────────────┘
```

Re-ingest of the same `object_id` with a different `content_hash` resets state to `pending`, attempt_count to 0, last_error to NULL, and re-publishes to the stream. Re-ingest with the same `content_hash` AND the same active `strategy_version` is a no-op (FR-002).

The operator-triggered namespace-wide re-embed (FR-013) does not change `state` directly — it bulk-re-publishes already-`embedded` items by XADD-ing them to the stream; the worker transitions them through `in_flight` → `embedded` again with the new `strategy_version`.

---

## 2. Modified table: `namespace_configs` (migration 011)

Add the catalog feature toggle and active strategy profile.

```sql
ALTER TABLE namespace_configs
    ADD COLUMN catalog_enabled            BOOLEAN  NOT NULL DEFAULT FALSE,
    ADD COLUMN catalog_strategy_id        TEXT,
    ADD COLUMN catalog_strategy_version   TEXT,
    ADD COLUMN catalog_strategy_params    JSONB    NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN catalog_max_attempts       INTEGER  NOT NULL DEFAULT 5,
    ADD COLUMN catalog_max_content_bytes  INTEGER  NOT NULL DEFAULT 32768;
```

### Field semantics

| Field | Notes |
|---|---|
| `catalog_enabled` | Master toggle. When `false`, catalog ingest endpoint returns 404 for this namespace (FR-008). |
| `catalog_strategy_id`, `catalog_strategy_version` | Active strategy profile per FR-007. Both NOT NULL when `catalog_enabled=true`; enforced at admin write time, not at the schema level (kept nullable to allow disable→enable cycles). |
| `catalog_strategy_params` | The FR-007 extension slot. V1 hashing strategy reads no params. Future external-LLM strategies read e.g. `{"api_key_secret_ref":"vault://...","model":"text-embedding-3-large"}`. |
| `catalog_max_attempts` | Per-namespace override of the global `EMBED_MAX_ATTEMPTS` env default. After this many transient failures, item moves to `dead_letter`. |
| `catalog_max_content_bytes` | Per-namespace override of the global `CATALOG_MAX_CONTENT_BYTES` env default. Items above this are rejected at ingest with 413. |

### Validation rules (enforced in admin handler, not in schema)

1. Enabling catalog requires `catalog_strategy_id` + `catalog_strategy_version` to resolve via the in-process registry to a `Strategy` whose `Dim()` equals the namespace's existing `embedding_dim`. Mismatch → 400 with both numbers in the error body (US2 acceptance #2).
2. Disabling catalog (transition `catalog_enabled` from true to false) is allowed but does NOT delete `catalog_items` rows or Qdrant points; operator must explicitly clear them via `DELETE /api/admin/v1/namespaces/{ns}/catalog/items` (out of P1 scope; can be added later).
3. Changing `catalog_strategy_id` or `catalog_strategy_version` while `catalog_enabled=true` is the trigger event for FR-013. The admin handler updates the namespace_config and immediately enqueues a re-embed batch run, OR the operator can update the config first and trigger re-embed separately — both paths are supported.

---

## 3. Reused table: `batch_run_logs`

No schema change. Two new conventional values for the existing `phase{1,2,3}_*` aggregation are documented:

- The `phase` field on the row (column `phase` does not exist; the convention is to set `phase1_*` columns when only one phase ran). For the embedder, we set `subjects_processed` to the count of catalog items processed and use `error_message` for any namespace-wide failure context.
- A new convention in the existing `trigger_source` column: values are now `'cron' | 'admin' | 'embed'` (FR-019). `'embed'` is set by `cmd/embedder` when it self-records the completion of a re-embed run; `'admin'` is set by the admin endpoint that initiates the re-embed.

This avoids any new migration on `batch_run_logs` and keeps the existing admin batch-runs UI unchanged.

---

## 4. Qdrant point payload conventions

When the embedder upserts a vector to `{ns}_objects_dense`, it writes the existing point shape PLUS two payload keys:

```json
{
  "point_id": <BIGSERIAL from id_mappings>,
  "vector": [...float32, length = namespace.embedding_dim...],
  "payload": {
    "object_id": "<external string id>",
    "namespace": "<ns>",
    "strategy_id": "<id>",
    "strategy_version": "<version>",
    "embedded_at": "<ISO 8601 UTC>"
  }
}
```

The `object_id` and `namespace` keys are already used by the recommend service. The two new keys (`strategy_id`, `strategy_version`, `embedded_at`) are additive and ignored by recommend; they exist purely for operator inspection and the FR-019 admin debug surface.

---

## 5. Redis Streams transport schema

### Catalog embed stream

- **Stream name**: `catalog:embed:{namespace}` (one per namespace; per-namespace lifetime tied to `catalog_enabled=true`)
- **Consumer group**: `embedder` (single group, multiple workers)
- **Entry fields** (`XADD ... * field value [field value ...]`):

| Field | Type | Notes |
|---|---|---|
| `catalog_item_id` | string (int64) | Postgres `catalog_items.id` |
| `namespace` | string | Redundant with stream name but kept for log greppability |
| `object_id` | string | Convenience for log readability |
| `strategy_id` | string | Active strategy at the time of ingest |
| `strategy_version` | string | Active strategy version at the time of ingest |
| `enqueued_at` | string (RFC 3339) | For p95 freshness latency calculation |

The embedder ignores `strategy_id`/`strategy_version` from the entry and re-resolves them from the namespace's *current* `catalog_strategy_id`+`version` at the moment of processing — this is what realises Q2's "new ingests are embedded under the *current* active version" behaviour even when the stream entry was enqueued under an older version.

### Reaper convention

- `XAUTOCLAIM` with `MIN-IDLE-TIME 60000` (60 s) reaps abandoned in-flight entries.
- Entries that hit `attempt_count > catalog_max_attempts` are `XACK`-ed and the corresponding catalog_items row is moved to `state='dead_letter'`. The operator re-drives via the admin endpoint, which sets the row back to `pending`, resets `attempt_count`, and `XADD`s a fresh entry.

---

## 6. Strategy abstraction (Go contracts in `internal/core/embedstrategy/`)

```go
// Strategy is the V1 contract every embedding implementation honours.
// V2 external-LLM strategies satisfy the same interface — that is the
// FR-021 / SC-009 forward-compat seam.
type Strategy interface {
    ID() string                                                    // immutable per registration
    Version() string                                               // immutable per registration
    Dim() int                                                      // produced vector length
    MaxInputBytes() int                                            // 0 = no per-strategy cap
    Embed(ctx context.Context, content string) ([]float32, error)  // returns L2-normalised vector
}

// Params is the FR-007 extension slot. V1 hashing reads nothing from it.
type Params map[string]any

// Factory builds a Strategy for a given Params (e.g. for V2 strategies that
// resolve credentials at construction time).
type Factory func(p Params) (Strategy, error)

// Registry is process-singleton: strategies self-register from init().
type Registry struct { /* unexported */ }

func (r *Registry) Register(id, version string, f Factory)
func (r *Registry) Build(id, version string, p Params) (Strategy, error)
func (r *Registry) Has(id, version string) bool
func (r *Registry) List() []StrategyDescriptor  // for admin UI: list available strategies

type StrategyDescriptor struct {
    ID, Version string
    Dim         int
    Description string
}

// Sentinel errors stable for tests and admin UI mapping.
var (
    ErrUnknownStrategy   = errors.New("embedstrategy: unknown id/version")
    ErrDimensionMismatch = errors.New("embedstrategy: produced dim != namespace embedding_dim")
    ErrZeroNorm          = errors.New("embedstrategy: produced zero-norm vector")
)
```

The V1 hashing strategy registers itself in `internal/embedder/hashing.go` `init()`:

```go
func init() {
    embedstrategy.DefaultRegistry().Register(
        "internal-hashing-ngrams", "v1",
        func(p Params) (Strategy, error) { return newHashingNgrams(p) },
    )
}
```

Adding a V2 OpenAI strategy is a new file `internal/embedder/openai.go` with its own `init()` registering `("external-openai-embedding-3","large")` — no other code changes anywhere in the repository.

---

## 7. Identity & uniqueness summary

| Identity | Source |
|---|---|
| Catalog item record | `catalog_items.id` (BIGSERIAL) |
| Catalog item business key | `(namespace, object_id)` UNIQUE |
| Qdrant point id | Resolved through existing `id_mappings` table — same as BYOE flow |
| Embedding job (transient) | Redis Streams entry id (`<ms>-<seq>`) |
| Strategy descriptor | `(strategy_id, strategy_version)` |

---

## 8. Volume / scale assumptions for V1

- **Items per namespace**: up to 10⁷ catalog rows per namespace (matches the spec's SC-006 "10,000 items" example, with 3 orders of magnitude headroom). Postgres handles this comfortably with the indexes above.
- **Steady-state ingest rate per namespace**: 1k items / second worst case. At ~1 KiB content average × 1k/s = 1 MiB/s of stream traffic per namespace — well within Redis Streams capacity.
- **Per-item embedding latency**: target p95 < 5 ms for V1 hashing strategy on 1 KiB content. Validated in benchmarks during implementation; if exceeded, FR-015 throughput indicators will surface it before SC-002 is missed.
- **Strategy registry**: O(10) entries lifetime; map lookup, no scaling concern.
