# Tasks: Admin Pipeline Controls

**Input**: Design documents from `specs/002-admin-pipeline-controls/`
**Branch**: `feat/web-admin-dashboard`
**Date**: 2026-05-03

**Organization**: Tasks are grouped by user story — each story is independently implementable and testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Which user story (US1 = batch trigger, US2 = events listing, US3 = event injection)
- File paths are repo-relative

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core changes that MUST be complete before any user story work begins.

**⚠️ CRITICAL**: All three user stories block on T001–T004.

- [x] T001 Export `processNamespace` as `RunNamespace(ctx, ns)` in `internal/compute/job.go` — rename the existing private method; update the single internal caller in `runOnce` to use the new name
- [x] T002 Add four new types to `internal/admin/types.go`: `TriggerBatchResponse`, `EventSummary`, `EventsListResponse`, `InjectEventRequest` — exact field definitions from `specs/002-admin-pipeline-controls/data-model.md`
- [x] T003 Add `job *compute.Job` field and `runningBatch sync.Map` to the `Service` struct in `internal/admin/service.go`; update `NewService` to accept `job *compute.Job` as a new parameter
- [x] T004 Wire `*compute.Job` initialization into `cmd/admin/main.go` using the same infrastructure pattern as `cmd/cron/main.go` (postgres pool, redis, qdrant, nsconfig service, compute service); pass `job` to `admin.NewService`

**Checkpoint**: `go build ./...` passes — foundation ready for all user story work.

---

## Phase 3: User Story 1 — Manual Batch Trigger (Priority: P1) 🎯 MVP

**Goal**: Admin can click "Run now" on a namespace card and trigger all 3 batch phases synchronously, with 409 protection against concurrent runs.

**Independent Test**: Press "Run now" on any existing namespace → loading state appears → on completion, Batch Runs list shows a new entry with full phase breakdown.

### Backend

- [x] T005 [US1] Add `TriggerBatch(ctx context.Context, ns string) (*TriggerBatchResponse, error)` to `internal/admin/service.go`: call `GetNamespace` (→ 404), `runningBatch.LoadOrStore` (→ 409), defer delete, apply 10-min context timeout, call `job.RunNamespace`, read latest batch log row to build response
- [x] T006 [US1] Add `TriggerBatch(w http.ResponseWriter, r *http.Request)` handler to `internal/admin/handler.go`: extract `{ns}` from path, call service, map errors (404→404, "running"→409, deadline exceeded→504, else→500), write JSON response
- [x] T007 [US1] Register route in `cmd/admin/main.go` inside the protected group: `r.Post("/api/admin/v1/namespaces/{ns}/batch-runs/trigger", h.TriggerBatch)`

### Frontend

- [x] T008 [P] [US1] Create `web/admin/src/hooks/useTriggerBatch.ts`: `useMutation` POST to `/api/admin/v1/namespaces/{ns}/batch-runs/trigger`; on success invalidate `['batch-runs']` and `['namespaces-overview']` queries
- [x] T009 [P] [US1] Add "Run now" button to `NamespaceCard` in `web/admin/src/pages/NamespacesPage.tsx`: idle=secondary button "Run now", loading=disabled "Running…", success=brief "Done ✓" (2 s then revert), error=inline `<ErrorBanner>`; uses `useTriggerBatch` mutation

### Tests

- [x] T010 [P] [US1] Add handler tests for `TriggerBatch` to `internal/admin/handler_test.go`: 200 happy path, 404 namespace not found, 409 already running, 504 timeout
- [x] T011 [P] [US1] Create `internal/admin/service_test.go`: `TriggerBatch` happy path (mock job), 409 concurrent lock (simulate LoadOrStore clash), 404 namespace not found

**Checkpoint**: `POST /api/admin/v1/namespaces/{ns}/batch-runs/trigger` works end-to-end; "Run now" button visible and functional in UI.

---

## Phase 4: User Story 2 — Events Listing (Priority: P2)

**Goal**: Admin can open an Events page, pick a namespace, see the 50 most recent events newest-first, and optionally filter by subject_id.

**Independent Test**: Inject one event via curl → open Events page for the namespace → confirm the event appears at the top of the table with correct fields.

### Backend

