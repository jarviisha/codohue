# Namespace Settings redesign — implementation plan

Status: implemented and verified locally (2026-09-22).
Existing working branch: `fix/admin-console-operator-ux`.
Preserve existing uncommitted work, Astryx components and semantic design tokens.

## Agreed contract

- Four UI tabs, independently saved business groups; one configuration resource.
- Each group owns its draft, baseline and revision. Switching tabs preserves drafts.
- Saving a group changes only its fields and clears only its successful draft.
- Transactional consistency applies to the selected group and its dependent fields,
  not to all outstanding drafts across tabs.
- Leaving Settings or changing namespace prompts if any group has an unsaved draft.
- System Runtime remains read-only. Batch, re-embed and key rotation remain separate operations.

## Field ownership

| Group | Existing fields | Application guidance |
| --- | --- | --- |
| recommendations | alpha, gamma, max_results, seen_items_days, exclude_authored | New calculations; existing recommendation caches may delay visible changes |
| signals | action_weights, lambda | Sparse rebuild; action weights also affect trending |
| trending | trending_window, trending_ttl, lambda_trending | Next batch rebuild; expiry does not trigger recompute |
| embeddings | dense_source, embedding_dim, dense_distance, catalog_strategy_id/version/params, catalog_max_attempts, catalog_max_content_bytes | Producer-specific transition; compatibility checks; existing items may need re-embedding |

Each stored field has exactly one owning group. A source change must not silently
rewrite recommendations.alpha. Cross-group dependencies are validated against the
latest stored configuration and explained as warnings/errors when appropriate.

## API design

Use `/api/admin/v1/namespaces/{ns}/configuration`:

- GET: namespace identity/generation, four groups with revisions, stored values,
  field capabilities/lock reasons, inheritance metadata, and application guidance.
- PATCH: `{ "group": "trending", "generation": 1, "base_revision": 5,
  "changes": { "trending_window": 72, "trending_ttl": 3600 } }`.
- POST `/configuration/validation`: same candidate shape; validates without writes,
  job execution or reserving the revision. PATCH always revalidates.

Keep existing field names where possible; avoid unnecessary aliases in the new API.
PATCH responds with the canonical saved group, its new revision, relevant warnings
and required follow-up actions. No additional GET is required to confirm a save.

- Omitted field: unchanged.
- Explicit null: restore inheritance only for allowlisted nullable override fields.
- Null for non-inheritable fields, unknown groups/fields, or fields from another
  group: validation error. Typed parsing must distinguish missing/null/value.
- action_weights: replacement map within the signals patch; removal is explicit.
- No-op change: no revision increment and no rebuild recommendation.
- 409: generation/revision conflict with current group revision and canonical values.
- 422: field-addressed validation errors (`group.field`) and dependency details.
- Missing namespace: 404; PATCH must not implicitly create or reactivate it.
- Auth, RBAC and session CSRF match the existing administrative read/write routes.

Effective inherited defaults must come from authoritative process configuration.
Do not infer API or embedder defaults from admin's environment. If runtime reports
are missing or replicas disagree, return explicit unknown/mixed observations with
provenance; stored inheritance remains meaningful even without an observed value.

## Storage and concurrency

Add the next available migration (currently 029) for four positive bigint group
revision columns on namespace_configs, initialized to 1. Revisions are distinct
from the existing namespace lifecycle generation and updated_at timestamp.

Use a database trigger to increment only groups whose owned columns actually
change (`IS DISTINCT FROM` handles nullable overrides). This also covers existing
namespace/catalog writers and prevents revision bookkeeping from diverging.
Creation, no-op writes, multiple groups changed by legacy writes, and nullable
resets need explicit tests. Ignore timestamps/key rotation when computing group
changes. Recheck the column list whenever config fields are added.

For a new PATCH, use the existing lifecycle writer discipline and a short database
transaction: read/lock current row, check generation and selected group revision,
merge selected fields, validate cross-field invariants, update, return canonical
values/revision. Different groups may serialize briefly on the row but do not
conflict solely because another group changed. Same-group stale writes conflict.
Keep existing lifecycle/advisory/row lock ordering consistent to avoid deadlocks.
Qdrant, Redis and job enqueueing are not part of the PostgreSQL transaction; do
not claim distributed atomicity or destroy collections during configuration save.

Existing APIs remain wire-compatible and advance affected revisions through the
same persistence path/trigger. Legacy clients without a precondition remain
last-write-wins: their writes invalidate new clients' baselines, but cannot offer
full stale-write protection themselves. Document this limit and migration path.

## UI design

- Compact namespace header, one tab row with active/error/unsaved states.
- Main form plus contextual application guidance on desktop; one column on mobile.
- Relevant fields only for the selected embedding source. Catalog is edited inline
  in Embeddings, replacing the separate catalog configuration modal entry point.
- Per-tab sticky actions shown when dirty: changed-field count, Discard, optional
  Review changes panel, Save changes. Always identify the affected group.
- Tab switching is local navigation: retain every draft, do not display a leave prompt.
- URL-selectable tabs support Back/Forward and direct links. Old config/catalog
  anchors map to the corresponding tab; operational Catalog links remain valid.
- Numeric edits are tracked before blur; valid values commit before save; invalid
  text is explained rather than silently dropped from the pending-change count.
- Defaults use explicit Use system default / Override controls, showing effective
  value and provenance when available. No ambiguous blank-input semantics.
- Background refresh updates clean groups; dirty groups retain baseline and draft.
- Conflict panel compares baseline, current server values and draft. Preserve edits;
  allow explicit reconciliation rather than an automatic force overwrite.
