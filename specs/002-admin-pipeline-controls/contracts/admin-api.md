# Admin API Contracts: Pipeline Controls

All endpoints require session cookie authentication (same as existing admin routes).

---

## POST /api/admin/v1/namespaces/{ns}/batch-runs/trigger

Runs all batch phases for the namespace synchronously. Blocks until complete.

**Auth**: session cookie  
**Path param**: `ns` — namespace name

**Request**: no body

**Response 200 — success**:
```json
{
  "batch_run_id": 42,
  "namespace": "my_feed",
  "started_at": "2026-05-03T10:00:00Z",
  "duration_ms": 1234,
  "success": true
}
```

**Response 409 — already running**:
```json
{ "error": "batch already in progress for namespace my_feed" }
```

**Response 404 — namespace not found**:
```json
{ "error": "namespace not found" }
```

**Response 504 — timeout (>10 min)**:
```json
{ "error": "batch run timed out" }
```

---

## GET /api/admin/v1/namespaces/{ns}/events

Returns a paginated list of events for a namespace, newest first.

**Auth**: session cookie  
**Path param**: `ns` — namespace name  
**Query params**:
- `limit` — integer, default 50, max 200
- `offset` — integer, default 0
- `subject_id` — string, optional filter

**Response 200**:
```json
{
  "events": [
    {
      "id": 1001,
      "namespace": "my_feed",
      "subject_id": "user-1",
      "object_id": "item-42",
      "action": "VIEW",
      "weight": 1.0,
      "occurred_at": "2026-05-03T09:55:00Z"
    }
  ],
  "total": 2847,
  "limit": 50,
  "offset": 0
}
```

**Response 400** — invalid limit/offset:
```json
{ "error": "limit must be between 1 and 200" }
```

---

## POST /api/admin/v1/namespaces/{ns}/events

Injects a test event into the pipeline for a namespace.

**Auth**: session cookie  
**Path param**: `ns` — namespace name

**Request body**:
```json
{
  "subject_id": "user-1",
  "object_id": "item-42",
  "action": "VIEW",
  "occurred_at": "2026-05-03T10:00:00Z"
}
```

`occurred_at` is optional — server uses current time if omitted.

**Response 202** — accepted:
```json
{ "ok": true }
```

**Response 400** — validation error:
```json
{ "error": "action VIEW is not configured for namespace my_feed" }
```

**Response 502** — proxy to main API failed:
```json
{ "error": "upstream event API unavailable" }
```

---

## CLAUDE.md REST API Table Additions (cmd/admin section)

| Method | Path                                              | Auth    | Description                                             |
| ------ | ------------------------------------------------- | ------- | ------------------------------------------------------- |
| `POST` | `/api/admin/v1/namespaces/{ns}/batch-runs/trigger` | session | Run batch phases for namespace immediately (synchronous) |
| `GET`  | `/api/admin/v1/namespaces/{ns}/events`            | session | Paginated recent events (`?limit=&offset=&subject_id=`) |
| `POST` | `/api/admin/v1/namespaces/{ns}/events`            | session | Inject a test event (proxied to cmd/api)                |
