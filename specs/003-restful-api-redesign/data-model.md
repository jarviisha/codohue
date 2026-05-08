# Phase 1 Data Model: RESTful API Redesign

**Feature**: 003-restful-api-redesign
**Date**: 2026-05-07

## Database

**No changes.** This feature is a transport-layer refactor. No tables are added, dropped, or altered. No migrations are introduced. The existing schema (events, namespace_configs, id_mappings, batch_runs) is the same before and after.

## API DTO model

The data model affected by this feature is the set of request/response shapes serialized over HTTP. The Go domain types (entities like `Event`, `BatchRun`, `Namespace`, `Subject`, etc.) are unchanged; only the DTOs at the HTTP boundary move.

### Conventions (apply to all DTOs unless overridden)

- **Request DTOs**: never carry a `namespace` field if the route includes `{ns}` in its path.
- **Response DTOs**: bare typed objects. List responses use `{items: [...], total: <int>}`. Computed responses (recommendations, rankings) use `{items, total, source, generated_at}`.
- **Error response**: unchanged — `{error: {code: <string>, message: <string>}}`. Owned by `internal/core/httpapi`.
- **JSON field names**: `snake_case` for all wire-level fields.
- **Timestamps**: RFC 3339 strings.

### Data-plane DTOs (`cmd/api`, port 2001)

#### IngestRequest (POST `/v1/namespaces/{ns}/events`)

```go
type IngestRequest struct {
    SubjectID       string                 `json:"subject_id"`
    ObjectID        string                 `json:"object_id"`
    Action          string                 `json:"action"`           // click | like | comment | share | skip | ...
    OccurredAt      time.Time              `json:"occurred_at,omitempty"`
    ObjectCreatedAt *time.Time             `json:"object_created_at,omitempty"`
    Metadata        map[string]any         `json:"metadata,omitempty"`
}
```

- `Namespace` field **removed**.
- Response: `202 Accepted`, body empty.

#### RecommendResponse (GET `/v1/namespaces/{ns}/subjects/{id}/recommendations`)

```go
type RecommendResponse struct {
    Items       []RecommendItem `json:"items"`
    Total       int             `json:"total"`
    Source      string          `json:"source"`        // "cf" | "trending" | "cf_hybrid" | "cold_start"
    GeneratedAt time.Time       `json:"generated_at"`
}

type RecommendItem struct {
    ID    string  `json:"id"`
    Score float64 `json:"score"`
}
```

- Unchanged shape from current canonical `Response` type. The handler entry point changes; the DTO does not.

#### RankRequest (POST `/v1/namespaces/{ns}/rankings`)

```go
type RankRequest struct {
    SubjectID  string   `json:"subject_id"`
    Candidates []string `json:"candidates"`             // max 500
}
```

- `Namespace` field **removed**.
- Response: `RankResponse` with `{items: [{id, score}, ...], generated_at}`. Status `200 OK`.

#### RankResponse (POST `/v1/namespaces/{ns}/rankings`)

```go
type RankResponse struct {
    Items       []RankItem `json:"items"`
    GeneratedAt time.Time  `json:"generated_at"`
}

type RankItem struct {
    ID    string  `json:"id"`
    Score float64 `json:"score"`
}
```

#### TrendingResponse (GET `/v1/namespaces/{ns}/trending`)

```go
type TrendingResponse struct {
    Items       []TrendingItem `json:"items"`
    Total       int            `json:"total"`
    GeneratedAt time.Time      `json:"generated_at"`
    WindowHours int            `json:"window_hours"`
}

type TrendingItem struct {
    ID    string  `json:"id"`
    Score float64 `json:"score"`
}
```

- Unchanged from existing implementation.

#### EmbeddingRequest (PUT `/v1/namespaces/{ns}/{objects|subjects}/{id}/embedding`)

```go
type EmbeddingRequest struct {
    Vector []float32 `json:"vector"`
}
```

- Method changes from POST to PUT.
- Response: `204 No Content`, body empty.

#### Object delete (DELETE `/v1/namespaces/{ns}/objects/{id}`)

- No request body.
- Response: `204 No Content`, body empty.

### Admin-plane DTOs (`cmd/admin`, port 2002)

#### Auth — CreateSessionRequest (POST `/api/v1/auth/sessions`)

```go
type CreateSessionRequest struct {
    APIKey string `json:"api_key"`
}

type CreateSessionResponse struct {
    ExpiresAt time.Time `json:"expires_at"`
}
```

- On success: `201 Created` + session cookie set + body with expiry.
- DELETE `/api/v1/auth/sessions/current`: no body, returns `204 No Content`.

#### Namespace upsert (PUT `/api/admin/v1/namespaces/{ns}`)

