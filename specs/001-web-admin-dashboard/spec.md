# Feature Specification: Web Admin Dashboard

**Feature Branch**: `001-web-admin-dashboard`
**Created**: 2026-04-28
**Status**: Ready for Planning
**Input**: User description: "Giúp tôi lên kế hoạch việc thêm tính năng web admin quản lý hệ thống. trước khi triển khai hay giúp tôi ra quyết định cho các tính năng cần thiết nhé"

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 - System Health At a Glance (Priority: P1)

An operator opens the admin dashboard and immediately sees the health status of all infrastructure dependencies: PostgreSQL, Redis, and Qdrant. They can tell at a glance whether the system is fully operational or degraded, without needing to call `/healthz` manually or SSH into the server.

**Why this priority**: When something breaks in production, the operator's first question is "what is down?" A real-time health overview is the minimum viable value from any admin dashboard.

**Independent Test**: Navigate to the dashboard home page — the health status panel shows green/yellow/red indicators for each dependency and a last-checked timestamp. No other features need to exist for this test to be valid.

**Acceptance Scenarios**:

1. **Given** all dependencies are healthy, **When** the operator loads the dashboard, **Then** all three dependency indicators show "OK" and the overall status is "healthy".
2. **Given** Redis is unavailable, **When** the operator loads the dashboard, **Then** the Redis indicator shows "degraded" and the overall status reflects the degraded state.
3. **Given** the operator is not authenticated, **When** they access the dashboard, **Then** they are redirected to a login page.

---

### User Story 2 - Namespace Configuration Management (Priority: P2)

An operator needs to create a new namespace for a new product surface or update an existing namespace's tuning parameters (action weights, decay, alpha, dense strategy). They can do this through a form-based UI without constructing raw JSON API calls manually.

**Why this priority**: Namespace config is the primary control plane for the recommendation engine. Currently this requires manual `PUT /v1/config/namespaces/{ns}` calls. A UI reduces operator error and makes config auditable.

**Independent Test**: Create a new namespace via the dashboard form, confirm it appears in the namespace list, then update one of its parameters and verify the change persists.

**Acceptance Scenarios**:

1. **Given** the operator is on the namespace list, **When** they submit a "Create Namespace" form with valid parameters, **Then** the namespace appears in the list with its generated API key shown once.
2. **Given** an existing namespace, **When** the operator updates `lambda` and saves, **Then** the new value is reflected in the namespace detail view.
3. **Given** invalid input (e.g., `alpha` outside 0–1 range), **When** the operator submits, **Then** an inline validation error appears and the form is not submitted.
4. **Given** the operator views a namespace detail, **When** the namespace already has an API key, **Then** the plaintext key is NOT shown (only a masked indicator and a "Regenerate" option).

---

### User Story 3 - Recommendation Debugger (Priority: P2)

An operator suspects a specific user is getting bad recommendations. They can enter a namespace and subject ID into the dashboard and instantly see the ranked recommendation list, the source strategy used (collaborative_filtering, hybrid, fallback_popular, etc.), and each item's score and rank — all without using curl or the SDK.

**Why this priority**: Debugging recommendation quality is one of the most frequent operational tasks in a live recommendation system. Without tooling, it requires manual API calls and reading raw JSON.

**Independent Test**: Enter a subject ID and namespace in the debugger, submit, and see the recommendation list with scores and the source strategy. Can be tested without any other dashboard feature.

**Acceptance Scenarios**:

1. **Given** a subject with interaction history, **When** the operator queries recommendations, **Then** the dashboard shows items with score, rank, and source strategy.
2. **Given** a cold-start subject, **When** the operator queries recommendations, **Then** the dashboard shows fallback results and clearly labels the source as `fallback_popular`.
3. **Given** an invalid namespace or subject ID, **When** the operator submits, **Then** a clear error message is shown instead of a blank list.

---

### User Story 4 - Batch Job & Metrics Overview (Priority: P3)

An operator wants to know when the last batch recompute ran, how many subjects were processed, and whether recent batch runs succeeded. They also want to see key throughput metrics (recommendation request rate, cache hit rate, trending item count) without opening Prometheus directly.

**Why this priority**: Operational visibility into the cron pipeline is critical for knowing if recommendations are fresh. However, this is less urgent than health status and namespace management.

**Independent Test**: The metrics panel shows `codohue_batch_job_lag_seconds`, `codohue_batch_entities_processed`, and `codohue_redis_cache_requests_total` as readable numbers with labels. Can be tested independently of the recommendation debugger or namespace forms.

**Acceptance Scenarios**:

1. **Given** the cron job has run at least once, **When** the operator views the metrics panel, **Then** batch lag, subjects processed, and last run time are displayed.
2. **Given** the cron job has never run (fresh deployment), **When** the operator views the metrics panel, **Then** the panel shows "No batch run recorded yet" rather than zero or an error.

---

### User Story 5 - Trending Items Viewer (Priority: P3)

An operator can browse the current trending items for any namespace — showing each item's score, position, and the TTL remaining on the Redis cache — to verify that the trending pipeline is producing sensible results.

**Why this priority**: Trending is a critical fallback for cold-start users. Operators need to verify the trending cache is populated and sensible, especially after deploying config changes.

**Independent Test**: Select a namespace, view the trending list, confirm items are shown with scores and the cache TTL is displayed.

**Acceptance Scenarios**:

