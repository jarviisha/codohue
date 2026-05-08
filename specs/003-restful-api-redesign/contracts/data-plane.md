# Data-Plane API Contract

**Server**: `cmd/api`, port `2001`
**Auth**: per-namespace Bearer token (bcrypt-hashed key from `namespace_configs.api_key_hash`); falls back to global `RECOMMENDER_API_KEY` if no namespace key is provisioned.
**Error envelope**: `{error: {code, message}}` (RFC 7807-lite, owned by `internal/core/httpapi`).

> All routes in this contract are the **canonical** post-redesign surface. No legacy form is registered. Any path not listed here returns 404.

---

## Operational (no auth, unversioned)

### `GET /ping`
- **200**: `{}` body, plain text "pong" or empty.
- **Purpose**: Liveness.

### `GET /healthz`
- **200**: `{postgres: "ok"|"down", redis: "ok"|"down", qdrant: "ok"|"down"}`
- **503**: when any dependency is down.

### `GET /metrics`
- Prometheus text format. No version. Unchanged.

---

## Events

### `POST /v1/namespaces/{ns}/events`

**Auth**: namespace key.
**Request body**:
```json
{
  "subject_id": "user_42",
  "object_id": "post_99",
  "action": "click",
  "occurred_at": "2026-05-07T10:30:00Z",
  "object_created_at": "2026-05-01T08:00:00Z",
  "metadata": { "platform": "web" }
}
```
- `namespace` field MUST NOT be present.
- `occurred_at`, `object_created_at`, `metadata` are optional.

**Responses**:
- **202 Accepted** — event accepted, no body.
- **400** — missing `subject_id`/`object_id`/`action`, or `action` not allowed for namespace.
- **401** — missing/invalid Bearer token.

---

## Recommendations

### `GET /v1/namespaces/{ns}/subjects/{id}/recommendations`

**Auth**: namespace key.
**Query params**:
- `limit` (1–200, default 20)
- `offset` (≥ 0, default 0)

**Response 200**:
```json
{
  "items": [
    { "id": "post_99", "score": 0.87 },
    { "id": "post_42", "score": 0.81 }
  ],
  "total": 20,
  "source": "cf_hybrid",
  "generated_at": "2026-05-07T10:30:00Z"
}
```

**Errors**:
- **400** — invalid `limit`/`offset`.
- **401** — auth failed.

---

## Rankings

### `POST /v1/namespaces/{ns}/rankings`

**Auth**: namespace key (via middleware on the route group — no in-handler auth).
**Request body**:
```json
{
  "subject_id": "user_42",
  "candidates": ["post_1", "post_2", "post_3"]
}
```
- `namespace` field MUST NOT be present.
- `candidates`: 1–500 entries.

**Response 200**:
```json
{
  "items": [
    { "id": "post_2", "score": 0.91 },
    { "id": "post_1", "score": 0.55 },
    { "id": "post_3", "score": 0.12 }
  ],
  "generated_at": "2026-05-07T10:30:00Z"
}
```

**Errors**:
- **400** — missing `subject_id`, empty `candidates`, or `len(candidates) > 500`.
- **401** — auth failed.

> Rankings are computed in-line; nothing is persisted. Hence 200 (not 201/202) and no `Location` header.

---

## Trending

### `GET /v1/namespaces/{ns}/trending`

**Auth**: namespace key.
**Query params**:
- `limit` (1–200, default 20)
- `offset` (≥ 0, default 0)
- `window_hours` (1–168, default 24)

**Response 200**:
```json
{
  "items": [
    { "id": "post_99", "score": 12.4 }
  ],
  "total": 20,
  "window_hours": 24,
  "generated_at": "2026-05-07T10:30:00Z"
}
```

**Errors**:
- **400** — invalid query params.
- **401** — auth failed.

---

## BYOE embeddings

### `PUT /v1/namespaces/{ns}/objects/{id}/embedding`
### `PUT /v1/namespaces/{ns}/subjects/{id}/embedding`

**Auth**: namespace key.
**Request body**:
```json
{ "vector": [0.12, -0.05, 0.88, ...] }
```
- Length must equal `embedding_dim` configured for the namespace.

**Response**:
- **204 No Content** — vector stored or replaced (idempotent).

**Errors**:
- **400** — wrong dimension, missing `vector`, malformed JSON.
- **401** — auth failed.
- **405** — if a client mistakenly POSTs (the same path doesn't accept POST in the redesigned surface).

---

## Object delete

### `DELETE /v1/namespaces/{ns}/objects/{id}`

**Auth**: namespace key.

**Response**:
- **204 No Content** — object removed from `{ns}_objects`, `{ns}_objects_dense`, and `id_mappings` (idempotent — succeeds even if the object never existed).

**Errors**:
- **401** — auth failed.

---

## Removed routes (return 404)

The following pre-redesign paths are removed from the data plane:

| Method | Removed path |
|--------|--------------|
| GET    | `/v1/recommendations?namespace=&subject_id=` |
| GET    | `/v1/namespaces/{ns}/recommendations?subject_id=` |
| POST   | `/v1/rank` |
| POST   | `/v1/namespaces/{ns}/rank` |
| GET    | `/v1/trending/{ns}` |
| POST   | `/v1/objects/{ns}/{id}/embedding` |
| POST   | `/v1/subjects/{ns}/{id}/embedding` |
| DELETE | `/v1/objects/{ns}/{id}` |
| POST   | `/v1/namespaces/{ns}/objects/{id}/embedding` *(now PUT)* |
| POST   | `/v1/namespaces/{ns}/subjects/{id}/embedding` *(now PUT)* |
| PUT    | `/v1/config/namespaces/{namespace}` *(moved to admin plane)* |