```go
type NamespaceUpsertRequest struct {
    Config nsconfig.Config `json:"config"`              // existing nsconfig DTO
}

type NamespaceUpsertResponse struct {
    Namespace string  `json:"namespace"`
    APIKey    *string `json:"api_key,omitempty"`        // populated only on first-time create
    Config    nsconfig.Config `json:"config"`
}
```

- `200 OK` on update, `201 Created` on first-time create (with `api_key` set).

#### Batch-run create (POST `/api/admin/v1/namespaces/{ns}/batch-runs`)

```go
type BatchRunCreateResponse struct {
    ID         string    `json:"id"`
    Namespace  string    `json:"namespace"`
    Status     string    `json:"status"`                // "queued" | "running"
    StartedAt  time.Time `json:"started_at,omitempty"`
}
```

- `202 Accepted`, `Location: /api/admin/v1/namespaces/{ns}/batch-runs/{id}`.

#### Batch-run list (GET `/api/admin/v1/batch-runs`, GET `/api/admin/v1/namespaces/{ns}/batch-runs`)

```go
type BatchRunListResponse struct {
    Items []BatchRun `json:"items"`
    Total int        `json:"total"`
}
```

- `BatchRun` shape is unchanged from current implementation.

#### Qdrant inspection (GET `/api/admin/v1/namespaces/{ns}/qdrant`)

```go
type QdrantInspectResponse struct {
    Subjects      QdrantCollection `json:"subjects"`
    Objects       QdrantCollection `json:"objects"`
    SubjectsDense QdrantCollection `json:"subjects_dense"`
    ObjectsDense  QdrantCollection `json:"objects_dense"`
}

type QdrantCollection struct {
    PointsCount int64 `json:"points_count"`
    Exists      bool  `json:"exists"`
}
```

- Same fields the current `qdrant-stats` endpoint returns; only the URL changes.

#### Recommendations debug (GET `/api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations?debug=true`)

```go
type AdminRecommendResponse struct {
    Items       []RecommendItem `json:"items"`
    Total       int             `json:"total"`
    Source      string          `json:"source"`
    GeneratedAt time.Time       `json:"generated_at"`
    Debug       *RecommendDebug `json:"debug,omitempty"` // populated when ?debug=true
}

type RecommendDebug struct {
    SparseNNZ        int     `json:"sparse_nnz"`
    DenseScore       float64 `json:"dense_score"`
    Alpha            float64 `json:"alpha"`
    SeenItemsCount   int     `json:"seen_items_count"`
    InteractionCount int     `json:"interaction_count"`
}
```

- Same response shape as the public `RecommendResponse` plus the optional `debug` block.

#### Subject profile (GET `/api/admin/v1/namespaces/{ns}/subjects/{id}/profile`)

```go
type SubjectProfileResponse struct {
    SubjectID         string    `json:"subject_id"`
    InteractionCount  int       `json:"interaction_count"`
    SeenItems         []string  `json:"seen_items"`
    SparseVectorNNZ   int       `json:"sparse_vector_nnz"`
    LastEventAt       time.Time `json:"last_event_at,omitempty"`
}
```

- Unchanged from current implementation.

#### Events (GET `/api/admin/v1/namespaces/{ns}/events`)

```go
type EventsListResponse struct {
    Items []Event `json:"items"`
    Total int     `json:"total"`
}
```

- `Event` shape unchanged.

#### Trending (GET `/api/admin/v1/namespaces/{ns}/trending`)

- Same shape as the public `TrendingResponse` plus optional `redis_ttl_seconds` if Redis exposes it.

#### Demo data (POST/DELETE `/api/admin/v1/demo-data`)

```go
type DemoSeedResponse struct {
    Namespace      string `json:"namespace"`
    EventsInserted int    `json:"events_inserted"`
}
```

- `202 Accepted` on POST.
- DELETE has no body, returns `204 No Content`.

## Removed DTOs / fields

- `IngestRequest.Namespace` — removed.
- `RankRequest.Namespace` — removed.
- Recommend `Get` handler's reading of `namespace` from query — removed.
- The `validateKey` parameter passed to `recommend.NewHandler` and the deferred `validateKey` call inside the `Rank` handler — removed (auth via middleware on the route group).

## Validation rules (unchanged)

- `subject_id`, `object_id`: non-empty strings.
- `action`: one of the configured action types in the namespace's `action_weights` (validated by `ingest` service).
- `candidates`: 1 ≤ len ≤ 500.
- `limit`: 1 ≤ n ≤ 200; default 20.
- `offset`: 0 ≤ n; default 0.
- `window_hours`: 1 ≤ n ≤ 168; default 24.

## State transitions

No new state machines. Existing batch-run state machine (`queued` → `running` → `succeeded` | `failed`) is unchanged; only the resource path that creates a new `BatchRun` row changes.
