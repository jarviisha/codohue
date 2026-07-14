# Feature Specification: Dense Source Unification

**Feature Branch**: `feat/compute-dense-source-unification`  
**Created**: 2026-06-19  
**Status**: Draft  
**Input**: User description: "Unify the namespace dense-vector configuration by collapsing the two coupled fields `dense_strategy` and `catalog_enabled` into a single mutually-exclusive enum `dense_source` with values {disabled, item2vec, svd, byoe, catalog}."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Choose the object dense-vector producer in one decision (Priority: P1)

An operator configuring a namespace wants to pick how object dense vectors are produced
by making **one** choice from a single list, instead of coordinating two separate settings
in a required order. Selecting "catalog" enables catalog auto-embedding directly — there is
no separate enable toggle and no obligation to first set an unrelated-sounding value.

**Why this priority**: This is the core value of the change — it removes the configuration
friction and the confusing "borrowed" setting that the current two-field model imposes. It
also delivers the cleanup that makes every other story possible.

**Independent Test**: On a fresh namespace, set the dense source to "catalog" with the
required embedding parameters in a single configuration action and confirm catalog ingest is
accepted and objects are auto-embedded — without separately toggling any "catalog enabled"
flag or pre-setting any other dense field.

**Acceptance Scenarios**:

1. **Given** a newly created namespace, **When** the operator sets the dense source to
   "catalog" with a valid embedding dimension and strategy, **Then** the configuration is
   accepted and catalog content ingest for that namespace is allowed.
2. **Given** a namespace whose dense source is "catalog", **When** the operator inspects the
   configuration, **Then** exactly one field describes the producer ("catalog") and there is
   no separate enable flag or conflicting dense setting.
3. **Given** a namespace whose dense source is "item2vec" or "svd", **When** the batch
   recompute runs, **Then** object and subject dense vectors are trained as before.
4. **Given** a namespace whose dense source is "disabled", **When** recommendations are
   requested, **Then** results are served from sparse signals only with no dense blending.

---

### User Story 2 - Conflicting producers are impossible by construction (Priority: P2)

An operator must never be able to configure two producers that both write the same object
dense vectors (which previously risked silent overwrites between the batch recompute and the
auto-embedder). Because the producer is now a single mutually-exclusive choice, the
contradictory combination cannot be expressed at all, and the previous rejection/validation
for that combination is no longer needed.

**Why this priority**: It preserves the safety guarantee that motivated the original
cross-field constraint, but achieves it structurally rather than through runtime validation
the operator can trip over.

**Independent Test**: Attempt to configure both auto-embedding and a training strategy at
once; confirm the configuration model offers no way to select both, so no conflict error can
occur.

**Acceptance Scenarios**:

1. **Given** the namespace configuration interface, **When** the operator selects a dense
   source, **Then** only one producer can be active and selecting one excludes the others.
2. **Given** a namespace set to "catalog", **When** a client attempts to push its own object
   dense vector directly, **Then** the request is rejected because catalog is the authoritative
   producer of object dense vectors.
3. **Given** a namespace set to "byoe", **When** a client pushes its own object or subject
   dense vector, **Then** the request is accepted.

---

### User Story 3 - Existing namespaces keep their behavior after migration (Priority: P3)

Operators with namespaces already configured under the old two-field model expect their
recommendation behavior to be unchanged after the system migrates to the single-field model.

**Why this priority**: Correct data migration is required for adoption, but it is a one-time
transition rather than ongoing user-facing value, so it ranks below the primary capability.

**Independent Test**: Take a representative set of pre-migration namespaces (catalog on,
item2vec, svd, byoe, disabled), run the migration, and confirm each one's recommendation and
embedding behavior is identical before and after.

**Acceptance Scenarios**:

1. **Given** a pre-migration namespace with catalog auto-embedding on, **When** migration
   runs, **Then** its dense source becomes "catalog" and catalog ingest plus auto-embedding
   continue to work.
2. **Given** a pre-migration namespace with a training strategy or byoe or disabled and
   catalog off, **When** migration runs, **Then** its dense source equals that prior value and
   behavior is unchanged.
3. **Given** any namespace, **When** an invalid dense source value is written, **Then** the
   system rejects it.

---

### Edge Cases

- What happens when an operator selects "catalog" but omits the embedding dimension or the
  strategy parameters? The configuration is rejected with a clear, actionable message naming
  the missing/mismatched value.
- How does the system handle a namespace whose object dense vectors already exist at one
  embedding dimension when the dense source is changed to a producer at a different dimension?
  The dimension coupling is validated and the change is rejected rather than silently producing
  mismatched vectors.
- What happens to a namespace set to "catalog" that never receives any catalog content? No
  object dense vectors are produced; recommendations fall back to available signals without
  error.
