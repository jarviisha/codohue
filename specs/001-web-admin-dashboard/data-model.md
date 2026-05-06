# Data Model: Web Admin Dashboard

**Date**: 2026-04-28 | **Plan**: [plan.md](./plan.md)

---

## New: batch_run_logs (PostgreSQL)

Persists the outcome of each per-namespace cron batch cycle. Written by `cmd/cron`; read by `cmd/admin`.

| Column | Type | Nullable | Notes |
|--------|------|----------|-------|
| `id` | BIGSERIAL | NO | Primary key |
| `namespace` | TEXT | NO | Namespace name |
| `started_at` | TIMESTAMPTZ | NO | When the batch cycle began for this namespace |
| `completed_at` | TIMESTAMPTZ | YES | NULL while in-progress |
| `duration_ms` | INTEGER | YES | Wall-clock duration of the full cycle in ms |
| `subjects_processed` | INTEGER | NO | Count of subjects whose vectors were recomputed (default 0) |
| `success` | BOOLEAN | NO | FALSE until the cycle completes successfully |
| `error_message` | TEXT | YES | Set on failure; NULL on success |

**Index**: `(namespace, started_at DESC)` — supports per-namespace recent-run queries efficiently.

**State transitions**:
1. `cmd/cron` begins namespace cycle → INSERT row (success=FALSE, completed_at=NULL)
2. Cycle completes successfully → UPDATE: success=TRUE, completed_at=NOW(), duration_ms, subjects_processed
3. Cycle fails → UPDATE: success=FALSE, completed_at=NOW(), error_message

**Retention**: No automatic purge in v1. Rows accumulate indefinitely. Dashboard queries are capped at `LIMIT 50` per namespace.

**Migration files**:
- `migrations/006_batch_run_logs.up.sql`
- `migrations/006_batch_run_logs.down.sql`

---

## Existing: namespace_configs (PostgreSQL) — read-only from admin

The admin reads this table directly to list and view namespace configurations. No new columns.

Key columns used by the admin:
- `namespace` — display name / identifier
- `action_weights` — JSONB, per-action weights
- `lambda`, `gamma`, `alpha` — decay and blend parameters
- `max_results`, `seen_items_days` — recommendation limits
- `dense_strategy`, `embedding_dim`, `dense_distance` — dense hybrid config
- `trending_window`, `trending_ttl`, `lambda_trending` — trending config
- `updated_at` — last config change timestamp
- `api_key_hash` — bcrypt hash; admin shows only whether a key exists (non-null), never the hash

---

## New: Admin session (in-memory / signed cookie)

Not a DB table. A session is a signed JWT cookie:

| Claim | Value |
|-------|-------|
| `sub` | `"admin"` (fixed) |
| `iat` | Unix timestamp of login |
| `exp` | `iat + 28800` (8 hours) |

**Signing**: HMAC-SHA256, key = `RECOMMENDER_API_KEY`. Session is automatically invalidated on key rotation.

---

## Go types (internal/admin/types.go)

```go
// NamespaceConfig is the admin view of a namespace configuration.
type NamespaceConfig struct {
    Namespace      string             `json:"namespace"`
    ActionWeights  map[string]float64 `json:"action_weights"`
    Lambda         float64            `json:"lambda"`
    Gamma          float64            `json:"gamma"`
    Alpha          float64            `json:"alpha"`
    MaxResults     int                `json:"max_results"`
    SeenItemsDays  int                `json:"seen_items_days"`
    DenseStrategy  string             `json:"dense_strategy"`
    EmbeddingDim   int                `json:"embedding_dim"`
    DenseDistance  string             `json:"dense_distance"`
    TrendingWindow int                `json:"trending_window"`
    TrendingTTL    int                `json:"trending_ttl"`
    LambdaTrending float64            `json:"lambda_trending"`
    HasAPIKey      bool               `json:"has_api_key"`
    UpdatedAt      time.Time          `json:"updated_at"`
}

// BatchRunLog is one cron batch cycle record for a namespace.
type BatchRunLog struct {
    ID                int64      `json:"id"`
    Namespace         string     `json:"namespace"`
    StartedAt         time.Time  `json:"started_at"`
    CompletedAt       *time.Time `json:"completed_at"`
    DurationMs        *int       `json:"duration_ms"`
    SubjectsProcessed int        `json:"subjects_processed"`
    Success           bool       `json:"success"`
    ErrorMessage      *string    `json:"error_message"`
}

// TrendingAdminEntry extends the trending item with Redis TTL info.
type TrendingAdminEntry struct {
    ObjectID   string  `json:"object_id"`
    Score      float64 `json:"score"`
    CacheTTLSec int    `json:"cache_ttl_sec"` // -1 = no expiry, -2 = key missing
}
```
