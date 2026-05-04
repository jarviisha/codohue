# Data Model: Admin Pipeline Controls

## New Types (internal/admin/types.go)

```go
// TriggerBatchResponse is returned when an on-demand batch run completes.
type TriggerBatchResponse struct {
    BatchRunID  int64  `json:"batch_run_id"`
    Namespace   string `json:"namespace"`
    StartedAt   string `json:"started_at"`
    DurationMs  int    `json:"duration_ms"`
    Success     bool   `json:"success"`
}

// EventSummary is a single event row for the admin events list.
type EventSummary struct {
    ID          int64   `json:"id"`
    Namespace   string  `json:"namespace"`
    SubjectID   string  `json:"subject_id"`
    ObjectID    string  `json:"object_id"`
    Action      string  `json:"action"`
    Weight      float64 `json:"weight"`
    OccurredAt  string  `json:"occurred_at"`
}

// EventsListResponse wraps a page of events with pagination metadata.
type EventsListResponse struct {
    Events []EventSummary `json:"events"`
    Total  int            `json:"total"`
    Limit  int            `json:"limit"`
    Offset int            `json:"offset"`
}

// InjectEventRequest is the payload for the admin event injection endpoint.
type InjectEventRequest struct {
    SubjectID  string  `json:"subject_id"`
    ObjectID   string  `json:"object_id"`
    Action     string  `json:"action"`
    OccurredAt *string `json:"occurred_at,omitempty"`
}
```

## No Database Schema Changes

All queries read from the existing `events` table. No new migrations needed.

## New Repository Method (internal/admin/repository.go)

```go
// GetRecentEvents returns a paginated list of events for a namespace.
// subjectID is optional — pass empty string to return all subjects.
GetRecentEvents(ctx context.Context, ns string, limit, offset int, subjectID string) ([]EventSummary, int, error)
```

**SQL (events listing)**:
```sql
SELECT id, namespace, subject_id, object_id, action, weight, occurred_at
FROM events
WHERE namespace = $1
  AND ($2 = '' OR subject_id = $2)
ORDER BY occurred_at DESC
LIMIT $3 OFFSET $4
```

**SQL (count)**:
```sql
SELECT COUNT(*) FROM events
WHERE namespace = $1
  AND ($2 = '' OR subject_id = $2)
```

## New Service Methods (internal/admin/service.go)

```go
// TriggerBatch runs all batch phases for a namespace synchronously.
// Returns 409 if a batch is already in progress for that namespace.
TriggerBatch(ctx context.Context, ns string) (*TriggerBatchResponse, error)

// GetRecentEvents returns a paginated event list.
GetRecentEvents(ctx context.Context, ns string, limit, offset int, subjectID string) (*EventsListResponse, error)

// InjectEvent proxies a test event to cmd/api.
InjectEvent(ctx context.Context, ns string, req InjectEventRequest) error
```

## Concurrency State (internal/admin/service.go)

```go
type Service struct {
    // ... existing fields ...
    job         *compute.Job        // for on-demand batch trigger
    runningBatch sync.Map           // map[string]bool — keyed by namespace
}
```

The `sync.Map` tracks which namespaces have a batch in progress. Its lifecycle is tied to the admin process — no persistence.

## Frontend New Types (web/admin/src/)

```typescript
// hooks/useTriggerBatch.ts
interface TriggerBatchResponse {
  batch_run_id: number
  namespace: string
  started_at: string
  duration_ms: number
  success: boolean
}

// hooks/useEvents.ts
interface EventSummary {
  id: number
  namespace: string
  subject_id: string
  object_id: string
  action: string
  weight: number
  occurred_at: string
}

interface EventsListResponse {
  events: EventSummary[]
  total: number
  limit: number
  offset: number
}
```

## Frontend New Files

```text
web/admin/src/
├── hooks/
│   ├── useTriggerBatch.ts     # POST /api/admin/v1/namespaces/{ns}/batch-runs/trigger
│   ├── useEvents.ts           # GET  /api/admin/v1/namespaces/{ns}/events
│   └── useInjectEvent.ts      # POST /api/admin/v1/namespaces/{ns}/events
└── pages/
    └── EventsPage.tsx         # New page: events list + inject form
```
