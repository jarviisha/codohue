# Feature Specification: Backend Audit Remediation

**Feature Branch**: `fix/redis-backend-audit-remediation`
**Created**: 2026-08-24
**Status**: Draft
**Input**: User description: "Plan remediation for all audited project findings, excluding web/admin."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Keep the service secure and available (Priority: P1)

Operators can run Codohue continuously without reachable known dependency vulnerabilities or durable input queues eventually exhausting available memory during normal sustained traffic.

**Why this priority**: The current dependency findings expose reachable vulnerable symbols, while unbounded queues can stop all new writes once storage is exhausted.

**Independent Test**: Run the vulnerability scan with no reachable findings, then process sustained event, catalog, and embedding traffic and verify completed entries are reclaimed without losing entries that have not completed processing.

**Acceptance Scenarios**:

1. **Given** the production dependency set, **When** it is scanned against the current vulnerability database, **Then** no known vulnerability is reported on a reachable execution path.
2. **Given** a stream entry that has not completed processing, **When** retention runs, **Then** the entry remains available for recovery.
3. **Given** a stream entry successfully processed by all required consumers, **When** retention runs, **Then** the entry is eventually reclaimed and stream storage remains bounded under sustained traffic.
4. **Given** an outage that creates a processing backlog, **When** consumers recover, **Then** no unprocessed entry is removed solely because a fixed queue length was exceeded.

---

### User Story 2 - Preserve namespace lifecycle integrity (Priority: P1)

Operators can delete or reset namespaces knowing that concurrent ingest, embedding, and compute work cannot recreate data after the operation reports success.

**Why this priority**: A successful destructive operation must be authoritative; resurrected events, mappings, queues, or vectors create invisible cross-store inconsistency.

**Independent Test**: Continuously submit all supported write types while deleting a namespace, then verify that after success the namespace has no durable state, queued work, mappings, or vector collections and cannot accept further writes until explicitly recreated.

**Acceptance Scenarios**:

1. **Given** data-plane and background writes in flight, **When** namespace deletion begins, **Then** new writes for that namespace are rejected or fenced and in-flight writes cannot commit after the deletion boundary.
2. **Given** queued event or catalog work for a deleted namespace, **When** workers receive it, **Then** it cannot recreate namespace-owned state.
3. **Given** an application-wide reset, **When** it reports success, **Then** no writer or background job can recreate pre-reset state.
4. **Given** a deleted namespace name is later reused, **When** the new namespace starts receiving work, **Then** work from the previous lifecycle is not replayed into it.

---

### User Story 3 - Keep recommendation state consistent over time (Priority: P1)

Clients receive recommendations produced only from current, valid namespace configuration, item identity, embedding strategy, and timestamps.

**Why this priority**: Stale vectors, skipped re-embedding, unsafe timestamps, or obsolete identity mappings can silently return incorrect recommendations or make valid items unreachable.

**Independent Test**: Exercise event expiry, same-version strategy replacement, future creation timestamps, and pre-migration identity collisions; verify stale state is removed, affected items are rebuilt, scores remain finite, and all existing items remain addressable.

**Acceptance Scenarios**:

1. **Given** a configured namespace whose last event ages out of the active window, **When** scheduled maintenance runs, **Then** stale sparse and system-owned dense vectors are removed.
2. **Given** an embedding strategy identifier changes while its version string remains the same, **When** re-embedding is requested, **Then** every item not matching the full target identity is processed and completion reflects that full identity.
3. **Given** a creation timestamp beyond the permitted clock skew, **When** an event or embedding is submitted, **Then** it is rejected and cannot produce a non-finite recommendation score.
4. **Given** existing vector identities created before namespace-scoped mappings, **When** the mapping transition completes, **Then** catalog, client-provided, sparse, and computed dense vectors remain reachable by their string identifiers.
5. **Given** an identity transition has completed, **When** rollback is requested, **Then** the system either performs a verified lossless rollback or refuses before modifying state with an actionable recovery instruction.

---

### User Story 4 - Fail writes and reads honestly (Priority: P2)

Clients and operators receive a failure when required configuration or cleanup work fails, rather than a successful response backed by defaults, partially updated configuration, or orphaned vectors.

**Why this priority**: False success makes repair difficult and can cache or expose incorrect behavior for subsequent requests.

