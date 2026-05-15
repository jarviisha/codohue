# Feature Specification: RESTful API Redesign

**Feature Branch**: `003-restful-api-redesign`
**Created**: 2026-05-07
**Status**: Complete
**Input**: User description: "Redesign Codohue REST API before production launch. Drop legacy duplicate paths, eliminate RPC verbs, move namespace mutation off the data-plane to the admin plane only, adopt sub-resource style for recommendations, standardize on bare typed responses with consistent status codes, and update the admin web UI accordingly."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Client integrator works against a single canonical API surface (Priority: P1)

A backend engineer integrating their product with Codohue needs to ingest behavioral events, fetch recommendations for a subject, rank candidates, and store BYOE embeddings. Today they encounter two URL styles for nearly every operation (a legacy form and a canonical form), and several endpoints require the namespace to be supplied both in the URL and in the request body. They need one obvious way to call each capability.

**Why this priority**: This is the primary value of the redesign. Every new integrator pays a tax of confusion when faced with duplicate endpoints, and dual namespace declarations introduce real bugs. Codohue is pre-production, so we have a one-time chance to ship a clean v1.

**Independent Test**: Can be fully tested by exercising every documented client-facing capability against only the canonical path style and confirming each returns the expected business result; calling any of the previously documented legacy paths returns 404.

**Acceptance Scenarios**:

1. **Given** a valid namespace API key, **When** the integrator calls the canonical recommendations path with subject id and limit, **Then** they receive a successful response containing `items`, `total`, `source`, and `generated_at` directly in the response body (no `data` wrapper).
2. **Given** a valid namespace API key, **When** the integrator calls the canonical event ingest path with a body that contains no `namespace` field, **Then** the event is accepted and stored.
3. **Given** any of the previously documented legacy paths (recommendations with `?namespace=`, `/v1/rank`, `/v1/trending/{ns}`, `/v1/objects/{ns}/{id}/embedding`, `/v1/subjects/{ns}/{id}/embedding`, `DELETE /v1/objects/{ns}/{id}`), **When** the integrator calls them, **Then** they receive 404.
4. **Given** a valid namespace API key, **When** the integrator calls the canonical ranking path with `subject_id` and `candidates` in the body, **Then** they receive a 200 response with ranked items.
5. **Given** a valid namespace API key, **When** the integrator stores an object embedding using the new idempotent write, **Then** the call succeeds with no response body and a subsequent identical call also succeeds with the same outcome.

---

### User Story 2 - Admin operator manages namespaces and pipeline jobs through resource-oriented endpoints (Priority: P2)

An operator using the admin web UI needs to create or update namespace configs, trigger batch runs on demand, inspect Qdrant collection state, debug recommendations for a subject, and seed/clear demo data. Today the admin API mixes verb-suffixed paths (`/batch-runs/trigger`, `/recommend/debug`) with resource paths and exposes auth at unversioned URLs (`/api/auth/login`).

**Why this priority**: Critical for day-to-day operations but isolated to the admin surface, so the blast radius is smaller than the client API.

**Independent Test**: Can be fully tested by exercising every admin endpoint from the admin UI: namespace list/get/upsert, batch-run list/trigger, Qdrant inspection, subject profile, debug recommendations, demo seed/clear, login/logout.

**Acceptance Scenarios**:

1. **Given** a valid admin session, **When** the operator triggers a batch run via the resource-oriented create-batch-run path, **Then** the response is 202 Accepted with a `Location` header pointing to the new batch-run resource.
2. **Given** a valid admin session, **When** the operator upserts a namespace config via the admin server, **Then** the call returns 200 (existing) or 201 (new with plaintext API key in body), and the data-plane API server exposes no equivalent mutation route.
3. **Given** a valid admin session, **When** the operator requests the Qdrant collection inspection for a namespace, **Then** they receive points-count statistics for `{ns}_subjects`, `{ns}_objects`, `{ns}_subjects_dense`, and `{ns}_objects_dense`.
4. **Given** a valid admin session, **When** the operator requests recommendations for a subject with the `debug=true` query parameter, **Then** the response includes diagnostic fields (e.g. sparse NNZ, dense score, blend weights) in addition to the items list.
5. **Given** a valid admin session, **When** the operator logs out via the canonical session-deletion path, **Then** the session cookie is cleared and subsequent requests are unauthorized.
6. **Given** a valid admin session, **When** the operator seeds or clears demo data, **Then** the seed call returns 202 (async work) and the clear call returns 204, and both target the same resource path.

---

### User Story 3 - Internal contributor adds a new endpoint following established conventions (Priority: P3)

A developer adding a new domain or capability to Codohue should find a single coherent set of conventions for path structure, response shape, status codes, auth placement, and DTO design. They should not need to choose between competing styles or grep prior art for every decision.

**Why this priority**: Quality-of-life improvement for ongoing development. Important for long-term consistency but not user-facing.

**Independent Test**: Can be verified by reviewing post-redesign route tables and handler DTOs against a documented checklist (path style, request body shape, response shape, status codes, auth wiring) — no runtime test required.

**Acceptance Scenarios**:

1. **Given** the post-redesign codebase, **When** a contributor reads the data-plane and admin route tables, **Then** every business route uses the canonical `/v1/namespaces/{ns}/...` (or admin-prefixed equivalent) form with no legacy duplicates and no RPC verb suffixes.
2. **Given** the post-redesign codebase, **When** a contributor reviews handler request DTOs, **Then** no DTO contains a field that duplicates a value already supplied by a path parameter.
3. **Given** the post-redesign codebase, **When** a contributor reviews response payloads, **Then** every endpoint returns a typed object directly (bare response, no `{data: ...}` wrapper) and errors follow the existing `{error: {code, message}}` shape.
4. **Given** the post-redesign codebase, **When** a contributor reviews route registration, **Then** authentication is enforced consistently by middleware on route groups rather than by ad-hoc checks inside individual handlers.

### Edge Cases

- A request to any removed legacy path returns 404 with the standard error envelope; no automatic redirect is provided (acceptable because Codohue has no production traffic yet).
- A request body that still contains a redundant `namespace` field is silently ignored; the path parameter is the single source of truth.
- A ranking request with more than the maximum allowed candidates returns 400 (the existing 500-item cap is preserved).
- A recommendations request for a subject with zero interactions returns the cold-start fallback (trending) under the same response schema.
- A namespace API key used on an admin-only route is rejected with 401/403; admin-only routes do not accept namespace keys.
- A session cookie used on the data-plane API is ignored; data-plane routes accept only Bearer tokens.
- A request to the admin demo-data seed endpoint while a previous seed is still running is handled the same way as today (no new concurrency requirement is introduced by this redesign).
- A request to a path that previously used `POST` for an idempotent embedding write but now requires `PUT` returns `405 Method Not Allowed` with the standard error envelope, prompting clients to update.

## Requirements *(mandatory)*

### Functional Requirements

#### Path consolidation and naming

- **FR-001**: The data-plane API MUST expose every client-facing business capability under a single canonical path of the form `/v1/namespaces/{ns}/...` (or sub-resources thereof). All previously documented "legacy" paths MUST be removed.
- **FR-002**: Recommendation reads MUST be served from a sub-resource path of the form `/v1/namespaces/{ns}/subjects/{id}/recommendations` accepting `limit` and `offset` query parameters. Both `/v1/recommendations?namespace=...` and `/v1/namespaces/{ns}/recommendations?subject_id=...` MUST be removed.
- **FR-003**: Ranking computation MUST be served from `POST /v1/namespaces/{ns}/rankings` with `subject_id` and `candidates` in the request body. The path `/v1/rank` and the `namespace`-in-body parameter MUST be removed.
- **FR-004**: Trending reads MUST be served from `GET /v1/namespaces/{ns}/trending` with `limit`, `offset`, and `window_hours` query parameters. The path `/v1/trending/{ns}` MUST be removed.
- **FR-005**: BYOE embedding writes MUST use `PUT /v1/namespaces/{ns}/objects/{id}/embedding` and `PUT /v1/namespaces/{ns}/subjects/{id}/embedding` with idempotent semantics. Both POST-based legacy variants MUST be removed.
- **FR-006**: Object deletion MUST use `DELETE /v1/namespaces/{ns}/objects/{id}`. The path `DELETE /v1/objects/{ns}/{id}` MUST be removed.

#### Request and response shape

- **FR-007**: No client-facing request body MUST require or accept a `namespace` field when the namespace is already specified in the path. The path parameter is the single source of truth for namespace.
- **FR-008**: Successful responses for endpoints that return data MUST be typed objects without a wrapping `data` envelope. Recommendation and ranking responses MUST include the fields `items`, `total`, `source`, and `generated_at`. List endpoints (e.g., namespaces, events, batch-runs) MUST include `items` and `total`.
- **FR-009**: Error responses MUST retain the existing shape `{error: {code, message}}` for all endpoints across both data-plane and admin servers.

#### HTTP semantics

- **FR-010**: HTTP status codes MUST follow these rules: `200 OK` for successful reads and computed-result POSTs (rankings); `201 Created` for resource creation that returns a representation (e.g. first-time namespace upsert returning a plaintext key); `202 Accepted` for asynchronous or queued operations (event ingest, batch-run trigger, demo-data seed); `204 No Content` for idempotent writes with empty bodies (embedding stores, object delete, logout, demo-data clear); `404 Not Found` for any request to a removed legacy path or a non-existent resource.
- **FR-011**: Non-CRUD operations MUST be expressed as resources, not as verb suffixes. Specifically, paths containing the segments `trigger`, `debug`, `login`, `logout`, `rank`, `compute`, `stats`, or any colon-style action (`:action`) MUST NOT exist in the redesigned surface.

#### Admin / data-plane separation