1. **Given** the trending cache is populated, **When** the operator selects a namespace and opens the trending view, **Then** items appear ranked by score with their Redis TTL shown.
2. **Given** the trending cache is empty, **When** the operator opens the trending view, **Then** a clear "No trending data — run cron to populate" message is shown.

---

### Edge Cases

- What happens when the admin dashboard itself cannot reach the Codohue API? Dashboard should display a connection error, not a blank/broken page.
- What if a namespace has tens of thousands of events? Event listings must be paginated; no unbounded fetches.
- What if the operator opens the recommendation debugger for a namespace that does not exist? Return a clear "namespace not found" error rather than an empty recommendation list.
- What if two operators simultaneously update the same namespace config? Last-write-wins is acceptable for v1; no optimistic locking required.

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The dashboard MUST require authentication before displaying any data or accepting any write operation.
- **FR-002**: The dashboard MUST display real-time health status for PostgreSQL, Redis, and Qdrant, sourced from the `/healthz` endpoint.
- **FR-003**: Operators MUST be able to list all namespaces and view each namespace's current configuration parameters.
- **FR-004**: Operators MUST be able to create a new namespace with all supported configuration parameters via a validated form.
- **FR-005**: Operators MUST be able to update an existing namespace's configuration parameters via a validated form.
- **FR-006**: The dashboard MUST display a namespace's API key exactly once — at creation time. Subsequent views MUST NOT expose the plaintext key.
- **FR-007**: Operators MUST be able to query recommendations for any subject in any namespace and see item scores, ranks, and the source strategy.
- **FR-008**: The dashboard MUST display key operational metrics: batch job lag, subjects processed per run, recommendation request rate, and Redis cache hit rate.
- **FR-009**: Operators MUST be able to view the current trending items for a selected namespace, including each item's score.
- **FR-010**: All write operations (namespace create/update) MUST be protected by the single admin credential (`CODOHUE_ADMIN_API_KEY`). There is no separate read-only tier — any authenticated operator can perform all actions.
- **FR-011**: The dashboard MUST be accessible via a standard web browser with no additional client-side installation.
- **FR-012**: All forms MUST validate inputs client-side before submission and surface server-side validation errors inline.

### Key Entities

- **Namespace**: A logical tenant of the recommendation engine with its own config, events, and vector collections. Has parameters: action_weights, lambda, gamma, alpha, dense_strategy, embedding_dim, trending_window, trending_ttl, lambda_trending, max_results, seen_items_days.
- **HealthStatus**: Per-dependency status (ok / degraded) aggregated from `/healthz`. Not persisted; read on demand.
- **BatchJobRun**: Metadata about a cron execution — timestamp, namespace, subjects processed, duration, success/failure status. Persisted to a new `batch_run_logs` table in PostgreSQL by `cmd/cron` at the end of each run. Dashboard reads this table to display run history.
- **RecommendationDebugResult**: Ephemeral result of a recommendation query — items with score/rank, source strategy, total, generated_at.
- **TrendingSnapshot**: Current trending items from Redis for a namespace — items with scores and remaining TTL.

---

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can determine the full system health status (all dependency states) within 5 seconds of opening the dashboard.
- **SC-002**: An operator can create or update a namespace configuration in under 2 minutes without consulting external documentation.
- **SC-003**: An operator can debug a specific user's recommendations (query + inspect results) in under 30 seconds.
- **SC-004**: The dashboard itself does not introduce additional load that degrades recommendation API response times by more than 5% under normal operating conditions.
- **SC-005**: All read-only views remain accessible even when one non-critical dependency is degraded.
- **SC-006**: Operators report that common operational tasks (health check, namespace update, recommendation debug) no longer require direct API calls or SSH access.

---

## Assumptions

- The admin dashboard is operated by a small number of trusted operators (1–5 people); no multi-tenancy or row-level access control is needed for v1.
- The dashboard is an internal tool; it does not need to be publicly accessible and can sit behind network-level access control (VPN, firewall) for an additional security layer.
- Authentication uses the existing `CODOHUE_ADMIN_API_KEY` environment variable — no separate user/password database is needed.
- The existing Codohue HTTP API (`cmd/api`) is the primary data source for recommendation and namespace operations; the admin service also reads `batch_run_logs` directly from PostgreSQL.
- Mobile support is out of scope for v1; the dashboard targets desktop browsers only.
- The dashboard does not need to trigger a manual batch recompute for v1 (operators can run `make run-cron` directly); this can be added in v2 if needed.

## Architecture Constraints *(decisions already made)*

These constraints are locked and must be respected during planning and implementation:

- **Deployment**: A new standalone binary `cmd/admin` running on a separate port (default: 2002). Deployed as an independent service alongside `cmd/api` and `cmd/cron` in Docker Compose.
- **Frontend**: React single-page application. The Go `cmd/admin` binary serves the compiled React build as static files and exposes a dedicated admin API for backend data. The React app calls both the admin API and the existing `cmd/api` endpoints directly.
- **New migration**: A `batch_run_logs` table must be added via a new migration. `cmd/cron` writes one row per namespace per batch run (timestamp, subjects_processed, duration_ms, success bool, error_message).
- **Repo layout**:
  ```
  cmd/admin/          ← Go HTTP server (serves static + admin API)
  internal/admin/     ← admin-specific handlers and business logic
  web/admin/          ← React source (Vite + React)
  migrations/         ← new migration for batch_run_logs
  ```
- **Build**: `make build-admin` compiles React then embeds the build output into the Go binary via `embed.FS`. A single binary ships everything.
