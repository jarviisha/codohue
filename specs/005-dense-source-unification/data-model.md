# Phase 1 Data Model: Dense Source Unification

## Entity: Namespace dense configuration

Subset of `namespace_configs` relevant to this feature. Only the producer selection changes;
embedding dimension, blend ratio, and catalog strategy parameters are retained as-is.

### Before (current)

| Column | Type | Values | Role |
|--------|------|--------|------|
| `dense_strategy` | TEXT | `disabled` \| `item2vec` \| `svd` \| `byoe` | Producer of dense vectors via cron / external |
| `catalog_enabled` | BOOLEAN | `true` \| `false` | Independently toggles catalog auto-embedding |
| `embedding_dim` | INTEGER | e.g. 64/128/256/512 | Dense vector dimension |
| `alpha` | FLOAT | `0.0`–`1.0` | Sparse/dense blend ratio (unchanged, separate) |
| `catalog_strategy_id` | TEXT | nullable | Catalog embedding strategy id |
| `catalog_strategy_version` | TEXT | nullable | Catalog embedding strategy version |
| `catalog_strategy_params` | JSONB | nullable | Strategy params (e.g. `{"dim":128}`) |
| `catalog_max_attempts` | INTEGER | default 5 | Retry budget before dead-letter |
| `catalog_max_content_bytes` | INTEGER | default 32768 | Per-ns content cap |

Cross-field invariant enforced in code: `catalog_enabled = TRUE` ⇒ `dense_strategy ∈ {byoe, disabled}`.

### After (target)

| Column | Type | Values | Role |
|--------|------|--------|------|
| `dense_source` | TEXT, `CHECK IN (...)` | `disabled` \| `item2vec` \| `svd` \| `byoe` \| `catalog` | **Single** producer selection |
| `embedding_dim` | INTEGER | unchanged | Dense vector dimension |
| `alpha` | FLOAT | unchanged | Sparse/dense blend ratio |
| `catalog_strategy_id` | TEXT | unchanged | Meaningful only when `dense_source='catalog'` |
| `catalog_strategy_version` | TEXT | unchanged | " |
| `catalog_strategy_params` | JSONB | unchanged | " |
| `catalog_max_attempts` | INTEGER | unchanged | " |
| `catalog_max_content_bytes` | INTEGER | unchanged | " |

`catalog_enabled` is **dropped**. The cross-field invariant is gone — it cannot be violated
because two producers can no longer be selected.

## Value semantics (the decision table, normative)

| `dense_source` | Cron Phase 2 trains object+subject? | Recommend blends dense? | Embedder embeds objects? | Blocks BYOE object PUT? | Subject vectors |
|----------------|-------------------------------------|-------------------------|--------------------------|-------------------------|-----------------|
| `disabled`     | no  | no  | no  | no  | none |
| `item2vec`     | yes | yes | no  | no  | trained (mean-pool) |
| `svd`          | yes | yes | no  | no  | trained (mean-pool) |
| `byoe`         | no  | yes | no  | no  | external PUT |
| `catalog`      | no  | yes | yes | yes | external PUT or none |

## Migration mapping

```
pre.catalog_enabled = TRUE                       → dense_source = 'catalog'
pre.catalog_enabled = FALSE (or NULL)            → dense_source = pre.dense_strategy
```

Backfill is deterministic and total (every row maps to exactly one valid value). Because the
old invariant guaranteed `catalog_enabled=TRUE ⇒ dense_strategy ∈ {byoe,disabled}`, no row can
have catalog on *and* a training strategy, so "catalog wins" never silently discards an
item2vec/svd selection.

**Mechanism (phased):** Migration 016 only *adds* `dense_source` and backfills it; `dense_strategy`
and `catalog_enabled` remain during the dual-write window. Migration 017 *drops* the two legacy
columns after every reader is on `dense_source`. The end-state schema in the "After" table is
reached only once 017 has run.

## Validation rules

- VR-1: `dense_source` MUST be one of the five values (DB `CHECK` + service-level guard).
- VR-2: Setting `dense_source='catalog'` MUST require a valid `catalog_strategy_id` /
  `_version` / `_params`, and `embedding_dim` MUST equal the strategy's output dimension;
  inconsistency rejected with both dimensions named (existing `DimensionMismatchError` reused).
- VR-3: There is no `dense_source` value that simultaneously enables two producers, so the
  former `DenseStrategyConflictError` is removed (no longer reachable).
- VR-4: Changing `dense_source` to a producer at a dimension different from existing Qdrant
  vectors MUST be rejected (existing dimension-coupling behavior preserved).

## State / transitions

`dense_source` is a plain configuration value, not a state machine. Allowed transitions: any
value → any value, subject to VR-1/VR-2/VR-4 at write time. Switching away from `catalog`
stops new auto-embedding but does not retroactively delete existing object dense vectors (same
as today); switching to a training strategy will cause the next cron Phase 2 to overwrite object
vectors, which is now an explicit, single-field operator choice rather than a guarded combination.
