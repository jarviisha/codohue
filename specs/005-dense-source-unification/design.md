# Design: Unify `dense_strategy` + `catalog_enabled` into a single `dense_source` enum

Status: Draft (proposal)
Author: design sketch, pending review

## Problem

The "source of object dense vectors" is currently spread across **two independent
fields** on `namespace_configs`:

- `dense_strategy` ∈ `{disabled, item2vec, svd, byoe}`
- `catalog_enabled` ∈ `{true, false}`

These two are **not actually independent**. Catalog auto-embedding is a *fifth*
producer of object dense vectors, ranking alongside item2vec / svd / byoe. Because
it is modeled as a separate boolean, the two fields can describe a contradictory
state (cron Phase 2 and the embedder both writing `{ns}_objects_dense`), so the
code carries a **cross-field constraint** to forbid it:

- `internal/nsconfig/service.go:62,133` — `conflictsWithCatalog()` rejects enabling
  catalog unless `dense_strategy ∈ {byoe, disabled}`.
- `internal/nsconfig/types.go:71` — `DenseStrategyConflictError` plus its plumbing in
  `internal/admin/handler.go`, `cmd/admin/nsconfig_adapter.go`,
  `cmd/admin/catalog_adapter.go`.

Two concrete smells fall out of this:

1. **`byoe` is forced as a misnomer.** Enabling catalog requires `dense_strategy=byoe`
   ("bring your own embedding"), but in catalog mode nobody brings their own — the
   embedder *produces* the vectors. `byoe` is borrowed only for its side effects
   (cron Phase 2 skipped, dense-retrieval still active). See
   `internal/recommend/service.go:315`, where dense blending turns on for any
   `dense_strategy != "" && != "disabled"`; catalog needs `byoe` rather than
   `disabled` purely so its vectors get read.

2. **Bootstrap friction.** An operator must set `embedding_dim` and
   `dense_strategy=byoe` (in the right order) *before* the separate
   `PUT /api/admin/v1/namespaces/{ns}/catalog` call will succeed, or they hit a
   `400 dense_strategy_conflict` / `400 dimension mismatch`.

## Proposal

Collapse the two fields into a single mutually-exclusive enum that names the object
dense-vector producer directly:

```
dense_source ∈ {
    "disabled"   // no dense vectors anywhere; recommend runs sparse-only
    "item2vec"   // cron Phase 2 trains object + subject vectors
    "svd"        // cron Phase 2 trains object + subject vectors
    "byoe"       // client PUTs vectors for both object and subject
    "catalog"    // embedder auto-embeds objects from content; subjects via BYOE PUT
}
```

Selecting `catalog` **is** enabling catalog. The cross-field constraint becomes
impossible to violate (you cannot pick two sources), so it is deleted rather than
enforced.

> **Note (rejected alternative).** An earlier sketch split this into two axes —
> `object_dense_source` + `subject_dense_source`. Rejected: `item2vec`/`svd` train
> object *and* subject vectors in a single Phase 2 pass
> (`internal/compute/job.go:450-456`), so independent axes would admit nonsensical
> combinations (object=item2vec, subject=svd). One axis matches the compute reality.

### Decision table

The two old fields drove three scattered decisions. Unified:

| `dense_source` | Cron Phase 2 trains? | Recommend blends dense? | Embedder embeds objects? | Blocks BYOE object PUT? |
| -------------- | -------------------- | ----------------------- | ------------------------ | ----------------------- |
| `disabled`     | no                   | **no**                  | no                       | no                      |
| `item2vec`     | **yes**              | yes                     | no                       | no                      |
| `svd`          | **yes**              | yes                     | no                       | no                      |
| `byoe`         | no                   | yes                     | no                       | no                      |
| `catalog`      | no                   | **yes**                 | **yes**                  | **yes**                 |

Key separation: "blends dense" is decoupled from "cron trains". That decoupling is
exactly what `catalog` previously had to fake by borrowing `byoe`.

`alpha` stays a separate field controlling the sparse/dense blend ratio. It is **not**
folded into `dense_source`.

## Migration

Repurpose the existing `dense_strategy` column (rename for honest naming) and drop
`catalog_enabled`. Catalog wins the backfill because it is the real object dense
producer.

```sql
-- migrations/016_dense_source.up.sql
ALTER TABLE namespace_configs RENAME COLUMN dense_strategy TO dense_source;

UPDATE namespace_configs SET dense_source = 'catalog' WHERE catalog_enabled = TRUE;

ALTER TABLE namespace_configs DROP COLUMN catalog_enabled;

ALTER TABLE namespace_configs
  ADD CONSTRAINT dense_source_chk
  CHECK (dense_source IN ('disabled','item2vec','svd','byoe','catalog'));
```