**Independent Test**: Inject configuration-store, dense-delete, attribution, and strategy-validation failures and verify no incorrect response is cached and no request reports full success after only a partial mutation.

**Acceptance Scenarios**:

1. **Given** namespace configuration cannot be loaded, **When** recommendation or ranking is requested, **Then** the request fails or returns an explicitly degraded non-cacheable response; it never silently uses defaults.
2. **Given** a nonexistent namespace and a globally privileged credential, **When** a data-plane mutation is attempted, **Then** no vector collection, mapping, object, or event is created.
3. **Given** sparse deletion succeeds but dense deletion fails, **When** object deletion completes, **Then** the caller receives a retryable failure and retrying converges safely.
4. **Given** a combined namespace and catalog configuration request is invalid, **When** it is rejected, **Then** no subset of the requested configuration change remains committed.
5. **Given** author attribution is requested with catalog content, **When** attribution cannot be persisted, **Then** the request does not report full success without a durable repair path.

---

### User Story 5 - Recover every pending item fairly and reconcile without gaps (Priority: P2)

Operators can rely on recovery workers and reconciliation pagination to eventually visit every eligible item, including large backlogs and rows sharing the same update time.

**Why this priority**: Cursor errors can strand work forever without producing a hard failure.

**Independent Test**: Create backlogs larger than one reclaim page with early permanently failing entries and create reconciliation pages that split equal timestamps; verify every later entry is eventually processed exactly through the supported idempotent path.

**Acceptance Scenarios**:

1. **Given** more pending entries than one reclaim batch, **When** repeated recovery passes run, **Then** the recovery cursor advances and later entries are not starved by earlier failures.
2. **Given** multiple catalog rows with an identical update time spanning page boundaries, **When** a caller resumes reconciliation, **Then** no row is skipped or duplicated because of the shared timestamp.
3. **Given** recovery is interrupted mid-scan, **When** it resumes, **Then** processing remains safe and converges without losing work.

---

### User Story 6 - Expose operations safely (Priority: P3)

Operators retain health and metrics visibility without exposing namespace activity or internal dependency details to unauthenticated public clients.

**Why this priority**: The endpoints aid operations but currently disclose deployment and tenant information on the public API listener.

**Independent Test**: Access operational endpoints from trusted and untrusted network contexts and verify trusted monitoring remains functional while public unauthenticated access reveals no tenant or internal dependency details.

**Acceptance Scenarios**:

1. **Given** an untrusted client on the public API listener, **When** it requests metrics, **Then** namespace-labelled operational data is unavailable.
2. **Given** an untrusted client requests health status, **When** a dependency is unhealthy, **Then** no internal address, credential fragment, or raw dependency error is disclosed.
3. **Given** the trusted monitoring path, **When** health and metrics are collected, **Then** existing operational signals remain available.

### Edge Cases

