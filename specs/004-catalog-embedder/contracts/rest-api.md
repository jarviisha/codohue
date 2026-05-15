# Contract: REST API surface

**Feature**: 004-catalog-embedder
**Date**: 2026-05-09
**Status**: Plan-phase contract; canonical paths land in code at implementation time. Every endpoint below MUST appear in the REST API table in [/CLAUDE.md](../../../CLAUDE.md) before merge (Constitution III).

This contract covers two planes:

- **Data plane** (`cmd/api`, port 2001): the new `POST /v1/namespaces/{ns}/catalog` endpoint.
- **Admin plane** (`cmd/admin`, port 2002): namespace catalog config + operational catalog views.

A small change is also made to the existing `cmd/api` BYOE write endpoint to enforce the source-of-truth precedence policy (FR-018) — documented at the end.

---

## Data plane (cmd/api, port 2001)

### `POST /v1/namespaces/{ns}/catalog`

Ingest a catalog item for a namespace.

**Auth**: per-namespace bearer key (existing data-plane scheme; falls back to `CODOHUE_ADMIN_API_KEY` only when the namespace has no provisioned key, identical to the existing event-ingest semantics).

**Request body**:

```json
{
  "object_id": "post_42abc",
  "content": "Hôm nay trời đẹp quá, ai cũng muốn ra biển! #weekend",
  "metadata": {
    "author_id": "user_77",
    "language_hint": "vi",
    "post_kind": "short"
  }
}
```

| Field | Required | Notes |
|---|---|---|
| `object_id` | yes | Stable client-side string id. Together with `{ns}` from the URL forms the unique key. |
| `content` | yes | The exact text fed to the embedder. Used to compute `content_hash` (FR-002). MUST NOT be empty after trimming. MUST NOT exceed `namespace_configs.catalog_max_content_bytes` (default 32 KiB) — see 413 below. |
| `metadata` | no | Arbitrary JSON object. Stored verbatim. **Does not feed the embedder** and **does not contribute to the content hash** (Q4 / FR-002). |

`namespace` MUST NOT appear in the body — the URL is the only namespace declaration (consistent with the 003 RESTful redesign decision).

**Responses**:

| Status | Body shape | When |
|---|---|---|
| `202 Accepted` | empty | Item accepted; embedding work is asynchronous. The HTTP call MUST NOT block on embedding (FR-003). |
| `400 Bad Request` | `{"error":"..."}` | Invalid JSON, missing `object_id`, missing or empty `content`, malformed `metadata`. |
| `401 Unauthorized` | `{"error":"..."}` | Missing or wrong bearer token. |
| `404 Not Found` | `{"error":"namespace not found or catalog auto-embedding not enabled"}` | The namespace does not exist OR its `catalog_enabled=false`. **Same status for both** so unauthenticated probes can't enumerate namespaces (FR-008). |
| `413 Payload Too Large` | `{"error":"content exceeds catalog_max_content_bytes (limit=NNN, got=MMM)"}` | `len(content) > catalog_max_content_bytes` (FR-020 / R9). |
| `422 Unprocessable Entity` | `{"error":"content is empty after trimming"}` | Content is whitespace-only or empty. |

**Idempotency**: Repeated `POST` of the same `object_id` with the same `content` (under the same active strategy version) is a no-op at the embedding layer (FR-002). The HTTP response is still `202 Accepted` to keep client logic simple.

**Side effects on success (state transitions)**:

1. INSERT or UPDATE `catalog_items` row keyed by `(ns, object_id)`. On UPDATE, `state` is reset to `pending` only if `content_hash` changed.
2. `XADD catalog:embed:{ns} * catalog_item_id <id> namespace <ns> object_id <oid> strategy_id <id> strategy_version <ver> enqueued_at <ts>` — only if state was set to `pending` in step 1.

**Cache implications**: This endpoint never reads the recommendation cache. It does not invalidate it either — the dense vector update will naturally surface in the next cache miss.

---

## Admin plane (cmd/admin, port 2002, session cookie auth)

### `GET /api/admin/v1/namespaces/{ns}/catalog`

Status snapshot for a namespace's catalog configuration and current operational state.

**Response 200**:

```json
{
  "enabled": true,
  "strategy_id": "internal-hashing-ngrams",
  "strategy_version": "v1",
  "embedding_dim": 128,
  "max_attempts": 5,
  "max_content_bytes": 32768,
  "available_strategies": [
    { "id": "internal-hashing-ngrams", "version": "v1", "dim": 128 },
    { "id": "internal-hashing-ngrams", "version": "v1", "dim": 256 }
  ],
  "backlog": {
    "pending":    42,
    "in_flight":  3,
    "embedded":   9821,
    "failed":     0,
    "dead_letter": 1,
    "stream_len": 45
  },
  "last_run": {
    "id":            123,
    "trigger_source":"admin",
    "started_at":    "2026-05-09T14:00:00Z",
    "completed_at":  "2026-05-09T14:03:21Z",
    "subjects_processed": 9821,
    "success":       true
  }
}
```

If the namespace has `catalog_enabled=false`, all of the operational fields are still returned (they are zero / absent) so the admin UI can render an "enable" affordance without two round-trips. `available_strategies` lists every strategy the embedder binary has registered, filtered to those whose `Dim()` equals the namespace's `embedding_dim` (so the operator only sees admissible options — Q5 acceptance #2).

### `PUT /api/admin/v1/namespaces/{ns}/catalog`

Enable / update / disable a namespace's catalog configuration.

**Request body**:

```json
{
  "enabled": true,
  "strategy_id": "internal-hashing-ngrams",
  "strategy_version": "v1",
  "params": {},
  "max_attempts": 5,
  "max_content_bytes": 32768
}
```

| Field | Notes |
|---|---|
| `enabled` | When `false`, all other fields ignored (existing values preserved); the master toggle is the only effect. |
| `strategy_id`, `strategy_version` | Required when `enabled=true`. Must resolve via the registry (`embedstrategy.Registry.Build`) and produce a `Strategy` whose `Dim()` equals the namespace's existing `embedding_dim` (Q5). |
| `params` | Strategy-specific extension slot (FR-007). V1 hashing strategy reads nothing here. |
| `max_attempts` | Defaults to env `CODOHUE_EMBED_MAX_ATTEMPTS` then 5. |
| `max_content_bytes` | Defaults to env `CODOHUE_CATALOG_MAX_CONTENT_BYTES` then 32768. |

**Responses**:

