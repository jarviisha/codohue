# Data Model: Backend Audit Remediation

## NamespaceLifecycle

Persistent tombstone for every namespace name ever activated.

| Field | Type | Rules |
|-------|------|-------|
| `namespace` | text | Primary key; canonical namespace name |
| `generation` | bigint | Starts at 1; increments only on recreation after `deleted` |
| `state` | text | `active`, `deleting`, or `deleted` |
| `activated_at` | timestamp | Start of the current generation |
| `legacy_messages_allowed` | boolean | True only for explicitly grandfathered generation 1 |
| `last_error` | text nullable | Most recent resumable delete failure |
| `updated_at` | timestamp | State transition time |

Unique identity: `(namespace,generation)`.

State transitions:

```text
absent ──create──> active(generation=1)
active ──delete──> deleting ──verified cleanup──> deleted
deleting ──retry──> deleting ──verified cleanup──> deleted
deleted ──recreate──> active(generation+1)
```

No direct `deleting -> active` transition exists. Recreation is forbidden until deletion is
verified. Config updates do not change generation.

## SystemLifecycle

Singleton durable gate for application-wide maintenance.

| Field | Type | Rules |
|-------|------|-------|
| `singleton` | boolean | Primary key constrained to true |
| `state` | text | `active` or `resetting` |
| `last_error` | text nullable | Most recent reset failure |
| `updated_at` | timestamp | State transition time |

`resetting` blocks namespace creation and every namespace mutation even if the process that
started reset has exited. Only verified completion returns it to `active`.

## NamespaceConfig generation

`namespace_configs` gains `generation BIGINT NOT NULL`. It references the matching
`NamespaceLifecycle` identity and is returned by internal config readers and namespace
creation/detail contracts. Existing rows backfill to generation 1.

Namespace-owned PostgreSQL tables retain their namespace column and gain validated
`ON DELETE CASCADE` foreign keys to the active config. The lifecycle tombstone is not cascaded.
Before validation, migration preflight reports and removes or quarantines existing orphans.

## Lifecycle lease

An in-memory value backed by a session- or transaction-scoped PostgreSQL advisory lock.

| Attribute | Meaning |
|-----------|---------|
| Namespace | Namespace covered by the lease |
| Generation | Generation re-read after lock acquisition |
| Mode | Shared writer or exclusive lifecycle mutation |
| Global mode | Shared normal operation or exclusive reset |

Acquisition order is fixed: global lease, namespace lifecycle lease, then existing compute
lock if needed. A lease stored in context supports nested calls without reacquiring and lets
ID mapping mutations assert that a matching lease exists.

## Stream envelope generation

The existing event, catalog, and embed payloads gain an additive
`namespace_generation` integer.

Validation:

- Exact active generation: process normally.
- Missing generation: process only when lifecycle generation is 1 and
  `legacy_messages_allowed=true`.
- Mismatch, `deleting`, or `deleted`: permanent stale work; ACK/drop and metric.
- Lifecycle store unavailable: transient failure; do not ACK.

Global streams remain shared. Per-namespace embed streams use a generation-qualified physical
name after generation 1.

## Generation-aware physical name

One helper maps `(logical kind, namespace, generation)` to Redis keys and Qdrant collections.
Generation 1 preserves existing names to avoid copying current production data. Generation 2+
uses a generation marker so late artifacts cannot be visible to current readers.

Kinds include recommendation cache, trending, embed stream, subjects, objects,
subjects-dense, and objects-dense. Deleted-generation artifacts are janitor candidates.

## EmbeddingStrategyIdentity

Value object containing `strategy_id` and `strategy_version`. Equality requires both fields.
Re-embed batch runs already persist both; catalog item stale/embedded predicates use tuple
comparison including NULL-safe mismatch semantics.

## CatalogReconciliationCursor

Opaque, URL-safe cursor containing:

| Field | Type | Rules |
|-------|------|-------|
| `updated_at` | timestamp | Last returned row timestamp |
| `id` | bigint | Last returned row primary key |
| `version` | integer | Cursor encoding version, initially 1 |

Rows after a cursor satisfy `updated_at > t OR (updated_at = t AND id > id)`, ordered by the
same tuple. The response omits `next_cursor` on the terminal page.

## IDMappingRepairRun

Durable, resumable record for migration-022 reconciliation.

| Field | Type | Rules |
|-------|------|-------|
| `id` | bigint | Primary key |
| `state` | text | `audited`, `snapshotting`, `applying`, `verifying`, `complete`, `failed` |
| `pg_snapshot_ref` | text nullable | Coordinated PostgreSQL backup reference |
| `qdrant_snapshot_refs` | json | Collection-to-snapshot reference map |
| `manifest_hash` | text | Hash of the immutable audited item set |
| `started_at` / `completed_at` | timestamp | Run lifecycle |
| `error` | text nullable | Last resumable failure |

Apply requires an immutable manifest and recorded snapshots. `failed` may resume from the
last item state; it does not roll back automatically.

## IDMappingRepairItem

One audited tuple per run and logical identity.

| Field | Type | Rules |
|-------|------|-------|
| `run_id` | bigint | FK to repair run |
| `namespace` | text | Logical namespace |
| `entity_type` | text | `subject` or `object` |
| `string_id` | text | Payload identity |
| `old_numeric_ids` | bigint array | IDs observed across stores |
| `target_numeric_id` | bigint nullable | Selected authoritative ID after audit |
| `sources` | json | PostgreSQL and collection evidence |
| `payload_hash` / `vector_hash` | text nullable | Preservation checks |
| `state` | text | `pending`, `copied`, `verified`, `cleaned`, `quarantined`, `failed` |
| `error` | text nullable | Collision or repair failure |

Primary key: `(run_id,namespace,entity_type,string_id)`. Missing payload IDs, conflicting
payloads at a target ID, or ambiguous vectors enter `quarantined` and halt apply.

Numeric IDs become unique within `(namespace,entity_type)` rather than globally, permitting
historically shared numeric IDs across independent Qdrant collections while preventing two
logical IDs in the same collection from colliding.