- A consumer crashes after committing durable work but before acknowledging its stream entry.
- Multiple consumer groups have different acknowledgement progress on a shared stream.
- A namespace is deleted and recreated with the same name while old stream entries remain.
- A scheduled cleanup runs concurrently with a manual recompute or re-embed operation.
- Every event expires at the same maintenance boundary.
- A new strategy reuses a version label already used by another strategy.
- Creation time is exactly at the allowed future-skew boundary or contains a malformed timezone.
- Identity mappings contain cross-namespace and cross-entity collisions before migration.
- Dense storage is unavailable after sparse deletion has already succeeded.
- Reclaim and reconciliation cursors reach their terminal values with an empty final page.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST ship with no known reachable dependency vulnerability for which an applicable fixed release is available.
- **FR-002**: The system MUST bound completed stream history without deleting unprocessed or recoverable entries.
- **FR-003**: Stream retention MUST cover behavioral event, catalog transport, and per-namespace embedding queues under a documented ownership and consumer-progress policy.
- **FR-004**: Namespace deletion and application reset MUST establish a lifecycle boundary observed by every data-plane and background writer.
- **FR-005**: Work from an earlier namespace lifecycle MUST NOT mutate a later namespace created with the same name.
- **FR-006**: Scheduled maintenance MUST evaluate every configured namespace, including namespaces with zero events in the active window.
- **FR-007**: Scheduled maintenance MUST remove stale sparse and system-owned dense state when the authoritative keep set is empty.
- **FR-008**: Re-embed selection, progress, and completion MUST compare the full strategy identifier and version pair.
- **FR-009**: All client-controlled creation timestamps MUST use one documented clock-skew rule, and all scoring paths MUST remain finite for any stored payload.
- **FR-010**: The identity-mapping transition MUST include verified recovery for all existing vector producers, including producers that the server cannot recompute independently.
- **FR-011**: Identity-mapping rollback MUST be preflighted and MUST NOT leave a partially modified key scheme.
- **FR-012**: Required namespace configuration lookup failures MUST NOT silently fall back to defaults or populate recommendation caches.
- **FR-013**: Globally privileged credentials MUST NOT permit mutation of a namespace that does not exist.
- **FR-014**: Object deletion MUST report non-ignorable cleanup failures while remaining safe to retry.
- **FR-015**: Multi-part namespace configuration changes MUST validate completely before any requested field is committed.
- **FR-016**: Catalog content and requested attribution MUST have atomic success semantics or a durable, observable repair mechanism.
- **FR-017**: Pending-entry recovery MUST advance through the complete reclaim cursor space without starvation.
- **FR-018**: Catalog reconciliation MUST use a stable resume position that totally orders rows sharing the same update time.
- **FR-019**: Operational metrics containing namespace labels MUST be restricted to a trusted monitoring path.
- **FR-020**: Public health responses MUST be sanitized while trusted operators retain actionable diagnostics through protected channels.
- **FR-021**: Every changed business-logic component MUST have deterministic regression tests covering its failure and concurrency boundaries.
- **FR-022**: Remediation MUST NOT modify or require changes to `web/admin`.

### Key Entities

- **Namespace Lifecycle**: One generation of a namespace name, with states that determine whether writes and background work are allowed.
- **Stream Entry**: Durable unit of event, catalog, or embedding work with processing and acknowledgement progress.
- **Embedding Strategy Identity**: The immutable pair of strategy identifier and strategy version used to select and verify re-embedding work.
- **Vector Identity Mapping**: Association between namespace, entity type, string identifier, and numeric vector-store identifier.
- **Reconciliation Cursor**: Stable resume position that uniquely orders catalog changes.
- **Operational Endpoint**: Health or metric surface with a defined trust boundary and disclosure policy.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The release has zero reachable known dependency vulnerabilities in the supported production build at release time.
- **SC-002**: A sustained-load test processes at least ten retention windows while completed stream storage remains within the documented bound and 100% of unprocessed entries remain recoverable.
- **SC-003**: In 100 repeated delete/reset concurrency trials, zero post-success writes, vector collections, mappings, or queue entries from the deleted lifecycle remain.
- **SC-004**: After all events age out, the next scheduled maintenance cycle removes 100% of stale system-owned recommendation vectors for the namespace.
- **SC-005**: Replacing a strategy with another strategy at the same version label reprocesses 100% of mismatched items and reports accurate completion.
- **SC-006**: Every accepted recommendation response contains only finite scores under boundary and fuzzed timestamp inputs.
- **SC-007**: A migration rehearsal with cross-namespace and cross-entity identifier collisions preserves reachability of 100% of catalog, client-provided, sparse, and computed dense points.
- **SC-008**: Fault-injection tests produce zero cached responses based on missing configuration and zero successful object deletions with a known remaining dense point.
- **SC-009**: Recovery and reconciliation tests visit 100% of entries across at least three pages, including equal-timestamp boundaries and permanently failing early entries.
- **SC-010**: Unauthenticated public requests reveal zero namespace labels or raw dependency errors while trusted monitoring continues to collect all existing signals.
- **SC-011**: Full build, lint, unit, race, end-to-end, compose validation, migration rehearsal, and vulnerability checks pass before release.

## Assumptions

- The remediation covers Go services, SDKs, migrations, deployment configuration, and backend documentation; `web/admin` is explicitly excluded.
- Existing public data-plane response shapes remain backward compatible unless a security-safe failure response is required.
- At-least-once delivery remains the queue contract; consumers and writes continue to be idempotent.
- Operators can perform a staged rollout and maintenance window for identity migration when required.
- Trusted monitoring can use a private listener, private network, or authenticated route without requiring changes to the excluded web application.
- Dependency upgrades stay within compatible release lines unless testing demonstrates that a larger upgrade is required.