- [x] T012 [P] [US2] Add `GetRecentEvents(ctx context.Context, ns string, limit, offset int, subjectID string) ([]EventSummary, int, error)` to `internal/admin/repository.go`: run COUNT query then SELECT with LIMIT/OFFSET as defined in `data-model.md`
- [x] T013 [US2] Add `GetRecentEvents(ctx, ns, limit, offset int, subjectID string) (*EventsListResponse, error)` to `internal/admin/service.go`: clamp limit (≤0→50, >200→200), call `repo.GetRecentEvents`, wrap in `EventsListResponse`
- [x] T014 [US2] Add `GetRecentEvents(w, r)` handler to `internal/admin/handler.go`: parse `?limit`, `?offset`, `?subject_id`; return 400 if limit not in 1–200; call service; write JSON
- [x] T015 [US2] Register route in `cmd/admin/main.go`: `r.Get("/api/admin/v1/namespaces/{ns}/events", h.GetRecentEvents)`

### Frontend

- [x] T016 [P] [US2] Create `web/admin/src/hooks/useEvents.ts`: `useQuery` GET `/api/admin/v1/namespaces/{ns}/events?limit=&offset=&subject_id=`; enabled only when `namespace !== ''`; `staleTime: 5_000`
- [x] T017 [US2] Create `web/admin/src/pages/EventsPage.tsx`: page header + namespace selector dropdown; filters row (subject_id input + Apply); events table (Time | Subject ID | Object ID | Action | Weight columns); pagination ("← Prev / Next →" with "Showing X–Y of Z total"); empty state message; all styles use DESIGN.md semantic token classes
- [x] T018 [US2] Add "Events" `<NavLink to="/events">` to the Operations section of `web/admin/src/components/Layout.tsx`, between Batch Runs and Trending
- [x] T019 [US2] Register `<Route path="events" element={<EventsPage />} />` in `web/admin/src/App.tsx` inside the Layout children

### Tests

- [x] T020 [P] [US2] Add `GetRecentEvents` tests to `internal/admin/repository_test.go`: namespace filter, subject_id filter, pagination edge case (offset > total), empty namespace
- [x] T021 [P] [US2] Add `GetRecentEvents` handler tests to `internal/admin/handler_test.go`: 200 success, 400 limit out of range, 400 invalid offset
- [x] T022 [P] [US2] Add `GetRecentEvents` service tests to `internal/admin/service_test.go`: limit clamping (0→50, 300→200)

**Checkpoint**: `GET /api/admin/v1/namespaces/{ns}/events` returns paginated data; Events page renders with table and pagination.

---

## Phase 5: User Story 3 — Inject Test Event from UI (Priority: P3)

**Goal**: Admin can fill a form on the Events page and submit a test event without using curl; event appears in the list immediately.

**Independent Test**: Fill the inject form (subject_id="test-user-1", object_id="test-item-1", action="VIEW"), submit → 202 response → event appears at top of the events table.

### Backend

- [x] T023 [US3] Add `InjectEvent(ctx context.Context, ns string, req InjectEventRequest) error` to `internal/admin/service.go`: set `occurred_at` to `time.Now().UTC()` if nil; build JSON payload; HTTP POST to `{apiURL}/v1/namespaces/{ns}/events` with Bearer auth; return nil on 202, wrapped error otherwise
- [x] T024 [US3] Add `InjectEvent(w, r)` handler to `internal/admin/handler.go`: decode JSON body, validate non-empty `subject_id` and `object_id` (→ 400), call service, write `202 {"ok":true}`
- [x] T025 [US3] Register route in `cmd/admin/main.go`: `r.Post("/api/admin/v1/namespaces/{ns}/events", h.InjectEvent)`

### Frontend

- [x] T026 [P] [US3] Create `web/admin/src/hooks/useInjectEvent.ts`: `useMutation` POST to `/api/admin/v1/namespaces/{ns}/events`; on success invalidate `['events', ns]` query
- [x] T027 [US3] Add inject form to `web/admin/src/pages/EventsPage.tsx` above the filters row: Subject ID (text), Object ID (text), Action (select from namespace action_weights keys or default VIEW/LIKE/COMMENT/SHARE/SKIP), Submit button; uses `useInjectEvent` mutation; clears fields on success

### Tests