| Status | When |
|---|---|
| `200 OK` | Config persisted; no re-embed triggered. The admin client may follow up with a re-embed call below. |
| `400 Bad Request` | Unknown `(strategy_id, strategy_version)`, dimension mismatch with `embedding_dim`, or invalid `params` for the chosen strategy. The body MUST name both numbers (US2 acceptance #2). |
| `401 / 403` | Existing session-cookie auth semantics. |
| `404 Not Found` | Namespace does not exist. |

**Body on 400 dimension mismatch**:

```json
{
  "error": "strategy dimension mismatch",
  "strategy_dim": 256,
  "namespace_embedding_dim": 128
}
```

### `POST /api/admin/v1/namespaces/{ns}/catalog/re-embed`

Trigger a namespace-wide re-embed under the namespace's currently active `(strategy_id, strategy_version)`.

**Auth**: session cookie (admin).

**Request body**: empty.

**Responses**:

| Status | Body | Notes |
|---|---|---|
| `202 Accepted` | empty | Returns a `Location` header pointing to `/api/admin/v1/batch-runs/{id}` for the row created in `batch_run_logs`. |
| `404 Not Found` | `{"error":"..."}` | Namespace does not exist or `catalog_enabled=false`. |
| `409 Conflict` | `{"error":"a re-embed is already in progress for this namespace","batch_run_id":NNN}` | A `batch_run_logs` row with `phase` `embed%` and `status='running'` exists for this namespace (R6). |

**Side effects**:

1. INSERT `batch_run_logs` row with `trigger_source='admin'`, phase sentinel `embed_reembed`, `started_at=now()`.
2. `SELECT id FROM catalog_items WHERE namespace=$1 AND (strategy_version <> $2 OR strategy_version IS NULL) AND state IN ('embedded','failed','dead_letter')` to enumerate stale items.
3. For each stale item, set `state='pending'`, `attempt_count=0`, then `XADD catalog:embed:{ns}` exactly as the data-plane ingest does.

The embedder workers naturally drain the resulting backlog. A dedicated goroutine in `cmd/embedder` watches the count and updates `batch_run_logs.completed_at`, `subjects_processed`, `success` when the count reaches zero (R6).

### `GET /api/admin/v1/namespaces/{ns}/catalog/items`

Paginated browse of catalog items, intended for the admin UI to inspect pending / failed / dead-letter items (FR-014, FR-016).

**Query params**: `state` (one of `pending|in_flight|embedded|failed|dead_letter|all`, default `all`); `limit` (default 50, max 500); `offset` (default 0); `object_id` (optional substring filter).

**Response 200**: `{"items":[...], "total": N, "limit": L, "offset": O}` — items carry the projection `id, object_id, state, strategy_id, strategy_version, attempt_count, last_error, embedded_at, updated_at` (no `content`, since that may be large; see next endpoint to fetch full content).

### `GET /api/admin/v1/namespaces/{ns}/catalog/items/{id}`

Full catalog item record including `content` and `metadata`.

**Response 200**: full `catalog_items` row projection. **404** if not found.

### `POST /api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive`

Re-drive a single dead-letter (or `failed`) item.

**Side effects**: `UPDATE catalog_items SET state='pending', attempt_count=0, last_error=NULL WHERE id=$1 AND namespace=$2` then `XADD`. Returns `202 Accepted`. `404` if the item is not in `failed` or `dead_letter`.

A bulk variant `POST /api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter` re-drives every dead-letter item in the namespace in one call (SC-008).

### `DELETE /api/admin/v1/namespaces/{ns}/catalog/items/{id}`

Operator-side hard delete of a catalog item. Removes the Postgres row AND triggers the existing object-deletion path on Qdrant (FR-017). Returns `204 No Content`. Distinct from disabling the catalog feature for a namespace.

---

## Existing data-plane endpoint update

### `PUT /v1/namespaces/{ns}/objects/{id}/embedding` — BYOE write

**Behaviour change** (R8 / FR-018):

| Pre-feature | Post-feature |
|---|---|
| Always accepted (subject to auth + dim validation). | Returns **409 Conflict** when `namespace_configs.catalog_enabled=true`. Body: `{"error":"namespace uses catalog auto-embedding; BYOE writes for object dense vectors are not accepted"}`. Otherwise unchanged. |

The 409 check happens BEFORE the existing dim validation so that the operator-facing error is precise about cause. Adding/removing the catalog mode for a namespace only flips this one check; no migration of existing BYOE-written points is performed by this feature.

The BYOE write endpoint for **subjects** (`PUT /v1/namespaces/{ns}/subjects/{id}/embedding`) is NOT affected — the catalog feature only owns object dense vectors (per Assumption "Subject (user) embeddings"). Subject vectors continue to come from the cron mean-pool path.

---

## REST API table delta for `CLAUDE.md`

After implementation, the table in `CLAUDE.md` will gain the following data-plane row:

| Method   | Path                                          | Description                                                               |
| -------- | --------------------------------------------- | ------------------------------------------------------------------------- |
| `POST`   | `/v1/namespaces/{ns}/catalog`                 | Ingest a raw catalog item for auto-embedding (202 Accepted; idempotent). |

…and the following admin-plane rows:

| Method   | Path                                                                       | Auth    | Description                                                              |
| -------- | -------------------------------------------------------------------------- | ------- | ------------------------------------------------------------------------ |
| `GET`    | `/api/admin/v1/namespaces/{ns}/catalog`                                    | session | Catalog config + operational snapshot for a namespace                    |
| `PUT`    | `/api/admin/v1/namespaces/{ns}/catalog`                                    | session | Enable / update / disable a namespace's catalog configuration            |
| `POST`   | `/api/admin/v1/namespaces/{ns}/catalog/re-embed`                           | session | Trigger a namespace-wide re-embed (202 + `Location`)                     |
| `GET`    | `/api/admin/v1/namespaces/{ns}/catalog/items`                              | session | Paginated browse of catalog items (filterable by state)                  |
| `GET`    | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}`                         | session | Full catalog item including `content` and `metadata`                     |
| `POST`   | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive`                 | session | Re-drive a single failed / dead-letter item                              |
| `POST`   | `/api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter`           | session | Bulk re-drive every dead-letter item in the namespace (SC-008)           |
| `DELETE` | `/api/admin/v1/namespaces/{ns}/catalog/items/{id}`                         | session | Hard-delete a catalog item (Postgres + Qdrant point removal)             |
