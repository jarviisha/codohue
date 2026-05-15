# Research: Admin Pipeline Controls

## 1. Batch Trigger Mechanism

**Decision**: Export `processNamespace` as a new public method `RunNamespace(ctx, ns)` on `compute.Job`. Admin server creates its own `compute.Job` instance using the same infra it already wires up.

**Rationale**: `processNamespace` is currently unexported. It already encapsulates everything needed — batch log insertion, phase 1/2/3 execution, log update. Exporting it with a minimal wrapper avoids duplicating 75 lines of orchestration logic.

**Architecture impact**: `cmd/admin/main.go` initializes a `*compute.Job` the same way `cmd/cron/main.go` does — using the shared infra (postgres pool, redis client, qdrant client, nsconfig service, compute service). The batch runs synchronously in the admin request goroutine.

**Concurrency guard**: In-process `sync.Map` keyed by namespace name. Before starting, check-and-set atomically. If namespace already running → return 409. On completion (success or error), delete the key. This is sufficient because the admin server is a single process; if admin restarts, the lock is released naturally.

**Timeout**: Wrap the `RunNamespace` call with a 10-minute context timeout. Batch runs for real namespaces typically complete in seconds to minutes; 10 minutes is a generous ceiling. Return the context deadline error as a 504 if exceeded.

**Alternatives considered**:
- **Redis pub/sub trigger to cron process**: Decoupled but requires cron to expose a subscriber and adds messaging complexity. The admin and cron are separate processes; adding IPC is overkill for an admin feature.
- **HTTP server on cron binary**: Clean separation but adds another HTTP port, TLS concerns, and another service to health-check.
- **DB-based trigger table**: Reliable but adds polling latency (minimum 1–5s) and a new table. Overkill for admin debugging use.

---

## 2. Events Listing

**Decision**: Direct PostgreSQL query from `internal/admin/repository.go`. New method `GetRecentEvents(ctx, ns, limit, offset, subjectID)`.

**SQL**:
```sql
SELECT id, namespace, subject_id, object_id, action, weight, occurred_at
FROM events
WHERE namespace = $1
  AND ($2 = '' OR subject_id = $2)
ORDER BY occurred_at DESC
LIMIT $3 OFFSET $4
```

**Count query**:
```sql
SELECT COUNT(*) FROM events WHERE namespace = $1 AND ($2 = '' OR subject_id = $2)
```

**Rationale**: The existing `events` table is already indexed on `(namespace, subject_id)` and `occurred_at` (from migration 001). The query is straightforward and the indexes make it fast even for large tables.

**Pagination**: Default limit 50, max 200. Server-side — do not stream full table.

**Alternatives considered**:
- **Cursor-based pagination**: More efficient for very large tables but adds complexity to frontend. Offset-based is simpler and sufficient for admin debugging (admins rarely need page 200+).

---

## 3. Event Injection

**Decision**: Admin service proxies `POST {apiURL}/v1/namespaces/{ns}/events` using the global `CODOHUE_ADMIN_API_KEY` as Bearer token. No new endpoint needed on `cmd/api`.

**Rationale**: The auth middleware in `cmd/api` falls back to the global key when a namespace has no per-namespace key provisioned. The admin service already uses this pattern for `UpsertNamespace` and `DebugRecommend`. No new auth surface needed.

**Request payload**: Pass through `subject_id`, `object_id`, `action`, and optionally `occurred_at`. The admin UI will pre-populate `namespace` from the page context.

**Validation**: Frontend validates that action is one of the namespace's configured `action_weights` keys (falling back to the default set: VIEW, LIKE, COMMENT, SHARE, SKIP if action_weights is empty). The main API will also validate.

**Alternatives considered**:
- **Admin calls ingest service directly**: Would require cross-domain import (admin importing ingest), violating import boundary rules. Rejected.
- **New admin-specific event table**: Unnecessary complexity; test events should go through the real pipeline.

---

## 4. Frontend: Events Page Routing

**Decision**: Add Events as a sub-page accessed via the namespace detail page or as a new top-level nav item `/events` with a namespace selector.

**Chosen approach**: New top-level route `/events` with namespace selector (same pattern as Trending and Recommend Debug pages). This avoids cluttering the namespace detail page and is consistent with existing navigation patterns.

**New nav item**: Add "Events" under "Operations" group in the sidebar, between "Batch Runs" and "Trending".

---

## 5. Compute Job Dependencies in Admin

The `compute.NewJob` constructor (seen in the file) takes injectable functions for:
- `ensureCollectionsFn` — creates Qdrant collections if not present
- `runPhase2Dense` — dense embedding computation
- `runPhase3Trending` — trending score computation

`cmd/admin/main.go` will need to wire these the same way `cmd/cron/main.go` does — using the shared compute package functions. This reuses existing implementations with zero duplication.
