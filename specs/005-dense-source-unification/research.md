# Phase 0 Research: Dense Source Unification

All Technical Context unknowns are resolved below. No `NEEDS CLARIFICATION` remain.

## R1 — Wire contract blast radius: is `pkg/codohuetypes` affected?

**Decision**: No. `dense_strategy` and `catalog_enabled` are **not** part of the locked client
wire contract in `pkg/codohuetypes`. They live only in `internal/admin/types.go` (admin DTOs)
and `internal/nsconfig` / `internal/core/namespace`.

**Evidence**: `grep` over `pkg/codohuetypes/` shows the only catalog-related file is
`catalog.go`, which carries the catalog **ingest** request/response (object_id, content,
metadata) — neither `dense_strategy` nor `catalog_enabled`. The golden snapshots in
`pkg/codohuetypes/testdata/` therefore do **not** cover these fields.

**Rationale**: The admin config surface is session-cookie-authed and admin-internal; it is not
the SDK-facing contract. So FR-012's "pinned contract snapshots MUST be regenerated" applies to
**admin-internal** snapshot/golden tests (if any under `internal/admin`), not to the
`codohuetypes` golden suite.

**Alternatives considered**: Treating it as a SDK-breaking change and bumping codohuetypes —
rejected, factually unnecessary; would force a spurious golden regen.

## R2 — Migration shape: rename-in-place vs add-column + drop

**Decision**: Add a new `dense_source` column (migration 016), backfill
`dense_source = CASE WHEN catalog_enabled THEN 'catalog' ELSE dense_strategy END`, and add a
`CHECK` constraint pinning the five values. Keep `dense_strategy` and `catalog_enabled` during a
dual-write window; drop them in a **second** migration (017) once every binary reads
`dense_source`.

**Rationale**: A `RENAME COLUMN` is incompatible with the phased rollout in R3 — once renamed, an
old binary still selecting `dense_strategy` breaks mid-deploy. Add-column + dual-write keeps both
readable until all four binaries upgrade, then 017 drops the legacy columns. Net end-state is
identical to a rename.

**Alternatives considered**:
- `RENAME COLUMN` in place — rejected, breaks rolling deploys because an un-upgraded binary still
  reads the old column name (see R3).
- New `dense_source` column kept alongside `dense_strategy` permanently — rejected, leaves two
  columns describing one concept, defeating the feature.

## R3 — Phased rollout vs single-deploy cutover

**Decision**: Three-phase rollout — (1) add `dense_source` + dual-write both columns, (2)
migrate every reader onto `dense_source`, (3) drop `catalog_enabled` + delete conflict-validation
code. For a rename-based migration this collapses to: ship the migration and the reader changes
together in one release **only if** deploy is atomic; otherwise stage so no running binary reads
a column that has been renamed out from under it.

**Rationale**: `dense_strategy` / `catalog_enabled` are read by four binaries (api, cron, admin,
embedder). A hard rename in a rolling deploy would break an old binary still selecting the old
column name. The dual-write window keeps both readable until all binaries are upgraded.

**Alternatives considered**: Big-bang rename in one migration with simultaneous full redeploy —
acceptable only for environments that can tolerate a brief coordinated restart; documented as the
simpler path for single-node/dev. Production guidance is the phased approach.

## R4 — Subject dense vectors under `catalog`

**Decision**: Unchanged behavior — `catalog` skips batch Phase 2, so subject dense vectors are
produced **only** if supplied externally (the subject BYOE PUT is not blocked). Document this
explicitly; do not add automatic subject embedding (out of scope).

**Rationale**: This matches the current `catalog_enabled=true` + `dense_strategy=byoe` regime.
The unification must be behavior-preserving (SC-003), so no semantic change here.

**Alternatives considered**: Mean-pooling subject vectors from catalog object vectors — rejected,
new behavior, out of scope.

## R5 — Dense-blend gate wording for `catalog`

**Decision**: The recommend dense-blend gate becomes `dense_source != "disabled"` (combined with
the existing `0 < alpha < 1`). `catalog`, `byoe`, `item2vec`, `svd` all enable blending; only
`disabled` turns it off.

**Rationale**: Previously catalog had to masquerade as `byoe` precisely to satisfy the old gate
(`dense_strategy != "" && != "disabled"`). With `catalog` as a first-class value, the gate reads
naturally and the masquerade disappears — this is the central smell the feature removes.

**Evidence**: `internal/recommend/service.go:315` current condition; `:164` BYOE-object block.

**Alternatives considered**: An explicit allow-list `{item2vec,svd,byoe,catalog}` — equivalent,
but `!= disabled` is simpler and future-proof if more producers are added.

## R6 — Constitution alignment

**Decision**: Proceed; no governance gate tripped. The constitution's "exactly two binaries"
clause is already superseded by the documented four-binary architecture in CLAUDE.md, and this
feature adds none. All four Core Principles pass (see Constitution Check in plan.md).

**Rationale**: The change reduces coupling (deletes a cross-field constraint), stays within
existing domains, adds no endpoint, and touches a cold config path — aligning with Code Quality
(I) and not regressing Performance (IV).
