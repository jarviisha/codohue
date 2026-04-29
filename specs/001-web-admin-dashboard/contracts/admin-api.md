# Admin API Contract

**Service**: `cmd/admin` | **Base URL**: `http://<host>:2002` | **Date**: 2026-04-28

All `/api/admin/v1/*` routes require a valid session cookie (`codohue_admin_session`). The `/api/auth/*` routes are public (no session required).

---

## Authentication

### POST /api/auth/login

Creates a session by validating the submitted API key against `RECOMMENDER_API_KEY`.

**Request**:
```json
{ "api_key": "dev-secret-key" }
```

**Response 200** — sets `codohue_admin_session` HTTP-only cookie (8h expiry):
```json
{ "ok": true }
```

**Response 401**:
```json
{ "error": { "code": "unauthorized", "message": "invalid api key" } }
```

---

### DELETE /api/auth/logout

Clears the session cookie.

**Response 200**:
```json
{ "ok": true }
```

---

## Health

### GET /api/admin/v1/health

Proxies `cmd/api GET /healthz`. Returns dependency health.

**Response 200** (all healthy):
```json
{
  "postgres": "ok",
  "redis": "ok",
  "qdrant": "ok",
  "status": "ok"
}
```

**Response 503** (degraded):
```json
{
  "postgres": "ok",
  "redis": "degraded",
  "qdrant": "ok",
  "status": "degraded"
}
```

---

## Namespaces

### GET /api/admin/v1/namespaces

Lists all namespace configurations from PostgreSQL.

**Response 200**:
```json
{
  "namespaces": [
    {
      "namespace": "darkvoid_feed",
      "action_weights": { "VIEW": 1, "LIKE": 5, "COMMENT": 8, "SHARE": 10, "SKIP": -2 },
      "lambda": 0.05,
      "gamma": 0.02,
      "alpha": 0.7,
      "max_results": 50,
      "seen_items_days": 30,
      "dense_strategy": "item2vec",
      "embedding_dim": 64,
      "dense_distance": "cosine",
      "trending_window": 24,
      "trending_ttl": 600,
      "lambda_trending": 0.1,
      "has_api_key": true,
      "updated_at": "2026-04-28T08:00:00Z"
    }
  ]
}
```

---

### GET /api/admin/v1/namespaces/{ns}

Returns config for a single namespace.

**Response 200**: single `NamespaceConfig` object (same shape as list item above).

**Response 404**:
```json
{ "error": { "code": "not_found", "message": "namespace not found" } }
```

---

### PUT /api/admin/v1/namespaces/{ns}

Creates or updates a namespace. Proxies to `cmd/api PUT /v1/config/namespaces/{ns}` with `Authorization: Bearer $RECOMMENDER_API_KEY`.

**Request body** — all fields optional on update, required on create:
```json
{
  "action_weights": { "VIEW": 1, "LIKE": 5 },
  "lambda": 0.05,
  "gamma": 0.02,
  "alpha": 0.7,
  "max_results": 50,
  "seen_items_days": 30,
  "dense_strategy": "item2vec",
  "embedding_dim": 64,
  "dense_distance": "cosine",
  "trending_window": 24,
  "trending_ttl": 600,
  "lambda_trending": 0.1
}
```

**Response 200** (existing namespace — no key):
```json
{
  "namespace": "darkvoid_feed",
  "updated_at": "2026-04-28T09:00:00Z"
}
```

**Response 200** (new namespace — key shown once):
```json
{
  "namespace": "new_ns",
  "updated_at": "2026-04-28T09:00:00Z",
  "api_key": "plaintext-key-shown-once"
}
```

---

## Batch Runs

### GET /api/admin/v1/batch-runs

Returns recent batch run history.

**Query params**:
| Param | Default | Description |
|-------|---------|-------------|
| `namespace` | (all) | Filter by namespace |
| `limit` | 20 | Max rows returned (max 50) |

**Response 200**:
```json
{
  "runs": [
    {
      "id": 42,
      "namespace": "darkvoid_feed",
      "started_at": "2026-04-28T08:00:00Z",
      "completed_at": "2026-04-28T08:00:12Z",
      "duration_ms": 12340,
      "subjects_processed": 1502,
      "success": true,
      "error_message": null
    },
    {
      "id": 41,
      "namespace": "darkvoid_feed",
      "started_at": "2026-04-28T07:55:00Z",
      "completed_at": "2026-04-28T07:55:08Z",
      "duration_ms": 8120,
      "subjects_processed": 1500,
      "success": false,
      "error_message": "qdrant: connection refused"
    }
  ]
}
```

---

## Trending

### GET /api/admin/v1/trending/{ns}

Returns trending items with Redis TTL info.

**Query params**: `limit` (default 50), `offset` (default 0), `window_hours` (default: namespace config)

**Response 200**:
```json
{
  "namespace": "darkvoid_feed",
  "items": [
    { "object_id": "post_99", "score": 412.5, "cache_ttl_sec": 347 },
    { "object_id": "post_7",  "score": 310.0, "cache_ttl_sec": 347 }
  ],
  "window_hours": 24,
  "limit": 50,
  "offset": 0,
  "total": 2,
  "cache_ttl_sec": 347,
  "generated_at": "2026-04-28T08:45:00Z"
}
```

`cache_ttl_sec`: seconds until the Redis trending key expires. `-1` = key has no expiry; `-2` = key does not exist (trending cache empty).

---

## Recommendation Debug

### POST /api/admin/v1/recommend/debug

Queries recommendations for a subject and returns full debug info. Proxies to `cmd/api` using the global API key.

**Request**:
```json
{
  "namespace": "darkvoid_feed",
  "subject_id": "user-123",
  "limit": 10,
  "offset": 0
}
```

**Response 200**:
```json
{
  "subject_id": "user-123",
  "namespace": "darkvoid_feed",
  "items": [
    { "object_id": "post_88", "score": 0.91, "rank": 1 },
    { "object_id": "post_42", "score": 0.74, "rank": 2 }
  ],
  "source": "collaborative_filtering",
  "limit": 10,
  "offset": 0,
  "total": 48,
  "generated_at": "2026-04-28T08:35:00Z"
}
```

**Response 404**:
```json
{ "error": { "code": "not_found", "message": "namespace not found" } }
```

---

## Error Format

All errors follow the same shape as `cmd/api`:

```json
{
  "error": {
    "code": "string",
    "message": "string"
  }
}
```

Admin-specific error codes: `unauthorized`, `not_found`, `invalid_request`, `proxy_error`, `internal_error`.
