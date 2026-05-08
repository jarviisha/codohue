# Phase 0 Research: RESTful API Redesign

**Feature**: 003-restful-api-redesign
**Date**: 2026-05-07

This document records the design decisions made during the audit/clarify phase of `/speckit-specify`, so the implementation phase has a single authoritative reference.

## R1. Drop legacy paths immediately (no deprecation cycle)

**Decision**: Remove every legacy duplicate path in a single change set. No `Deprecation` / `Sunset` headers, no v0/v1 dual surface.

**Rationale**: Codohue has no production traffic. DarkVoid is a demo client and is updated separately. The cost of maintaining two surfaces (testing, docs, code branches) is not amortizable when the only consumer is internal.

**Alternatives considered**:
- *Keep legacy paths with `Deprecation` header for one quarter*: Adds maintenance burden (every endpoint exists twice in route table, every handler change risks breaking the other surface). Yields no benefit because no external client depends on it.
- *Provide HTTP 308 redirects from legacy → canonical*: Clients still get a working call, but code-path duplication and tests still required. Same trade-off as above with no real upside.

## R2. Pure REST style (no AIP `:action` colons, no verb suffixes)

**Decision**: Express every operation as a resource. Non-CRUD operations either become resource creation (`POST /v1/namespaces/{ns}/rankings` creates a ranking computation; `POST /api/admin/v1/namespaces/{ns}/batch-runs` creates a batch run) or read-with-mode (`GET .../recommendations?debug=true`).

**Rationale**: User chose pure REST during clarification (Q1). Pure REST keeps the URL space free of action verbs and is the simplest mental model for new integrators. AIP custom methods (`POST /...:trigger`) are valid in production at Google scale but feel surprising in a small Go service. Verb suffixes (`/trigger`, `/debug`, `/login`) are the worst of both worlds — neither resource nor explicit RPC.

**Alternatives considered**:
- *Google AIP `:action`*: Clearer at the call site that an action is happening; widely used in GCP. Rejected by user preference.
- *Mixed pragmatic (verbs allowed for stateless computations)*: User rejected; want absolute consistency.

## R3. Sub-resource path for recommendations

**Decision**: `GET /v1/namespaces/{ns}/subjects/{id}/recommendations` and `GET /api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations?debug=true`. The subject is the parent resource; recommendations are derived from it.

**Rationale**: User chose sub-resource style during clarification (Q2). The URL self-documents the relationship — recommendations belong to a subject. Query params are reserved for filters/modifiers (limit, offset, debug), not for identifying the resource.

**Alternatives considered**:
- *Search-style with `?subject_id=`*: More familiar for clients used to search APIs, but `subject_id` is identifying, not filtering — putting it in the path is more correct.

## R4. Bare typed responses (no `{data: ...}` envelope)

**Decision**: Return resource objects (or `{items, total, ...}` collection objects) directly in the body. Do not wrap in `{data: <object>, meta: <metadata>}`.

**Rationale**: User chose bare responses during clarification (Q3). Less indirection for clients; simpler to type in Go and TypeScript. Recommendation responses already include `items`, `total`, `source`, `generated_at` — these double as both the resource and its metadata, so a separate `meta` field would be redundant. Errors keep their standard `{error: {code, message}}` shape.

**Alternatives considered**:
- *Envelope `{data, meta}`*: Easier to add request-id / tracing fields without breaking clients. Rejected because Codohue is small enough that those fields can ride on response headers.

## R5. Move `PUT /v1/namespaces/{ns}` to admin plane only

**Decision**: Namespace mutation lives exclusively at `PUT /api/admin/v1/namespaces/{ns}` (admin server, port 2002, session cookie). The data-plane API server (`cmd/api`, port 2001) registers no namespace mutation route.

**Rationale**: Confirmed by user. Separation of concerns: the data plane handles per-request traffic (events, recommendations, rankings); the control plane handles configuration. Mixing them on port 2001 forced the data plane to also accept the global admin Bearer token, expanding its blast radius. After this change, port 2001 is purely traffic and accepts only namespace keys.

**Alternatives considered**:
- *Keep `PUT /v1/namespaces/{ns}` on the data plane (with admin Bearer)*: Convenient for tooling that already has the global key. Rejected — admin work is rare and should go through the admin plane consistently.

## R6. PUT (idempotent) for embedding writes, not POST

**Decision**: BYOE embedding stores use `PUT /v1/namespaces/{ns}/objects/{id}/embedding` and `PUT /v1/namespaces/{ns}/subjects/{id}/embedding`, returning `204 No Content`.

**Rationale**: Each subject/object has at most one current embedding; setting it is a replace operation. PUT is the canonical HTTP method for idempotent set/replace of a sub-resource. Switching from POST also surfaces the semantic intent to clients.

