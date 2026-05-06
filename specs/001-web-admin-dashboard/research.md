# Research: Web Admin Dashboard

**Date**: 2026-04-28 | **Plan**: [plan.md](./plan.md)

---

## R-001: React SPA embedded in Go binary via embed.FS

**Decision**: Build the React app with Vite into `web/admin/dist/`, then embed with `//go:embed web/admin/dist`. The Go HTTP server serves `dist/assets/*` as static files and returns `dist/index.html` for all unmatched paths (SPA fallback).

**Rationale**: Single binary deployment, no runtime file dependencies, zero-config static hosting. Vite produces a content-hashed asset bundle that is cache-friendly. `embed.FS` is standard library since Go 1.16 — no external dependency.

**Implementation pattern**:
```go
//go:embed web/admin/dist
var staticFiles embed.FS

distFS, _ := fs.Sub(staticFiles, "web/admin/dist")
r.Handle("/assets/*", http.FileServer(http.FS(distFS)))
r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
    http.ServeFileFS(w, r, distFS, "index.html")
})
```

**Alternatives considered**:
- Runtime static file serving from disk — rejected: requires operators to manage file paths on each deployment.
- Separate Nginx container serving static assets — rejected: adds infrastructure complexity; this is an internal admin tool, not a CDN-worthy public app.

---

## R-002: Admin session authentication

**Decision**: Browser submits `RECOMMENDER_API_KEY` once to `POST /api/auth/login`. The Go server validates it and issues a signed HTTP-only session cookie (HMAC-SHA256 JWT, signed with `RECOMMENDER_API_KEY` as secret, 8-hour expiry). Subsequent requests carry the cookie; middleware validates the JWT on every request.

**Rationale**: Avoids storing server-side session state (no Redis/DB session table needed). The operator enters the key once; the browser auto-sends the cookie. HTTP-only prevents XSS token theft. Using `RECOMMENDER_API_KEY` as the HMAC secret means the session is automatically invalidated if the key rotates.

**Alternatives considered**:
- HTTP Basic auth on every request — rejected: browser prompts are ugly; no logout capability.
- Storing session token in `localStorage` — rejected: XSS-vulnerable; HTTP-only cookie is safer.
- Dedicated session table in PostgreSQL — rejected: adds write load and schema complexity for a 1–5 user admin tool.

---

## R-003: Reading namespace configs in cmd/admin

**Decision**: `cmd/admin`'s repository layer reads `namespace_configs` directly from PostgreSQL using a read-only query. It does NOT call `cmd/api` for reads. For writes (create/update), it proxies to `cmd/api PUT /v1/config/namespaces/{ns}` using `RECOMMENDER_API_KEY` as the Bearer token.

**Rationale**: `cmd/api` currently has no `GET /v1/config/namespaces` or `GET /v1/config/namespaces/{ns}` endpoint. Adding these to `cmd/api` would bloat its admin surface. Reading directly from the shared PostgreSQL database is clean and already done by cron for the same data.

**Read query** (namespace list):
```sql
SELECT namespace, action_weights, lambda, gamma, alpha, max_results,
       seen_items_days, dense_strategy, embedding_dim, dense_distance,
       trending_window, trending_ttl, lambda_trending, updated_at
FROM namespace_configs
ORDER BY namespace;
```

**Alternatives considered**:
- Add GET endpoints to `cmd/api` — rejected: increases `cmd/api` surface area with admin-only routes; violates concern separation.
- Cache namespace configs in Redis — rejected: unnecessary for an admin tool with 1–5 users.

---

## R-004: batch_run_logs table design

**Decision**: New table written by `cmd/cron` at the completion of each full batch cycle (all phases for all namespaces). One row per namespace per run.

**Schema**:
```sql
CREATE TABLE batch_run_logs (
    id                  BIGSERIAL PRIMARY KEY,
    namespace           TEXT NOT NULL,
    started_at          TIMESTAMPTZ NOT NULL,
    completed_at        TIMESTAMPTZ,
    duration_ms         INTEGER,
    subjects_processed  INTEGER NOT NULL DEFAULT 0,
    success             BOOLEAN NOT NULL DEFAULT FALSE,
    error_message       TEXT
);

CREATE INDEX idx_batch_run_logs_ns_started
    ON batch_run_logs (namespace, started_at DESC);
```

`cmd/cron` inserts a row at the start of each namespace batch cycle (success=false, completed_at=null), then updates it on completion. This lets the dashboard show "in progress" state.

**Alternatives considered**:
- One row per full cron tick (all namespaces combined) — rejected: too coarse; operators need per-namespace visibility.
- Storing logs in Redis — rejected: TTL-based expiry is lossy; DB gives permanent audit history.

---

## R-005: Recommendation debugger — auth for proxied calls

**Decision**: `cmd/admin` proxies recommendation debug requests to `cmd/api` using `RECOMMENDER_API_KEY` as `Authorization: Bearer <key>`. This works because the existing two-tier auth accepts the global key as a fallback for all namespace-scoped routes.

**Rationale**: The operator never needs to know or enter individual namespace API keys. The admin already holds the global key as its own auth credential.

**Call pattern**:
```
Browser → POST /api/admin/v1/recommend/debug (session cookie)
        → cmd/admin validates session
        → cmd/admin calls cmd/api GET /v1/namespaces/{ns}/recommendations?subject_id=...
          with Authorization: Bearer $RECOMMENDER_API_KEY
        → returns result to browser
```

---

## R-006: Trending Redis TTL display

**Decision**: The admin trending view calls `TTL trending:{namespace}` via go-redis after fetching the trending list. The TTL (seconds remaining) is included in the admin API response.

**Rationale**: `TTL` is an O(1) Redis command. The trending key TTL tells operators how fresh the trending cache is and when it will expire.

**Note**: `TTL` returns -2 if the key does not exist, -1 if no expiry. The admin API maps these to `null` in the response JSON.

---

## R-007: Admin API URL prefix strategy

**Decision**: Admin-specific API routes use prefix `/api/admin/v1/`. The React SPA is served for all other paths. Static assets use `/assets/`.

**Rationale**: `/api/admin/v1/` is clearly separated from `/v1/` (the data-plane API). This makes it easy to apply separate middleware (session auth) to all admin routes without affecting the `/v1/` namespace. It also satisfies Constitution Gate III — the `/v1/` prefix is reserved for data-plane routes.

**Route table**:
```
POST   /api/auth/login              → create session
DELETE /api/auth/logout             → destroy session
GET    /api/admin/v1/health         → proxy /healthz from cmd/api
GET    /api/admin/v1/namespaces     → list namespace configs (DB read)
GET    /api/admin/v1/namespaces/{ns} → get one namespace config (DB read)
PUT    /api/admin/v1/namespaces/{ns} → create/update namespace (proxy to cmd/api)
GET    /api/admin/v1/batch-runs     → list batch runs (?namespace=&limit=20)
GET    /api/admin/v1/trending/{ns}  → proxy /v1/trending/{ns} from cmd/api + Redis TTL
POST   /api/admin/v1/recommend/debug → proxy recommend to cmd/api
```