- **FR-012**: Namespace configuration mutation MUST be served only by the admin server (`cmd/admin`, port 2002) at `PUT /api/admin/v1/namespaces/{ns}`. The data-plane API server (`cmd/api`, port 2001) MUST NOT register any namespace mutation route. The pre-redesign route `PUT /v1/config/namespaces/{namespace}` MUST be removed.
- **FR-013**: Admin authentication MUST be served from `POST /api/v1/auth/sessions` (login) and `DELETE /api/v1/auth/sessions/current` (logout). The unversioned paths `/api/auth/login` and `/api/auth/logout` MUST be removed.
- **FR-014**: Admin batch-run trigger MUST use `POST /api/admin/v1/namespaces/{ns}/batch-runs` and return `202 Accepted` with a `Location` header pointing to the created batch-run resource. The path suffix `/trigger` MUST be removed.
- **FR-015**: Admin Qdrant inspection MUST be served from `GET /api/admin/v1/namespaces/{ns}/qdrant`. The path `/qdrant-stats` MUST be removed.
- **FR-016**: Admin debug-recommendation reads MUST be served from `GET /api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations?debug=true`, sharing the same sub-resource path used by client recommendations but enriched with diagnostic fields. The path `POST /api/admin/v1/recommend/debug` MUST be removed.
- **FR-017**: Admin demo-data operations MUST use `POST /api/admin/v1/demo-data` (seed, returns 202) and `DELETE /api/admin/v1/demo-data` (clear, returns 204).

#### Authentication and authorization

- **FR-018**: Authentication on the data-plane API MUST be enforced by middleware applied to the route group `/v1/namespaces/{ns}/*`. No data-plane handler MAY perform authentication in-handler. Specifically, the rank handler's deferred auth check MUST be removed because the namespace is now in the path.
- **FR-019**: The two-tier auth model MUST be preserved: per-namespace bcrypt-hashed keys validate Bearer tokens on data-plane routes, with the global `CODOHUE_ADMIN_API_KEY` continuing to act as a fallback when a namespace has no key provisioned. Admin server routes MUST require a valid session cookie.

#### Versioning

- **FR-020**: Every business API route MUST be prefixed with `/v1/` (data-plane) or `/api/v1/` / `/api/admin/v1/` (admin server). Operational endpoints (`/ping`, `/healthz`, `/metrics`) are exempt and remain unversioned by convention.

#### Frontend integration

- **FR-021**: The admin web UI (`web/admin`) MUST be updated to call only the new canonical paths. No code path in the web UI MAY reference any removed legacy URL.

### Key Entities

This feature does not introduce new domain entities. It re-shapes the public API surface that exposes the existing entities (Namespace, Event, Subject, Object, Embedding, BatchRun, Session, TrendingItem). Database schema is unchanged.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After the change, every documented client-facing capability is reachable from exactly one URL path; the number of duplicate (legacy + canonical) routes drops from 6 to 0.
- **SC-002**: A new integrator can complete the "ingest one event, fetch one recommendation" happy path using only the published canonical paths in under 5 minutes from reading the documentation.
- **SC-003**: Zero handler request DTOs contain a `namespace` field when the namespace is also a path parameter (verified by code inspection across all handler packages).
- **SC-004**: All previously listed legacy paths return `404` in integration tests; all canonical paths return `2xx` and produce the same business outcomes as before the redesign for equivalent inputs.
- **SC-005**: The admin web UI builds and runs end-to-end against the new paths with zero broken pages and zero console errors during a manual walkthrough of every admin screen.
- **SC-006**: The post-redesign route registration in the data-plane and admin servers contains no path containing the substrings `trigger`, `debug`, `login`, `logout`, `rank` (as a verb, not the resource `rankings`), `compute`, `stats`, or any `:action` colon syntax. This is verifiable by listing registered routes.
- **SC-007**: The data-plane server registers zero routes that mutate namespace configuration. This is verifiable by listing routes registered on the data-plane router.
- **SC-008**: The two-tier authentication model continues to work: a request with a valid per-namespace key succeeds on its namespace and fails on others; a request with the global key succeeds where namespace fallback is configured; an admin route accepts session cookies only.

## Assumptions

- DarkVoid is a demonstration client and will be updated outside the scope of this spec; no production traffic depends on the legacy paths, so they can be removed without a deprecation cycle.
- Database schema is unaffected; no migration is required.
- Domain logic in services, repositories, batch jobs, and ingest workers is unchanged. Only the HTTP transport layer (route registration, handler DTOs, middleware wiring) and the admin web UI's API client are modified.
- Existing authentication primitives (bcrypt-hashed namespace keys, the global `CODOHUE_ADMIN_API_KEY` fallback, session cookies) remain in place; only their application to routes is re-examined.
- The current `{error: {code, message}}` error envelope is acceptable and will be preserved for consistency.
- The 90-day event retention window, time-decay multipliers, cold-start fallback rules, and recommendation cache TTL are unchanged by this redesign.
- The pre-existing 500-item cap on ranking candidates is preserved.
- Operational endpoints (`/ping`, `/healthz`, `/metrics`) are intentionally exempt from versioning and remain unchanged.
- Prometheus metric names are not affected by URL changes; metrics describe internal operations, not URL paths.
- The redesign happens in a single coordinated change set so that the data-plane API, admin API, and admin web UI are released together; no intermediate state in which the UI calls outdated paths is shipped.