- [x] T028 [P] [US3] Add `InjectEvent` handler tests to `internal/admin/handler_test.go`: 202 success, 400 empty subject_id, 400 empty object_id, 502 upstream failure
- [x] T029 [P] [US3] Add `InjectEvent` service tests to `internal/admin/service_test.go`: proxy 202 success, proxy non-202 error propagation

**Checkpoint**: Full inject→batch→check loop works in the UI without any curl commands.

---

## Phase 6: Polish & Cross-Cutting Concerns

- [x] T030 Update `CLAUDE.md` REST API table in the `cmd/admin` section: add rows for `POST /api/admin/v1/namespaces/{ns}/batch-runs/trigger`, `GET /api/admin/v1/namespaces/{ns}/events`, `POST /api/admin/v1/namespaces/{ns}/events` (exact text from `contracts/admin-api.md`)
- [x] T031 [P] Run `tsc --noEmit` in `web/admin/` and confirm zero type errors
- [x] T032 [P] Run `go test ./internal/admin/...` and confirm all tests pass
- [x] T033 [P] Run `go test ./internal/compute/...` and confirm RunNamespace export didn't break existing tests

---

## Dependencies & Execution Order

### Phase Dependencies

- **Foundational (Phase 2)**: No dependencies — start immediately. **BLOCKS all user stories.**
- **US1 (Phase 3)**: Depends on T001 (RunNamespace export), T002 (types), T003–T004 (service + wiring)
- **US2 (Phase 4)**: Depends on T002 (types), T003 (service struct). Can run in parallel with US1 after Foundational.
- **US3 (Phase 5)**: Depends on T002 (InjectEventRequest type), T003 (service struct). Can run in parallel with US1/US2 after Foundational.
- **Polish (Phase 6)**: Depends on all prior phases.

### Within Each User Story

- Backend service before handler (handler calls service)
- Handler before route registration
- Route registration in `main.go` (one file — serialize across stories to avoid conflicts)
- Frontend hooks ([P]) can run in parallel with backend work (different files)
- Tests ([P]) can run in parallel with each other

### Parallel Opportunities

```
After T001–T004 complete, all of these can run simultaneously:

[US1 backend]     T005 → T006 → T007
[US1 frontend]    T008, T009 (parallel with each other)
[US1 tests]       T010, T011 (parallel with each other)

[US2 backend]     T012, T013 → T014 → T015
[US2 frontend]    T016, T017 → T018 → T019
[US2 tests]       T020, T021, T022 (parallel with each other)

[US3 backend]     T023 → T024 → T025
[US3 frontend]    T026, T027 (parallel with each other)
[US3 tests]       T028, T029 (parallel with each other)
```

Note: `cmd/admin/main.go` is shared — serialize T007, T015, T025 (all touch route registration).

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 2: Foundational (T001–T004)
2. Complete Phase 3: User Story 1 (T005–T011)
3. **STOP and VALIDATE**: "Run now" button works; batch log appears in Batch Runs page
4. Demo to stakeholders — pipeline feedback loop now under 30 seconds

### Incremental Delivery

1. Foundational → US1: Batch trigger ("Run now" button) ← MVP
2. US2: Events listing ← Closes the "no events → no recs" debugging gap
3. US3: Event injection form ← Quality of life; admins can already use curl
4. Polish: CLAUDE.md + final checks

### Parallel Team Strategy

With two developers after Foundational:
- **Dev A**: US1 (batch trigger backend + frontend + tests) + route registration for T007
- **Dev B**: US2 (events listing backend + frontend + tests) — route registration T015 after Dev A lands T007
- US3 can be picked up by whoever finishes first

---

## Notes

- `cmd/admin/main.go` route registration (T007, T015, T025) must be serialized — same file
- `web/admin/src/pages/EventsPage.tsx` is shared between US2 (table) and US3 (inject form) — implement T017 before T027
- `internal/admin/service_test.go` is new (CREATE) — T011 creates it; T022 and T029 extend it
- `internal/admin/handler_test.go` is existing (MODIFY) — T010, T021, T028 each add to it
- All new `.tsx` files must use DESIGN.md semantic token classes (no hardcoded hex values)
- All new `.go` comments must be in English per CLAUDE.md conventions
