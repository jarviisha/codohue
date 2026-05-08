# Admin-Plane API Contract

**Server**: `cmd/admin`, port `2002`
**Auth**: session cookie `codohue_admin_session`, set by `POST /api/v1/auth/sessions` after validating the global `RECOMMENDER_API_KEY`.
**Error envelope**: `{error: {code, message}}`.

> All routes are under the canonical `/api/admin/v1/...` prefix unless otherwise noted. Auth routes are under `/api/v1/auth/...` (see [auth-plane.md](./auth-plane.md)).

---

## Health

### `GET /api/admin/v1/health`
- Proxies `GET /healthz` from `cmd/api`.
- **200**: `{postgres, redis, qdrant}` status fields.

---

## Namespaces

### `GET /api/admin/v1/namespaces`
**Query**: `include` (optional, currently supports `overview`).

**Response 200**:
```json
{
  "items": [
    {
      "namespace": "demo",
      "alpha": 0.7,
      "dense_strategy": "byoe",
      "embedding_dim": 384,
      "created_at": "2026-04-01T00:00:00Z"
    }
  ],
  "total": 1
}
```

When `?include=overview`, each item is enriched with subject/object counts and recent batch-run status.

### `GET /api/admin/v1/namespaces/{ns}`
- **200**: full namespace config (no plaintext API key).
- **404**: namespace not found.

### `PUT /api/admin/v1/namespaces/{ns}`
**Request body**: `nsconfig.Config` JSON (action weights, decay params, dense hybrid settings, etc.).

**Responses**:
- **200 OK** — existing namespace updated. Body returns the updated config.
- **201 Created** — new namespace created. Body returns config plus a one-time `api_key` field (the plaintext bcrypt-source).
- **400** — invalid config (e.g., missing required fields, invalid `dense_strategy`).

> This endpoint replaces the data-plane `PUT /v1/config/namespaces/{namespace}`. The data plane registers no namespace mutation route after the redesign.

---

## Batch runs

### `GET /api/admin/v1/batch-runs`
Cross-namespace batch-run list.
**Query**: `namespace`, `status`, `limit` (default 50), `offset`.

**Response 200**:
```json
{
  "items": [
    {
      "id": "br_abc123",
      "namespace": "demo",
      "status": "succeeded",
      "started_at": "2026-05-07T10:00:00Z",
      "finished_at": "2026-05-07T10:00:42Z",
      "phases": { "sparse": "ok", "dense": "ok", "trending": "ok" }
    }
  ],
  "total": 1
}
```

### `GET /api/admin/v1/namespaces/{ns}/batch-runs`
Same shape, scoped to one namespace.

### `POST /api/admin/v1/namespaces/{ns}/batch-runs`
Create (trigger) a batch run for the namespace.

**Request body**: empty (or `{}`). No body fields needed.

**Response 202 Accepted**:
- Header: `Location: /api/admin/v1/namespaces/{ns}/batch-runs/{id}`
- Body:
```json
{
  "id": "br_xyz789",
  "namespace": "demo",
  "status": "queued",
  "started_at": "2026-05-07T11:00:00Z"
}
```

> Replaces `POST /api/admin/v1/namespaces/{ns}/batch-runs/trigger`. Returns 202 (not 200) because the batch may still be running when the response is sent.

---

## Qdrant inspection

### `GET /api/admin/v1/namespaces/{ns}/qdrant`

**Response 200**:
```json
{
  "subjects":       { "exists": true, "points_count": 1024 },
  "objects":        { "exists": true, "points_count": 5120 },
  "subjects_dense": { "exists": false, "points_count": 0 },
  "objects_dense":  { "exists": false, "points_count": 0 }
}
```

**Errors**:
- **503** — when the Qdrant client is unavailable.

> Replaces `GET /api/admin/v1/namespaces/{ns}/qdrant-stats`.

---

## Subject inspection

### `GET /api/admin/v1/namespaces/{ns}/subjects/{id}/profile`

**Response 200**:
```json
{
  "subject_id": "user_42",
  "interaction_count": 217,
  "seen_items": ["post_1", "post_2"],
  "sparse_vector_nnz": 184,
  "last_event_at": "2026-05-07T09:00:00Z"
}
```

### `GET /api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations`
Same path the data plane exposes for client integrators, but mounted on the admin server with session auth.

**Query params**:
- `limit`, `offset` (same as data plane)
- `debug` — `true` to include the `debug` block.

**Response 200**:
```json
{
  "items": [{ "id": "post_99", "score": 0.87 }],
  "total": 20,
  "source": "cf_hybrid",
  "generated_at": "2026-05-07T10:30:00Z",
  "debug": {
    "sparse_nnz": 184,
    "dense_score": 0.42,
    "alpha": 0.7,
    "seen_items_count": 2,
    "interaction_count": 217
  }
}
```

> Replaces `POST /api/admin/v1/recommend/debug`.

---

## Trending

### `GET /api/admin/v1/namespaces/{ns}/trending`
**Query**: `limit`, `offset`, `window_hours`.

**Response 200**: same shape as the data-plane trending endpoint, plus optional `redis_ttl_seconds` for operator visibility.

---

## Events

### `GET /api/admin/v1/namespaces/{ns}/events`
**Query**: `subject_id` (optional filter), `limit` (default 50), `offset`.

**Response 200**:
```json
{
  "items": [
    {
      "subject_id": "user_42",
      "object_id": "post_99",
      "action": "click",
      "occurred_at": "2026-05-07T09:00:00Z"
    }
  ],
  "total": 1
}
```

### `POST /api/admin/v1/namespaces/{ns}/events`
Inject a test event. Proxied to `cmd/api` `POST /v1/namespaces/{ns}/events` internally.

**Request body**: same as `IngestRequest` (no `namespace` field).

**Response 202** — event accepted.

---

## Demo data

### `POST /api/admin/v1/demo-data`
Seed the demo namespace. Idempotent insert into `events` and `namespace_configs`.

**Response 202**:
```json
{
  "namespace": "demo",
  "events_inserted": 5000
}
```

### `DELETE /api/admin/v1/demo-data`
Clear the demo namespace and its derivatives.

**Response 204** — no body.

---

## Removed routes (return 404)

| Method | Removed path |
|--------|--------------|
| POST   | `/api/auth/login` *(now `POST /api/v1/auth/sessions`)* |
| DELETE | `/api/auth/logout` *(now `DELETE /api/v1/auth/sessions/current`)* |
| POST   | `/api/admin/v1/recommend/debug` *(now `GET /api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations?debug=true`)* |
| GET    | `/api/admin/v1/trending/{ns}` *(now `/api/admin/v1/namespaces/{ns}/trending`)* |
| GET    | `/api/admin/v1/subjects/{ns}/{id}/profile` *(now `/api/admin/v1/namespaces/{ns}/subjects/{id}/profile`)* |
| GET    | `/api/admin/v1/namespaces/{ns}/qdrant-stats` *(now `.../qdrant`)* |
| POST   | `/api/admin/v1/namespaces/{ns}/batch-runs/trigger` *(now `POST .../batch-runs`)* |
| POST   | `/api/admin/v1/demo` *(now `/api/admin/v1/demo-data`)* |
| DELETE | `/api/admin/v1/demo` *(now `/api/admin/v1/demo-data`)* |