- Save failure retains the draft. Success updates only that group's baseline.
- Application text distinguishes saved, rebuild required, job queued and job failed.
  Do not assert a revision is applied without consumer/job evidence.
- Keyboard tab navigation, visible focus, field-linked error summary, screen-reader
  status announcements and sticky controls that do not obscure focused fields.

## Delivery sequence and reviewable checkpoints

### 1. Contract and storage foundation

- [x] Finalize group ownership, transition rules and response/error examples.
- [x] Trace all writes, including provisioning, demo seeding, catalog and legacy routes.
- [x] Add migration, revision trigger and transaction/concurrency tests.
- [x] Define admin wire types; update pkg/codohuetypes/golden snapshots only if its
      public contract is extended. Do not silently change existing SDK responses.

Deliverable: revisions behave correctly without changing the current UI.

### 2. Unified configuration API

- [x] Add grouped read/patch/validation in internal/nsconfig service/repository.
- [x] Wire through cmd/admin/nsconfig_adapter.go, internal/admin handlers and router;
      preserve package boundaries rather than direct domain imports.
- [x] Implement strict field parsing, capability/lock reasons, null reset semantics,
      canonical save response and structured conflicts/errors.
- [x] Ensure validation and writes use shared rules; revalidate after locking.
- [x] Update ARCHITECTURE.md endpoint, storage and process-flow references.

Deliverable: API independently testable while old UI and routes still work.

### 3. Frontend data model and layout

- [x] Add typed configuration hooks and a namespace-scoped draft store per group.
- [x] Build tabs, contextual guidance and responsive field layouts with Astryx.
- [x] Add per-group actions, review panel, errors and conflict reconciliation.
- [x] Preserve unrelated drafts on save, refresh, conflict and validation errors.

Deliverable: independent tab saves with no lost draft or unnecessary conflict.

### 4. Embeddings/Catalog and operational follow-through

- [x] Move catalog settings into Embeddings using the grouped API.
- [x] Handle source transitions atomically with strategy/version, capabilities and
      explicit inheritance resets. Retain confirmations for disruptive actions.
- [x] Replace old editing links with tab deep links and remove duplicate forms.
- [x] Link to existing batch/re-embed flows; do not automatically run expensive jobs.
- [x] Finish accessible copy, units, locked-field reasons and application states.

Deliverable: one coherent settings surface; operations stay on their own pages.

### 5. Verification and rollout

- [x] Unit tests: field mapping, tri-state inputs, group validation, draft reducers.
- [x] Database tests: same-group conflict, different-group merge, cross-group
      dependency race, no-op, rollback, legacy writes and delete/recreate generation.
- [x] API tests: auth/CSRF, strict parsing, error contracts, canonical response,
      wrong generation, missing namespace and read-only validation.
- [x] Browser tests: all tab saves, multiple simultaneous drafts, Back/Forward,
      external edits, failure/retry, conflicts, defaults, numeric text and locks.
- [x] Visual/keyboard checks: 375/768/1440 widths, light/dark, 200% zoom and focus.
- [x] Run frontend lint/tests/build/browser; make test, make test-race and isolated
      make test-e2e. Keep real namespace data/configuration unchanged during tests.
- [x] Deploy additive migration first, compatible backend second, frontend last.
- [x] For application rollback, restore UI/backend while retaining additive revision
      columns/trigger. Do not roll the migration down while the new API is serving.
- [x] Rebuild local processes as needed; keep Vite dev available for user testing.

## Acceptance criteria

1. Save Trending never writes or clears drafts in the other three groups.
2. Two operators saving different valid groups both succeed; same-group stale
   writes return a conflict and preserve the losing draft.
3. Incompatible source/strategy/dimension combinations never partially persist.
4. A namespace deleted and recreated cannot accept a draft from its old generation.
5. Restore-default and retain-override are distinct, tested operations.
6. Every save response is canonical and identifies the saved group/revision.
7. A failed batch does not appear as failed configuration persistence.
8. Legacy APIs remain compatible, with their concurrency limitations documented.

## Outside this delivery

Dynamic system configuration, automatic restarts, ranking-quality simulation,
configuration history/rollback UI and end-to-end applied-revision tracking. Group
revisions provide a foundation for these but are not presented as those features.

## Implementation evidence

- Migration 029 applied to isolated test infrastructure and local stack.
- Grouped API implemented in nsconfig; admin bridge uses core namespace DTOs,
  without changing public SDK wire types.
- PostgreSQL tests cover same/different-group contention, no-op, validation-only,
  legacy invalidation, key rotation, nullable reset, atomic catalog transition,
  rollback of invalid candidates, latest cross-group validation and recreation.
- Frontend lint, unit tests, production build and browser suite pass. Browser
  coverage includes four tab saves, retained drafts, numeric text, duplicate actions,
  conflict review, save retry, inheritance, locks, URL history and navigation guard.
- Layout checked at 375/768/1440 in light/dark and 200% zoom; live Vite smoke checked
  desktop/mobile, grouped GET and validation-only POST without config writes.
- Full Go tests, race tests and isolated full E2E pass. Admin rebuilt locally;
  Vite remains on port 2006. No namespace settings changed by the live smoke check.
- Rollback procedure is documented in deploy/namespace-settings.md; rollback was
  not executed on the live stack.

Implementation details: successful PATCH returns the complete canonical projection,
so clean groups can refresh while dirty groups retain their own baselines. Validation
returns stored values after checking the candidate; it reserves no revision. Draft
helpers plus browser workflows cover the state model without introducing a global
store. Default observations explicitly retain unknown/mixed states and provenance.