**Alternatives considered**:
- *Keep POST*: Works but conflates create-vs-replace semantics. Rejected.
- *PATCH the parent (`PATCH /objects/{id}` with `{embedding: ...}`)*: Couples the embedding to a (nonexistent) parent object resource. Embeddings are the only mutable property exposed on objects — no benefit to introducing a parent resource just to PATCH it.

## R7. Rename `/qdrant-stats` → `/qdrant`

**Decision**: `GET /api/admin/v1/namespaces/{ns}/qdrant`.

**Rationale**: Confirmed by user. `qdrant-stats` is a verb-y noun (stats-of-qdrant). Treating Qdrant as a sub-resource of the namespace and reading it returns its current state (which happens to be statistics). Cleaner and consistent with FR-011 (no `stats` substring in paths).

**Alternatives considered**:
- *`/qdrant/stats`*: Adds nesting depth without clarifying. Rejected.

## R8. Auth via middleware on all data-plane namespace routes (remove deferred handler-level auth)

**Decision**: Apply `auth.RequireNamespace(...)` middleware to a single chi route group covering `/v1/namespaces/{ns}/*`. The current pattern in `cmd/api/main.go` (where `POST /v1/rank` runs auth inside the handler because the namespace is in the body) is removed because the redesigned `POST /v1/namespaces/{ns}/rankings` puts the namespace in the path.

**Rationale**: Single auth surface is easier to audit. Handlers should be pure business logic. The existing `auth.RequireNamespace` already accepts a `func(*http.Request) string` to extract the namespace from path params via `chi.URLParam(r, "ns")` — no new auth code is needed.

**Alternatives considered**:
- *Move auth into handler explicitly*: More flexible per-route, but inconsistent with the rest of the data plane. Rejected.

## R9. Sessions as a resource: `/api/v1/auth/sessions`

**Decision**: `POST /api/v1/auth/sessions` creates a session (login, returns 201). `DELETE /api/v1/auth/sessions/current` ends the current session (logout, returns 204). The unversioned `/api/auth/login` and `/api/auth/logout` paths are removed.

**Rationale**: Sessions are a real resource (they have a lifetime, an identity, can be inspected). Treating login/logout as session create/delete brings auth into the same RESTful style. Versioning auth aligns with FR-020 (every API route under `/api/v1/...` or `/api/admin/v1/...`).

**Alternatives considered**:
- *Keep `/api/auth/login` unversioned*: Auth shape might change later (e.g., add MFA). Versioning the path now is cheap insurance.
- *Use `/api/v1/sessions` without `/auth` segment*: Less self-documenting; `/auth/sessions` makes it obvious this is the authentication subsystem.

## R10. Demo data as a resource: `/api/admin/v1/demo-data`

**Decision**: `POST /api/admin/v1/demo-data` seeds the demo dataset (returns 202, async). `DELETE /api/admin/v1/demo-data` clears it (returns 204).

**Rationale**: The current path `/demo` is acceptable but `/demo-data` is more explicit (the resource is the data, not the demo). Using POST/DELETE on the same path expresses create/clear semantics naturally. 202 for seed acknowledges that the operation may run asynchronously (consistent with FR-010).

**Alternatives considered**:
- *Keep `/demo` as the path*: Works fine; renaming is mostly stylistic. Adopted `/demo-data` for explicitness.

## R11. Single-source namespace: path only, never body

**Decision**: Drop the `Namespace` field from every request DTO whose route already includes `{ns}` in the path. Handlers read the namespace via `chi.URLParam(r, "ns")` only.

**Rationale**: Two sources of truth invite mismatch bugs. The handler currently has to validate that body and path agree. Removing the body field eliminates a class of bugs and shrinks DTOs.

**Alternatives considered**:
- *Keep body field as redundant validator*: Defensive but useless — the path always wins. Rejected.

## R12. Status code matrix

**Decision**: Apply this fixed mapping across all endpoints:

| Situation | Code |
|-----------|------|
| Successful read | 200 |
| Successful computed-result POST (rankings) | 200 |
| Successful resource creation that returns body | 201 + `Location` header |
| Successful async/queued operation (event ingest, batch-run trigger, demo seed) | 202 + `Location` header where the result resource exists |
| Successful idempotent write with empty body (embedding store, object delete, logout, demo clear) | 204 |
| Validation error | 400 |
| Auth missing/invalid | 401 |
| Auth valid but not authorized | 403 |
| Resource (or removed legacy path) not found | 404 |
| Method not supported (e.g., POST on a now-PUT route) | 405 |
| Internal failure | 500 |

**Rationale**: The matrix above is RFC 9110-aligned and matches what client libraries (e.g., browser `fetch`, Go `http.StatusAccepted`) expect. Locking it in keeps handlers consistent.

## Open questions

None. All design decisions above have been confirmed during clarify or are deterministic from FRs.