The `catalog_strategy_id / catalog_strategy_version / catalog_strategy_params /
catalog_max_attempts / catalog_max_content_bytes` columns are **kept** — they remain
the parameters of the `catalog` source, meaningful only when
`dense_source = 'catalog'`.

### Phased rollout (no big-bang)

`dense_strategy` is a hot column with many readers; stage the change so no reader
breaks mid-deploy:

1. **Add + dual-write.** Add `dense_source`, backfill, and have the write path set
   both old and new columns. All readers still work.
2. **Migrate readers.** Move each reader (table below) onto `dense_source`.
3. **Drop.** Remove `catalog_enabled` and delete the cross-constraint code.

## Code touchpoints

### Delete (the payoff)

| Location | Remove |
| -------- | ------ |
| `internal/nsconfig/service.go:62,133` | `conflictsWithCatalog()` + both validation branches |
| `internal/nsconfig/types.go:71-79` | `DenseStrategyConflictError` |
| `internal/admin/handler.go:182-188,266-272` | both conflict error-mapping blocks |
| `internal/admin/types.go:184-189` | conflict error type |
| `cmd/admin/nsconfig_adapter.go:66-70`, `cmd/admin/catalog_adapter.go:86-98` | conflict plumbing |

The constraint disappears structurally: two sources can no longer be selected at once.

### Rewrite (condition changes)

| Location | Current | New |
| -------- | ------- | --- |
| `internal/compute/job.go:223` | `!= "" && != "byoe" && != "disabled"` | `source ∈ {item2vec, svd}` |
| `internal/recommend/service.go:315` | `!= "" && != "disabled"` | `source != "disabled"` (catalog included) |
| `internal/recommend/service.go:164` | `CatalogEnabled && obj` | `source == "catalog" && obj` |
| `internal/catalog/service.go:84` | `!CatalogEnabled` → 404 | `source != "catalog"` → 404 |
| `internal/embedder/service.go:168` | `!CatalogEnabled` | `source != "catalog"` |
| `internal/compute/job.go:425` | `switch cfg.DenseStrategy` | `switch cfg.DenseSource` (item2vec/svd cases only) |

### Rename (mechanical)

- `ListCatalogEnabled()` → `ListCatalogNamespaces()`, query `WHERE dense_source = 'catalog'`:
  `internal/nsconfig/repository.go:183,199`, `internal/nsconfig/service.go:26,105`,
  `internal/embedder/worker.go:45,162`, `internal/embedder/backlog_sampler.go:123`.
- `namespace.Config.DenseStrategy` / `.CatalogEnabled` → `.DenseSource`:
  `internal/core/namespace/types.go:20,32` and every read site.
- All `dense_strategy` / `catalog_enabled` SQL column references in
  `internal/nsconfig/repository.go` and `internal/admin/repository.go`.

## Wire contract & frontend impact

- `dense_strategy` and `catalog_enabled` are surfaced by the admin API
  (`internal/admin/types.go:22,29,328`). Removing/renaming them is a **breaking wire
  change**. Confirm whether these types live in `pkg/codohuetypes`; if so, regenerate
  the golden snapshots (`go test ./pkg/codohuetypes/... -run Golden -update`). The grep
  suggests they are `internal/admin`-only, which keeps the blast radius smaller.
- `web/admin` catalog form: replace the separate "enabled" toggle + `dense_strategy`
  dropdown with a single `dense_source` dropdown. The strategy_id / params inputs render
  only when `catalog` is selected. This is the largest DX win — one control instead of
  two coupled ones in a required order.
- The original "why two endpoints?" friction softens: `dense_source='catalog'` plus the
  strategy params can be set in a single call. The two reasons the catalog write path was
  split out (registry dependency + clobber protection) remain internal implementation
  choices, no longer a mandatory ordered two-call sequence for the operator.

## Risks / open questions

1. **Subject vectors in catalog mode.** `catalog` skips Phase 2, so subject dense vectors
   exist only if PUT via `/subjects/{id}/embedding`. The enum does not regress this, but
   the doc should state plainly: `catalog` = objects automatic, subjects manual.
2. **Demo/seed data** (`internal/admin/demo.go:49`) sets `DenseStrategy: "disabled"` — update
   to `DenseSource`.
3. **Loud-failure guards to add:** the `CHECK` constraint above, plus golden-snapshot
   coverage, so a bad value or accidental contract change surfaces immediately.
4. **`alpha` interaction** unchanged but worth a regression test: `catalog` + `alpha=1.0`
   should behave like sparse-only (dense weight zero) even though dense vectors exist.

## Out of scope

- Folding the catalog strategy params into the generic namespace upsert (separate decision;
  this design only removes the *enum-level* coupling).
- Adding model-backed embedding strategies to the registry (only `internal-hashing-ngrams@v1`
  ships today).
- Any change to `alpha`, sparse CF, or trending.