- What happens to subject dense vectors under "catalog"? They are not produced automatically;
  they exist only if supplied externally. This is unchanged from prior behavior and must be
  documented for operators.
- What happens when a client requests recommendations with a full-weight sparse blend while
  the dense source still produces vectors? Dense vectors are effectively unused for blending,
  matching sparse-only behavior.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The namespace configuration MUST express the producer of object dense vectors as
  a single field with the mutually-exclusive values: disabled, item2vec, svd, byoe, catalog.
- **FR-002**: The configuration model MUST NOT contain a separate boolean that independently
  enables catalog auto-embedding; selecting the "catalog" value MUST be the sole way to enable
  it.
- **FR-003**: When the dense source is "item2vec" or "svd", the system MUST train object and
  subject dense vectors during batch recompute; for all other values it MUST NOT.
- **FR-004**: When the dense source is any value other than "disabled", the system MUST allow
  dense vectors to participate in recommendation blending; when "disabled", it MUST NOT blend
  dense signals.
- **FR-005**: When the dense source is "catalog", the system MUST accept catalog content
  ingest for the namespace and auto-embed it into object dense vectors; for all other values it
  MUST reject catalog ingest for that namespace.
- **FR-006**: When the dense source is "catalog", the system MUST reject client attempts to
  directly supply object dense vectors (catalog is authoritative); for all other values that
  allow externally-supplied vectors, it MUST accept them.
- **FR-007**: The system MUST reject any configuration whose dense source value is outside the
  defined set.
- **FR-008**: When enabling "catalog", the system MUST validate that the embedding dimension
  and the selected strategy are consistent, and reject inconsistent configurations with a
  message that names both conflicting values.
- **FR-009**: The system MUST migrate every existing namespace to the single-field model such
  that prior recommendation and embedding behavior is preserved: namespaces with catalog
  auto-embedding on become "catalog"; all others adopt their prior dense strategy value.
- **FR-010**: The system MUST remove the prior cross-field conflict validation and its
  associated error surface, since the contradictory state can no longer be expressed.
- **FR-011**: The configuration surface exposed to operators MUST present the producer choice
  as one selection, and MUST only request strategy parameters when "catalog" is chosen.
- **FR-012**: The external configuration contract MUST be updated coherently so that consumers
  observe the single producer field rather than the two former fields, and any pinned contract
  snapshots MUST be regenerated to reflect the intended change.
- **FR-013**: The blend-ratio control and the catalog strategy parameters MUST remain separate
  configurable concerns and MUST NOT be folded into the producer field.

### Key Entities *(include if feature involves data)*

- **Namespace dense configuration**: The per-namespace settings that determine how dense
  vectors are produced and used. Key attributes: the single producer field (dense source), the
  embedding dimension, the blend ratio (separate), and the catalog strategy parameters
  (meaningful only when the producer is "catalog"). Replaces the former pair of a producer
  strategy field plus an independent catalog-enabled flag.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can enable catalog auto-embedding on a brand-new namespace in a
  single configuration action, with zero required preconditioning of other dense fields.
- **SC-002**: It is impossible to configure a namespace such that two producers write object
  dense vectors; the contradictory configuration cannot be submitted.
- **SC-003**: 100% of existing namespaces retain identical recommendation and embedding
  behavior after migration, verified across all five prior configurations (catalog, item2vec,
  svd, byoe, disabled).
- **SC-004**: The number of distinct configuration fields an operator must reason about to
  choose a dense producer drops from two to one.
- **SC-005**: No configuration request can result in the former "dense strategy conflicts with
  catalog" rejection, because that state is unrepresentable.
- **SC-006**: Any attempt to set an out-of-range producer value is rejected 100% of the time.

## Assumptions

- The five producer values (disabled, item2vec, svd, byoe, catalog) cover all current and
  near-term needs; no additional producers are introduced by this feature.
- item2vec and svd inherently produce both object and subject dense vectors in a single
  recompute pass, so a single producer field (rather than separate object/subject fields)
  matches system behavior. The two-axis alternative is explicitly rejected for this reason.
- Subject dense vectors under "catalog" continue to be externally supplied or absent; this
  feature does not add automatic subject embedding.
- The migration runs once against existing data; namespaces are not expected to be mid-write
  during the cutover, and a phased rollout (add new field and dual-write, migrate readers,
  then drop the old fields) is acceptable to avoid breakage.
- The existing catalog strategy registry and embedding parameters are reused unchanged; this
  feature only changes how the producer is selected, not how embedding is performed.
- The hand-written design sketch at `specs/005-dense-source-unification/design.md` is the
  source material and reflects the intended mapping and touchpoints.
