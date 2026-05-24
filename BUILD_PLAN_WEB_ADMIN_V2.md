# Codohue web/admin v2 — Build Plan

Plan triển khai **đồng bộ admin plane + UI** để biến `cmd/admin` thành mặt phẳng giám sát thật sự. Plan đặt 3 trục monitor làm tâm:

1. **Batch runs + cron phases** ([cmd/cron/](cmd/cron/), [internal/compute/](internal/compute/), bảng `batch_run_logs`)
2. **Embedder + catalog pipeline** ([cmd/embedder/](cmd/embedder/), [internal/embedder/](internal/embedder/), [internal/catalog/](internal/catalog/), bảng `catalog_items`, stream `catalog:embed:{ns}`)
3. **Live event ingest** ([cmd/api/](cmd/api/), [internal/ingest/](internal/ingest/), bảng `events`, stream `codohue:events`)

**Phạm vi:** rewrite toàn bộ [web/admin/](web/admin/) **và** redesign admin API tại chỗ trong [cmd/admin/](cmd/admin/) + [internal/admin/](internal/admin/). Hệ thống đang dev nên **chỉnh trực tiếp `/api/admin/v1/*`** — không tạo namespace mới, không giữ tương thích cũ. Bộ design system mới sẽ được cấp và áp ở Phase 0.

---

## 1. Mục tiêu & nguyên tắc

### 1.1 Mục tiêu sản phẩm

- **Operator nhìn vào màn hình là biết hệ thống đang khỏe hay không.** Mọi trang đầu vào trả lời được "có gì cần xử lý ngay không?" trong < 3 giây.
- **Drill-down một bước.** Từ trạng thái tổng quát → run / item / event cụ thể chỉ một cú click.
- **Push, không poll.** Mọi thay đổi đang xảy ra (run đang chạy, event đang ingest, backlog đang trôi) đẩy về UI qua SSE trong < 2 giây.
- **Hành động vận hành tại chỗ.** Trigger / cancel / retry batch run, redrive item, inject test event — đều ở đúng ngữ cảnh.

### 1.2 Nguyên tắc kỹ thuật

- **Admin API thiết kế cho UI giám sát**, không phải REST CRUD thuần. Cho phép aggregate endpoint, SSE, stream để giảm round-trip + đẩy real-time.
- **Sửa thẳng vào `/api/admin/v1/*`.** Endpoint cũ có thể đổi shape, đổi query param, thêm field — không cần tương thích vì chưa lên prod.
- **URL là nguồn sự thật** cho `{ns}` đang chọn và filter. Không namespace context store.
- **TanStack Query v5** quản state server. Mỗi domain một file `services/<domain>.ts`.
- **Một class string Tailwind chỉ ở primitive.** Trang chỉ compose primitives.
- **Mọi HTTP call qua `services/http.ts`.** Smoke test `tests/urls.test.mjs` chặn `fetch(` raw.
- **Embed vào binary `cmd/admin`** qua `embed.go` + build tag `embedui`.
- **Mọi struct trong [internal/admin/types.go](internal/admin/types.go)** có Go test JSON roundtrip để tránh field drift.

### 1.3 Anti-goals

Mobile UX · i18n · RBAC · theme tùy biến ngoài light/dark · density toggle · Grafana embed in-app · icon system (đợi cấp).

---

## 2. Khảo sát backend hiện có

### 2.1 Admin routes hiện tại

Bảng đầy đủ ở [cmd/admin/router.go](cmd/admin/router.go). Plan v2 **thay thế / mở rộng tại chỗ**:

- Endpoint hiện có như `GET /api/admin/v1/namespaces/{ns}/catalog`, `GET .../catalog/items`, `POST .../catalog/re-embed`, `GET /api/admin/v1/batch-runs` → **mở rộng response/query param** (additive).
- Endpoint thiếu (run detail, cancel, retry, dashboard aggregate, SSE streams, summary aggregations, metrics summary) → **thêm mới dưới `/api/admin/v1/*`**.
- Một số endpoint legacy không còn dùng sau redesign UI → **xóa khỏi router** (xem §7 mỗi phase).

### 2.2 Prometheus metrics có sẵn

12 metric ở [internal/infra/metrics/metrics.go](internal/infra/metrics/metrics.go). Plan thêm 4 metric data-plane (§3.8) + 5 metric admin-plane self-observability (§12.3), expose curated subset data-plane qua `GET /api/admin/v1/metrics/summary` (§3.6).

### 2.3 Schema hiện tại đụng vào

- [internal/admin/types.go](internal/admin/types.go) — `BatchRunLog`, `NamespaceCatalogResponse`, `CatalogBacklog`, `CatalogReEmbedSummary`, `EventSummary`, ... → **sửa shape khi cần**, không tạo struct V2 song song.
- `batch_run_logs` — migration **013** thêm cột `cancel_requested`, migration **015** thêm retention (§8).
- `catalog_items` — không đổi schema gốc, thêm bảng phụ `catalog_backlog_samples` (migration **014**).
- `events` — không cần migration.

---

## 3. Admin API redesign

Mọi endpoint nằm dưới `/api/admin/v1/*`. Nguyên tắc:

- **Aggregate endpoints** trả về payload đủ cho một view (giảm N+1).
- **SSE endpoints** trả `text/event-stream`, gửi `event: <kind>\ndata: <json>\n\n`, heartbeat `event: ping` mỗi 15s. SSE handler set `X-Accel-Buffering: no` để bypass Nginx buffer (xem §9.7).
- **Lifecycle endpoints** đầy đủ cho batch run: detail, cancel, retry.
- **Schema JSON** ổn định trong [internal/admin/types.go](internal/admin/types.go).

### 3.1 Fleet & namespace dashboards (aggregate)

#### `GET /api/admin/v1/overview` *(mới)*
Một payload đủ vẽ Fleet overview:

```json
{
  "generated_at": "2026-05-24T10:30:00Z",
  "health": { "postgres": "ok", "redis": "ok", "qdrant": "ok", "status": "ok" },
  "cron_heartbeat": { "last_run_at": "...", "lag_seconds": 42, "ok": true },
  "embedder_heartbeat": { "last_seen_at": "...", "ok": true },
  "alerts": [
    { "level": "warn", "namespace": "prod", "kind": "dead_letter_growth", "message": "..." }
  ],
  "namespaces": [
    {
      "namespace": "prod",
      "status": "active",
      "last_run": { "id": 1234, "started_at": "...", "success": true, "phase_status": ["ok","ok","ok"] },
      "events_24h": 192034,
      "events_per_min_now": 142.7,
      "catalog": { "enabled": true, "pending": 12, "dead_letter": 0 },
      "qdrant": { "subjects": 5120, "objects": 28700 }
    }
  ]
}
```

> Endpoint cũ `GET /api/admin/v1/namespaces?include=overview` bị **xóa** sau khi `/overview` lên — không còn UI nào dùng.

**Alert generation rules** (server tính, cập nhật mỗi lần `/overview` được gọi, cache 10s):

| `kind` | Threshold | `level` |
|---|---|---|
| `dead_letter_growth` | namespace có dead_letter tăng ≥ 5 item trong 5 phút | `warn` |
| `cron_lag` | `codohue_batch_job_lag_seconds` > 2× `CODOHUE_BATCH_INTERVAL_MINUTES` | `warn` |
| `embedder_silent` | `embedder_heartbeat.last_seen_at` > 2 phút | `error` |
| `health_degraded` | health response `status ≠ "ok"` | `error` |
| `consumer_lag_high` | `codohue_catalog_consumer_lag{namespace}` > 1000 | `warn` |

**UI mapping:** `level: "warn"` → Davinci `<Alert variant="warning">` hoặc `<Toast variant="warning">`; `level: "error"` → `variant="danger"`. Không bracketed token, không color-text-only — render qua component có sẵn của Davinci.

`events_per_min_now` lấy từ rolling counter `codohue_events_ingested_total` rate 1m (§3.6 mô tả implementation), **không** query bảng `events` mỗi request.

#### `GET /api/admin/v1/namespaces/{ns}/dashboard` *(mới)*
Mọi thứ cần cho trang `/ns/:ns`:
- Config snapshot
- Last 12 batch run (phase strip — `BatchRunSummary` list)
- Catalog backlog hiện tại
- Events 24h + rate hiện tại (per minute, từ rolling counter)
- Qdrant counts
- Trending TTL

Thay 5–6 round-trip riêng lẻ bằng một endpoint.

### 3.2 Batch runs — lifecycle & detail

#### `GET /api/admin/v1/batch-runs/{id}` *(mới)*
Trả `BatchRunDetail`:

```json
{
  "id": 1234,
  "namespace": "prod",
  "kind": "cf",
  "trigger_source": "cron",
  "started_at": "...",
  "completed_at": "...",
  "duration_ms": 12345,
  "success": true,
  "cancel_requested": false,
  "entities_processed": 5120,
  "error_message": null,
  "phases": [
    { "n": 1, "name": "sparse", "ok": true,  "skipped": null, "duration_ms": 3120, "subjects": 5120, "objects": 28700, "error": null },
    { "n": 2, "name": "dense",  "ok": null,  "skipped": "dense_strategy=byoe", "duration_ms": 0, "items": 0, "subjects": 0, "error": null },
    { "n": 3, "name": "trending", "ok": true, "skipped": null, "duration_ms": 980, "items": 4500, "error": null }
  ],
  "log_lines": [ { "ts": "...", "level": "info", "msg": "..." } ],
  "target_strategy": null
}
```

**Quy ước phase entry:** mảng `phases` luôn có đúng 3 entry (n=1,2,3) bất kể trạng thái. Phân biệt:
- `ok: true` — phase chạy thành công.
- `ok: false, error: "..."` — phase chạy và fail.
- `ok: null, skipped: "<reason>"` — phase bị bỏ qua (e.g. `"dense_strategy=byoe"`, `"redis_unavailable"`).

UI render qua Davinci `<Badge tone>`, **không** dùng bracketed token:
- `ok: true` → `<Badge tone="success">OK</Badge>`
- `ok: false` → `<Badge tone="danger">Failed</Badge>` + `<Tooltip>` cho `error`
- `ok: null` → `<Badge tone="neutral">Skipped</Badge>` + `<Tooltip>` cho `skipped` reason

UI kiểm `ok` trước (null = skip), không phân biệt fail vs skip qua `error` field.

#### `GET /api/admin/v1/batch-runs` (sửa shape)
List trả `BatchRunSummary` (nhẹ, không log_lines):

```json
{
  "id": 1234,
  "namespace": "prod",
  "kind": "cf",
  "trigger_source": "cron",
  "started_at": "...",
  "completed_at": "...",
  "duration_ms": 12345,
  "success": true,
  "cancel_requested": false,
  "entities_processed": 5120,
  "phase_status": ["ok", "skipped", "ok"],
  "error_message": null
}
```

`phase_status[i]` ∈ `"ok" | "fail" | "skipped" | null` (null = phase chưa chạy / chưa terminal). UI render 3 chip `<Badge tone>` cạnh nhau trong cell (tone map giống §3.2 quy ước). Filter `?kind=cf|reembed` giữ nguyên.

#### `POST /api/admin/v1/batch-runs/{id}/cancel` *(mới)*
Set `cancel_requested = true`. Cron poll cờ này giữa các phase, dừng sạch. Trả 200 với state mới; **409** nếu run đã terminal.

#### `POST /api/admin/v1/batch-runs/{id}/retry` *(mới)*
Tạo run mới cùng `namespace`, `kind`, `target_strategy` (nếu re-embed). Trả 202 + `Location`. Reject rules:
- **409** nếu run gốc còn đang chạy (chờ kết thúc rồi retry).
- **422** nếu `kind=reembed` mà catalog hiện đã disabled (target không còn nghĩa).
- **422** nếu `namespace` của run đã bị xóa.
- **404** nếu run gốc không tồn tại.

#### `GET /api/admin/v1/batch-runs/stats?window=24h&bucket=1h` *(mới)*
Time-series cho Fleet:
```json
{
  "window_seconds": 86400,
  "bucket_seconds": 3600,
  "series": [
    { "ts": "...", "ok": 12, "failed": 1, "cancelled": 0, "avg_duration_ms": 8400 }
  ]
}
```

#### `GET /api/admin/v1/batch-runs/{id}/stream` (SSE) *(mới)*
Push real-time cho trang detail:
- `event: phase_started` — `{"phase": 1, "started_at": "..."}`
- `event: phase_completed` — `{"phase": 1, "ok": true, "duration_ms": 3120, ...}`
- `event: log_line` — `{"ts": "...", "level": "info", "msg": "..."}`
- `event: run_completed` — `{"success": true, "duration_ms": 12345}`
- `event: cancelled` — khi cờ cancel_requested có hiệu lực
- `event: ping` mỗi 15s

**Khi run đã terminal:** server trả **204 No Content** cho stream request (không mở stream). UI biết fall back gọi snapshot `GET /batch-runs/{id}`. Đơn giản hơn 410 Gone vì client không cần phân biệt "không tồn tại" vs "đã xong".

Backend: in-process bus `internal/admin/eventbus`. Cron writer (qua interface mới `BatchRunObserver` ở [internal/compute/](internal/compute/)) đẩy event vào bus; SSE handler subscribe filtered theo run id. Kết nối tự đóng khi run terminal.

### 3.3 Catalog & embedder — aggregate + stream

#### `GET /api/admin/v1/namespaces/{ns}/catalog` (mở rộng response)
Giữ tên endpoint. Thêm field vào `NamespaceCatalogResponse`:
- `consumer_lag` — từ `XINFO GROUPS catalog:embed:{ns} catalog-embedder` (PEL depth, idle ms, consumers count).
- `failures_summary` — top 5 `last_error` + count trong 24h gần nhất.
- `recent_throughput` — items embedded 1m / 5m / 1h.

#### `GET /api/admin/v1/namespaces/{ns}/catalog/backlog-history?window=1h&bucket=1m` *(mới)*
Time-series từ bảng `catalog_backlog_samples` (§8 migration 014). Bảng được điền bởi sampler trong `cmd/embedder` mỗi 30s. Cửa sổ 1h / 24h / 7d.

#### `GET /api/admin/v1/namespaces/{ns}/catalog/failures-summary?window=24h` *(mới)*
Group `catalog_items` theo `last_error` (filter state ∈ `failed`/`dead_letter`, `updated_at` trong cửa sổ). Trả `[{reason, count, sample_object_id}]`.

#### `GET /api/admin/v1/namespaces/{ns}/catalog/stream` (SSE) *(mới)*
Push real-time cho trang catalog status:
- `event: backlog_snapshot` — `CatalogBacklog` mỗi 5s khi có thay đổi.
- `event: item_state_changed` — `{"id": ..., "object_id": "...", "from": "pending", "to": "embedded"}` (rate-limited 50/s).
- `event: reembed_progress` — `{"batch_run_id": ..., "processed": 1200, "total": 28000}` mỗi 2s khi re-embed.
- `event: dead_letter_grew` — alert khi count tăng so với baseline 5 phút trước.

Backend: embedder publish event vào Redis pub/sub `codohue:catalog-events:{ns}` mỗi khi process xong item; SSE handler subscribe + fan-out.

#### `GET /api/admin/v1/namespaces/{ns}/catalog/items` (mở rộng query)
Giữ tên endpoint. Thêm:
- `?include_summary=true` → kèm `state_counts: {pending: 12, in_flight: 1, embedded: 28699, ...}`.
- `?sort=updated_at|attempt_count` (desc/asc).

#### `POST /api/admin/v1/namespaces/{ns}/catalog/re-embed` (mở rộng body)
Giữ logic. Thêm body option `{"only_state": "embedded" | "failed" | "all"}` (default `all`).

**Use case từng option:**
- `all` (default) — flow gốc, re-embed mọi item. Dùng khi đổi strategy (BYOE → managed) hoặc khi nghi ngờ vector lỗi.
- `embedded` — chỉ re-embed item đã embedded ở strategy/version cũ. Dùng để **upgrade strategy version** (vd. embedded@bge-v1 → strategy@bge-v2).
- `failed` — re-embed sạch các item trạng thái `failed`, không động vào `dead_letter`. Dùng khi đã fix root cause của failure transient.

### 3.4 Events — live tail & aggregate

#### `GET /api/admin/v1/namespaces/{ns}/events/stream?action=&subject_id=` (SSE) *(mới)*
Live event tail:
- Backend XREAD BLOCK trên `codohue:events` với consumer group ephemeral `admin-tail-{requestID}` (cleanup khi disconnect).
- Filter server-side theo `action` + `subject_id`.
- `event: event` — `EventSummary` JSON.
- `event: ping` mỗi 15s.
- Backpressure: bounded channel 1024, drop oldest + push `event: dropped {"count": 12}`.

**Janitor consumer group ephemeral** chạy trong `cmd/admin` mỗi 5 phút:
- Đọc `XINFO GROUPS codohue:events` (và mỗi `catalog:embed:{ns}`).
- Xóa group có prefix `admin-tail-` và `idle > 1h`.
- Metric `codohue_admin_sse_orphan_groups_reaped_total` để track.

#### `GET /api/admin/v1/namespaces/{ns}/events/summary?window=1m|5m|1h&bucket=auto` *(mới)*
Server-side aggregation:
```json
{
  "window_seconds": 60,
  "total": 850,
  "rate_per_second": 14.16,
  "by_action": [
    { "action": "view", "count": 612, "rate": 10.2 },
    { "action": "like", "count": 180, "rate": 3.0 }
  ],
  "series": [ { "ts": "...", "count": 13 } ]
}
```

Implement: query `events WHERE namespace=? AND occurred_at > now() - interval`, group by `action` + `time_bucket`. Cache 5s.

#### `POST /api/admin/v1/namespaces/{ns}/events` (sửa response)
Logic không đổi. Trả `{"ok": true, "event_id": 12345}` để UI highlight được event vừa inject.

### 3.5 Operations stream — global event bus (SSE)

#### `GET /api/admin/v1/stream` *(mới)*
Một SSE kết nối duy nhất gắn với mỗi tab UI:
- `event: health_changed` — `{"status": "degraded", "components": ["qdrant"]}`
- `event: batch_run.started` / `.completed` — `{"id": ..., "namespace": "...", "kind": "cf", "success": true}`
- `event: catalog.dead_letter_grew` — `{"namespace": "...", "new_count": 5}`
- `event: ping` mỗi 30s

### 3.6 Metrics surface

#### `GET /api/admin/v1/metrics/summary` *(mới)*
Curated JSON từ rolling window internal:

```json
{
  "ingest": {
    "events_per_sec_1m": { "prod": 14.2, "staging": 0.4 },
    "errors_per_min_1h": { "prod": 0 }
  },
  "recommend": {
    "requests_per_sec_1m": { "prod": 320.5 },
    "qdrant_p95_ms": { "prod": 12 },
    "cache_hit_rate_1m": 0.84
  },
  "embedder": {
    "embed_p95_ms": { "prod": 480 },
    "failures_per_min_1h": { "prod": 0 }
  },
  "cron": {
    "batch_lag_seconds": 42
  }
}
```

**Source implementation.** Server duy trì rolling window song song với Prometheus counter (không scrape `/metrics` từ chính mình). Mỗi metric quan tâm có một slot trong `internal/admin/metricsroll`:
- Observation pushed vào ring buffer mỗi 10s (timer goroutine).
- Counter rate: `(latest - oldest) / window_seconds`.
- Histogram percentile (e.g. `qdrant_p95_ms`): t-digest sketch.
- Lý do: `prometheus.Gatherer` chỉ trả cumulative counter, không có rate sẵn — tính client-side trong UI thì phải đồng bộ scrape interval khắp nơi.

`events_per_min_now` ở `/overview` (§3.1) lấy từ chính slot `ingest.events_per_sec_1m` × 60.

### 3.7 Endpoint cũ bị xóa / thay thế

| Endpoint cũ | Thay bằng | Lý do |
|---|---|---|
| `GET /api/admin/v1/namespaces?include=overview` | `GET /api/admin/v1/overview` | Aggregate cluster-wide, gồm health + heartbeat. |
| `GET /api/admin/v1/batch-runs` (shape cũ trả full `BatchRunLog`) | Cùng path, shape `BatchRunSummary` | List nhẹ; detail nằm ở `/batch-runs/{id}`. |

### 3.8 Metrics data-plane mới (Prometheus)

- `codohue_events_ingested_total{namespace, action}` — Counter, [internal/ingest/](internal/ingest/).
- `codohue_ingest_errors_total{namespace, reason}` — Counter.
- `codohue_catalog_pending_items{namespace}` — Gauge, update trong embedder loop mỗi tick.
- `codohue_catalog_consumer_lag{namespace}` — Gauge từ XINFO GROUPS PEL depth.

Metrics admin-plane self-observability liệt kê ở §12.3.

---

## 4. Information architecture

### 4.1 Bản đồ tuyến đường

**Global**

| Path | Trang | Endpoint nền |
|---|---|---|
| `/login` | Login | `POST /api/v1/auth/sessions` |
| `/` | Fleet overview | `GET /api/admin/v1/overview` + `/api/admin/v1/stream` |
| `/health` | Service health detail | `GET /api/admin/v1/health` + `/metrics/summary` |
| `/namespaces` | Namespace list | `GET /api/admin/v1/namespaces` |
| `/namespaces/new` | Create namespace | `PUT /api/admin/v1/namespaces/{ns}` |
| `/batch-runs` | Lịch sử batch run global | `GET /api/admin/v1/batch-runs` + `/batch-runs/stats` |
| `/batch-runs/:id` | Chi tiết một run | `GET /api/admin/v1/batch-runs/{id}` + `/batch-runs/{id}/stream` |
| `/danger-zone` | App reset | `POST /api/admin/v1/reset` |

**Namespace-scoped** (dưới `<NamespaceLayout>` đọc `:ns` từ `useParams`)

| Path | Trang | Endpoint nền |
|---|---|---|
| `/ns/:ns` | Overview | `GET /api/admin/v1/namespaces/{ns}/dashboard` + `/stream` |
| `/ns/:ns/config` | Config | `GET·PUT /api/admin/v1/namespaces/{ns}` |
| `/ns/:ns/catalog` | Catalog status | `GET .../catalog` + `.../catalog/stream` + `.../backlog-history` |
| `/ns/:ns/catalog/items` | Items browser | `GET .../catalog/items?include_summary=true` |
| `/ns/:ns/catalog/items/:id` | Item detail | `GET .../catalog/items/{id}` |
| `/ns/:ns/batch-runs` | Lịch sử ns | `GET .../namespaces/{ns}/batch-runs` |
| `/ns/:ns/batch-runs/:id` | Chi tiết | Reuse global detail page |
| `/ns/:ns/events` | Live events | `GET .../events/stream` + `.../events/summary` |
| `/ns/:ns/events/inject` | Inject test | `POST .../events` |
| `/ns/:ns/trending` | Trending | `GET .../trending` |
| `/ns/:ns/debug` | Recommend debug | `GET .../subjects/{id}/recommendations?debug=true` |
| `/ns/:ns/demo-data` | Demo data | `POST·DELETE /api/admin/v1/demo-data` |

### 4.2 App shell (Davinci `<AppShell>`)

Dùng Davinci `<AppShell>` slot layout (`AppShellTopBar` / `AppShellSidebar` / `AppShellHeader` / `AppShellMain` / `AppShellAside` khi cần).

- **Top bar** (`<AppShellTopBar>`):
  - Davinci `<Breadcrumbs>` derive từ route segments (vd: `Codohue / prod / Catalog / Items`). Click segment để nhảy về.
  - Namespace switcher: Davinci `<Combobox>` ở giữa top bar; chọn ns → push route `/ns/{name}/...`.
  - Theme toggle (Davinci `<DropdownMenu>` light/dark/system, gọi `useTheme().setTheme`).
  - User menu (Davinci `<Avatar>` + `<DropdownMenu>` → Logout).
  - Badge số run đang chạy + health indicator (Davinci `<Badge tone>` + `<Tooltip>`) cập nhật qua `/api/admin/v1/stream`.
- **Sidebar** (`<AppShellSidebar>`): Davinci `<Nav>` chia hai section **Global** + **{ns}**. Khối {ns} ẩn khi chưa có namespace active.
- **Command palette `Cmd+K`**: custom widget compose từ Davinci `<Dialog>` + `<Combobox>` (Davinci không ship Command primitive). Recent-items push từ `/stream`.

---

## 5. Ba trục monitor — chi tiết view + endpoint

### 5.1 Trục Batch runs + cron phases

#### A. `/batch-runs` — Lịch sử toàn cluster
- Stats row (4 tiles: total / running / ok / failed).
- Time-series 24h từ `/batch-runs/stats`.
- Bảng dày, sắp `started_at DESC`. Subscribe `/stream` event `batch_run.started`/`.completed` cập nhật row tức thì.
- Toolbar filter: namespace · status · kind · time range preset.

#### B. `/batch-runs/:id` — Chi tiết một run
- Header status token + duration + trigger + namespace + actions (Cancel · Retry).
- **Connect SSE `/batch-runs/{id}/stream`** khi run chưa terminal. Nếu server trả 204 → run đã terminal → fall back snapshot.
- Phase strip = 3 `<Badge tone>` cạnh nhau (tone map theo §3.2). Pulse opacity (token Davinci motion) khi `phase_started`, snap về color cuối khi `phase_completed`. `<Tooltip>` cho skipped/error reason.
- Log viewer auto-scroll, append `log_line` real-time, filter level + search.
- Disconnect khi `run_completed` hoặc `cancelled`.
- Nếu là re-embed: hiện `target_strategy_id@version`, link sang `/ns/:ns/catalog`.

#### C. `/ns/:ns/batch-runs` — Lịch sử theo namespace
- Như A nhưng pre-filter, sparkline duration 24h, đánh dấu run > p95.

#### D. Phase-strip widget trên `/ns/:ns`
- 12 ô run gần nhất từ `dashboard.last_runs[].phase_status`. Click → mở detail.

### 5.2 Trục Embedder + catalog pipeline

#### A. `/ns/:ns/catalog` — Catalog status
- 6 metric tile backlog + `consumer_lag` tile.
- Throughput tile (items/s 1m + 5m).
- **Backlog timeline 1h** từ `/catalog/backlog-history`. Survive reload.
- Failures-summary block từ `/catalog/failures-summary` — top 5 reason + sample item.
- Last re-embed card + nút **Trigger re-embed** (modal với option `only_state`).
- Strategy picker.
- **Connect SSE `/catalog/stream`**: cập nhật backlog tiles + push toast khi `dead_letter_grew`.

#### B. `/ns/:ns/catalog/items` — Items browser
- Header tiles từ `?include_summary=true` (state_counts ngay trong response).
- Toolbar filter state + search object_id + sort.
- Row action: Redrive / Open / Delete.
- Footer bulk redrive khi state=dead_letter.
- Subscribe SSE `item_state_changed` cập nhật state inline.

#### C. `/ns/:ns/catalog/items/:id` — Item detail
- Content + metadata + vector preview + attempts history.
- Action: Redrive / Delete.

#### D. Re-embed progress overlay
- Khi re-embed đang chạy (từ `/stream` event `batch_run.started` với `kind=reembed`), thanh progress sticky bottom hiện `processed / total` cập nhật qua SSE `reembed_progress`. Click → `/batch-runs/:id`.

### 5.3 Trục Live event ingest

#### A. `/ns/:ns/events` — Live tail
- **Connect SSE `/events/stream?action=&subject_id=`** từ filter hiện tại.
- Bảng tail (ring buffer 1000 row, mới highlight 1s).
- Toolbar: filter action (server-side), filter subject_id, nút Pause/Resume, nút Inject (modal).
- Sidebar phải (refetch mỗi 5s):
  - 4 tile: events 1m / 5m / 1h / current rate.
  - Action mix donut.
  - Mini bar chart 60-bucket cuối.
- Alert nếu SSE `event: dropped` (client chậm hơn ingest).

#### B. `/ns/:ns/events/inject`
- Form `subject_id`, `object_id`, `action`, `occurred_at`.
- Submit → POST → redirect `/ns/:ns/events?highlight=<event_id>` highlight 3s.

---

## 6. Frontend architecture

### 6.1 Stack

- React 19 + TypeScript + Vite.
- **Davinci design system** (4 npm package internal — xem [web/admin/USAGE.md](web/admin/USAGE.md)):
  - `@jarviisha/davinci-tokens` — CSS variables + JS tokens.
  - `@jarviisha/davinci-tailwind-preset` — Tailwind v4 preset map token sang utility (`bg-background`, `text-foreground`, ...).
  - `@jarviisha/davinci-react-theme-provider` — `<ThemeProvider>` + `<ThemeScript>` (chống flash).
  - `@jarviisha/davinci-react-ui` — Button / Card / Table / Dialog / Drawer / Popover / Tooltip / Toast / Badge / Alert / Tabs / Pagination / Combobox / Input / Textarea / Select / Checkbox / Radio / Switch / EmptyState / Skeleton / Avatar / FormField / Nav / Breadcrumbs / DropdownMenu / Stack / Inline / Container / Divider / AppShell / DetailLayout / useToast / useFocusTrap.
- Tailwind v4 qua `davinci-tailwind-preset` — **không** tự định nghĩa `@theme`.
- React Router v7 **data router** (`createBrowserRouter`) — bắt buộc từ Phase 0 cho `useBlocker` (dirty-form guard).
- TanStack Query v5.
- Chart: **Recharts** (~150KB gzipped). Davinci không ship chart. Tradeoff vs visx ở §13 D6.
- SSE: `EventSource` API + custom hook factory `useServerStream(url, handlers)`.

**Visual philosophy.** Davinci là single-canvas, border-led, fill-for-intent (Jira/Linear/GitHub-style — xem [web/admin/DESIGN.md](web/admin/DESIGN.md) cho rationale). Status qua `<Badge tone>` / `<Alert variant>`, location qua `<Breadcrumbs>`. **Không** bracketed `[ OK ]` token, **không** PS1 prompt, **không** terminal/console aesthetic — đó là quyết định D9 (§13).

### 6.2 Tổ chức thư mục

Davinci cung cấp mọi UI primitive — **không** tự build `Button`/`Card`/`Table`/`Dialog`/`Toast`/`Badge`/`Alert`/v.v. Custom component chỉ ở `components/shell/` (compose Davinci AppShell + Nav + Breadcrumbs + Combobox), `components/charts/` (Recharts wrapper), và `components/monitoring/` (domain widget).

```
web/admin/src/
├── main.tsx               # <ThemeProvider> + <ToastProvider> + RouterProvider
├── index.css              # 3 Davinci CSS imports + Tailwind
├── routes/                # createBrowserRouter, nav declarative
├── services/
│   ├── http.ts            # base client, error normalize
│   ├── stream.ts          # useServerStream + reconnect/backoff + 401 redirect
│   ├── auth.ts
│   ├── overview.ts        # /overview + /stream subscriptions
│   ├── namespaces.ts
│   ├── batchRuns.ts       # list + detail + stream + stats + cancel/retry
│   ├── catalog.ts         # config + items + stream + history + failures-summary
│   ├── events.ts          # tail stream + summary + inject
│   ├── metrics.ts         # /metrics/summary
│   ├── trending.ts · recommend.ts · qdrant.ts · health.ts · danger.ts
│   └── queryKeys.ts
├── components/
│   ├── shell/             # AppShellHeader (compose Breadcrumbs + Combobox NS switcher + theme/user menu), CommandPalette (Dialog + Combobox), SidebarNav (Nav wrapper)
│   ├── charts/            # Sparkline, TimeSeriesChart, ActionMixDonut, BacklogChart (Recharts)
│   └── monitoring/        # PhaseStrip (3× Badge tone), LogLineViewer, EventTailRow, ConsumerLagTile, ReembedProgressBar
├── pages/
│   ├── fleet/             # FleetOverviewPage
│   ├── health/ · danger-zone/ · login/ · namespaces/
│   ├── batch-runs/        # ListPage, DetailPage (shared global + ns)
│   └── ns/
│       ├── NamespaceLayout.tsx · OverviewPage.tsx · ConfigPage.tsx
│       ├── catalog/       # StatusPage, ItemsPage, ItemDetailPage
│       ├── events/        # TailPage, InjectPage
│       ├── trending/ · debug/ · demo-data/
└── utils/
```

### 6.3 Stream pattern (chuẩn cho mọi SSE)

```ts
useServerStream(url, {
  onEvent: { phase_started: (data) => ..., log_line: (data) => ... },
  onPing: () => setLastPing(Date.now()),
  onError: (e) => ...,
  enabled: !runTerminal,
  reconnect: { backoffMs: [1000, 2000, 5000, 10000], maxAttempts: Infinity }
})
```

- Một stream connection mỗi tab UI cho `/api/admin/v1/stream` (global), spawn ad-hoc cho stream theo entity.
- Khi `document.visibilityState === 'hidden'` > 60s: disconnect, reconnect khi visible.
- Heartbeat watchdog: không nhận `ping` trong 45s → force reconnect.
- **401 từ SSE → redirect `/login?next=<current_path>`** (session expire mid-stream). Hook dispatch event global, không retry.

**Cross-origin dev mode.** Vite dev server (`localhost:5173`) → admin (`localhost:2002`):
- Frontend: `new EventSource(url, { withCredentials: true })`.
- Backend: `cmd/admin` middleware set CORS `Access-Control-Allow-Origin: http://localhost:5173` + `Access-Control-Allow-Credentials: true` khi env `CODOHUE_ALLOW_DEV_ORIGIN=http://localhost:5173`. Production (embed) same-origin, không cần.

### 6.4 Server unreachable

- `http.ts` normalize lỗi → `{ ok, error?: {code, message} }`.
- Global `Notice` khi `/stream` mất connection > 10s.
- 401 từ REST → redirect `/login?next=`. 401 từ SSE — như trên.

---

## 7. Phase plan (backend + frontend bundled)

Mỗi phase ship một slice end-to-end: API mới + UI mới + migration nếu cần. Test gate: lint + `tests/urls.test.mjs` + Go test xanh + performance budget (§12.2).

### Phase 0 — Foundation (1 tuần)

**Backend**
- Tạo `internal/admin/eventbus` package: in-process pub/sub với topic + filtered subscription.
- Tạo `internal/admin/sse` package: SSE helper (writer, heartbeat, reconnect ID, disconnect detect, `X-Accel-Buffering: no` header).
- Tạo `internal/admin/metricsroll` package: rolling window slot cho counter/histogram.
- Sửa [internal/admin/types.go](internal/admin/types.go) cho schema mới (BatchRunSummary / BatchRunDetail / PhaseEntry / OverviewResponse / NamespaceDashboardResponse / Alert / ...).
- Sửa [cmd/admin/router.go](cmd/admin/router.go) cho path mới (skeleton + test ping endpoint `GET /api/admin/v1/ping/stream`).
- CORS middleware khi `CODOHUE_ALLOW_DEV_ORIGIN` set.

**Frontend**
- `npm install @jarviisha/davinci-tokens @jarviisha/davinci-tailwind-preset @jarviisha/davinci-react-theme-provider @jarviisha/davinci-react-ui`.
- Wire `src/index.css` ba imports CSS Davinci (variables / light / dark) + Tailwind.
- Configure `tailwind.config.ts` với `presets: [davinci-preset]`.
- `index.html`: inline `getThemeScript()` trong `<head>` chống flash theme.
- `main.tsx`: `<ThemeProvider>` + `<ToastProvider>` + `<RouterProvider>`.
- Migrate sang `createBrowserRouter` (data router).
- Dựng AppShell shell qua Davinci: `<AppShell>` + `<AppShellTopBar>` (Breadcrumbs + namespace `<Combobox>` + theme/user `<DropdownMenu>`) + `<AppShellSidebar>` (`<Nav>` 2 section, ns block ẩn khi chưa có ns) + `<AppShellMain>` (`<Outlet/>`).
- `services/http.ts` + `services/stream.ts` + queryKeys.
- Login page (`<FormField>` + `<Input>` + `<Button>`).
- Đo bundle size baseline (Davinci impact) → cập nhật §12.2 nếu cần.

**Exit:** `make dev-admin` login OK, AppShell rỗng nhưng có chrome đầy đủ (breadcrumbs, theme toggle hoạt động không flash). Endpoint giả `/ping/stream` emit timestamp mỗi giây — chứng minh SSE pipeline end-to-end hoạt động (kể cả cross-origin dev mode).

---

### Phase 1 — Batch runs (1.5–2 tuần)

**Backend**
- Migration **013** thêm `cancel_requested BOOL` cột.
- Sửa cron [internal/compute/job.go](internal/compute/job.go) poll `cancel_requested` giữa phase.
- `BatchRunObserver` interface emit event vào `eventbus`.
- Endpoints:
  - `GET /overview` + alert rules
  - `GET /namespaces/{ns}/dashboard`
  - `GET /batch-runs/{id}`
  - `POST /batch-runs/{id}/cancel`
  - `POST /batch-runs/{id}/retry` + reject rules
  - `GET /batch-runs/stats?window=&bucket=`
  - `GET /batch-runs/{id}/stream` (204 khi terminal)
  - `GET /stream` (skeleton, emit `batch_run.*`)
- Sửa `GET /batch-runs` trả `BatchRunSummary`.
- Xóa `GET /namespaces?include=overview`.

**Frontend**
- `/` Fleet overview, `/namespaces`, `/namespaces/new`.
- `/ns/:ns` Overview (phase strip + dashboard aggregate).
- `/ns/:ns/config`.
- `/batch-runs` + `/ns/:ns/batch-runs` (time-series).
- `/batch-runs/:id` (SSE-driven khi đang chạy, snapshot khi terminal).
- `/health`.
- `LogLineViewer`, `PhaseStrip`, `TimeSeriesChart` primitives.

**Exit:** operator trigger run → mở detail → thấy log real-time → cancel → confirm.

---

### Phase 2 — Catalog & Embedder pipeline (2 tuần)

**Backend**
- Migration **014** `catalog_backlog_samples` + retention.
- Sampler trong `cmd/embedder` snapshot backlog mỗi 30s (skip khi không thay đổi, §8).
- Embedder publish `codohue:catalog-events:{ns}` pub/sub.
- Metrics: `codohue_catalog_pending_items`, `codohue_catalog_consumer_lag`.
- Endpoints `/catalog`, `/catalog/backlog-history`, `/catalog/failures-summary`, `/catalog/stream`, `/catalog/items` (mở rộng), `/catalog/re-embed` (mở rộng `only_state`).
- `/stream` mở rộng emit `catalog.dead_letter_grew`, `batch_run.reembed_progress`.

**Frontend**
- `/ns/:ns/catalog`, `/ns/:ns/catalog/items`, `/ns/:ns/catalog/items/:id`.
- Re-embed progress sticky overlay.
- `BacklogChart` primitive.

**Exit:** thấy backlog trôi theo timeline persisted; redrive dead-letter; chứng minh failure reason chiếm tỷ trọng cao nhất.

---

### Phase 3 — Live event ingest (1.5 tuần)

**Backend**
- Metrics: `codohue_events_ingested_total`, `codohue_ingest_errors_total`.
- Endpoints `/events/stream` (+ janitor consumer group ở `cmd/admin`), `/events/summary`, sửa `POST /events` trả `event_id`.
- `/metrics/summary` curated.

**Frontend**
- `/ns/:ns/events` — live tail + sidebar summary.
- `/ns/:ns/events/inject`.
- Fleet tile "events/s" gắn `/metrics/summary`.

**Exit:** tail real-time, inject xuất hiện < 2s, action mix so với baseline.

---

### Phase 4 — Polish & hardening (1 tuần)

**Backend**
- Migration **015** retention `batch_run_logs` + cron task.
- Audit toàn bộ endpoint cho graceful shutdown SSE.
- Stress test: 100 concurrent SSE consumers / 1k events/s tail (xem §12.2).
- Self-observability metrics (§12.3).
- Documentation: cập nhật [CLAUDE.md](CLAUDE.md) §REST API — admin.

**Frontend**
- Command palette index đầy đủ.
- `/ns/:ns/trending`, `/ns/:ns/debug`, `/ns/:ns/demo-data`, `/danger-zone`.
- Dirty-form guard.
- Error boundary toàn cục + toast notification gắn `/stream`.
- README + plan này cập nhật.

**Exit:** Cut over, xóa code v1 frontend cũ.

---

### Timeline tóm tắt

| Phase | Tuần | Phát hành |
|---|---|---|
| 0 — Foundation | 1 | Skeleton, login |
| 1 — Batch runs | 2–3 | Run real-time, cancel/retry, fleet overview |
| 2 — Catalog | 4–5 | Backlog timeline persisted + SSE + redrive |
| 3 — Live ingest | 6–7 | Event tail SSE + summary |
| 4 — Polish | 8 | Cut over + retention |

Tổng ~8 tuần.

---

## 8. Schema changes & migrations

### Migration 013 — batch_run_logs cancel support

```sql
-- UP
ALTER TABLE batch_run_logs
  ADD COLUMN cancel_requested BOOL NOT NULL DEFAULT false;
CREATE INDEX idx_batch_run_logs_running_cancel
  ON batch_run_logs (cancel_requested)
  WHERE completed_at IS NULL;

-- DOWN
DROP INDEX IF EXISTS idx_batch_run_logs_running_cancel;
ALTER TABLE batch_run_logs DROP COLUMN IF EXISTS cancel_requested;
```

Cron poll: giữa các phase đọc `SELECT cancel_requested FROM batch_run_logs WHERE id = $1`. Nếu true → set `success=false, error_message='operator_cancelled', completed_at=now()`, return early. Cancel mid-phase không khả thi mà không refactor sâu — chấp nhận cancel-between-phases.

### Migration 014 — catalog_backlog_samples

```sql
-- UP
CREATE TABLE catalog_backlog_samples (
  namespace   TEXT        NOT NULL,
  sampled_at  TIMESTAMPTZ NOT NULL,
  pending     INT         NOT NULL,
  in_flight   INT         NOT NULL,
  failed      INT         NOT NULL,
  dead_letter INT         NOT NULL,
  stream_len  INT         NOT NULL,
  PRIMARY KEY (namespace, sampled_at)
);
CREATE INDEX idx_catalog_backlog_samples_ns_time
  ON catalog_backlog_samples (namespace, sampled_at DESC);

-- DOWN
DROP TABLE IF EXISTS catalog_backlog_samples;
```

Sampler ở `cmd/embedder`: mỗi 30s đọc backlog cho mọi ns enabled.

**Sampler skip rule** (tránh bloat):
- Insert sample khi: (a) ≥ 5 phút từ sample trước, **hoặc** (b) bất kỳ field nào (pending/in_flight/failed/dead_letter/stream_len) thay đổi so với sample trước.
- Nếu không có ns nào enabled → sampler không chạy query.

Retention (chạy hourly cùng `cmd/cron`): `DELETE FROM catalog_backlog_samples WHERE sampled_at < now() - interval '7 days'`. Env: `CODOHUE_CATALOG_BACKLOG_RETENTION_DAYS=7`.

### Migration 015 — batch_run_logs retention

```sql
-- UP
CREATE INDEX idx_batch_run_logs_started_at
  ON batch_run_logs (started_at);

-- DOWN
DROP INDEX IF EXISTS idx_batch_run_logs_started_at;
```

`cmd/cron` thêm retention task chạy hourly:
- **Tier 1 (log strip)**: `UPDATE batch_run_logs SET log_lines = NULL WHERE started_at < now() - interval $1 AND log_lines IS NOT NULL` — giữ phase metadata, vứt log JSONB nặng.
- **Tier 2 (delete)**: `DELETE FROM batch_run_logs WHERE started_at < now() - interval $2`.

Env tunables (default 30/90 ngày):
- `CODOHUE_BATCH_LOG_RETENTION_DAYS=30`
- `CODOHUE_BATCH_RUN_RETENTION_DAYS=90`

### Code-side: `internal/admin/eventbus`

```go
type Event struct {
    Kind      string
    Namespace string
    EntityID  string
    Payload   any
    At        time.Time
}

type Bus interface {
    Publish(ctx context.Context, e Event)
    Subscribe(filter Filter) (<-chan Event, func())
}
```

Implement in-memory channel-based bus. Test concurrent fan-out 100 subscribers / 10k events.

### Code-side: `internal/admin/sse`

Helper wrap `http.ResponseWriter`:
- Set headers `Content-Type: text/event-stream`, `Cache-Control: no-cache`, `Connection: keep-alive`, `X-Accel-Buffering: no`.
- Flush sau mỗi `event:` write.
- Heartbeat ticker.
- Detect disconnect qua `r.Context().Done()`.
- Helper `ssetest.ReadEvents(t, resp, n)` cho test (§12.1).

### Code-side: `internal/admin/metricsroll`

- Slot rolling window cho counter (rate 1m/5m/1h) và histogram (t-digest p50/p95/p99).
- Goroutine timer 10s đọc Prometheus counter cumulative value, push vào ring buffer.
- API: `slot.Rate(window time.Duration)`, `slot.Percentile(p float64)`.

---

## 9. Rủi ro & quyết định cần chốt

1. **SSE single-replica.** In-process eventbus không thấy event ở replica khác. Plan giả định single-replica. Multi-replica → swap implementation sang Redis pub/sub. Interface đã trừ chỗ.

2. **Cancel batch chỉ giữa phase.** Cancel khi đang giữa phase 1 không khả thi mà không refactor sâu cron. UI hiển thị rõ.

3. **SSE consumer ephemeral group ở event tail.** Plan có janitor trong `cmd/admin` mỗi 5 phút (§3.4). Vẫn cần monitor `codohue_admin_sse_orphan_groups_reaped_total` để biết khi nào leak nhanh.

4. **Backpressure SSE.** Bounded channel 1024, drop oldest + emit `event: dropped`.

5. **Chart cost.** Recharts ~150KB gzipped. Đo `dist/` sau Phase 2 vs budget §12.2. Nặng quá → swap sparkline tự viết cho view nhỏ.

6. **JSON shape drift.** Sửa thẳng `/v1` không versioning → mọi struct phải có Go test JSON roundtrip + TypeScript type generator. Tool generator chốt ở §13 D7.

7. **Reverse proxy buffering.** Nginx mặc định buffer `text/event-stream` → SSE bị delay. SSE handler set `X-Accel-Buffering: no` (§3 + §8 sse helper). Cloudflare/Envoy/Caddy cần kiểm tra config tương đương khi deploy — document trong [CLAUDE.md](CLAUDE.md) deploy section khi tới prod.

8. **Davinci package version drift.** Davinci là internal npm package (`@jarviisha/davinci-*`); breaking change rename token (xem migration table cuối [USAGE.md](web/admin/USAGE.md)) phải đồng bộ qua changeset. Lock version exact trong `package.json`, theo release notes Davinci. Khi Davinci bump, đánh giá impact trước khi merge.

---

## 10. Out of scope

Mobile sidebar drawer · i18n · RBAC / multi-user · theme tùy biến ngoài light/dark · density toggle · Grafana embed in-app · icon system (đợi cấp) · multi-replica admin · cancel mid-phase · event replay (chỉ tail forward, không seek backward qua SSE) · backward compatibility cho admin API (dev mode, đập đi xây lại tự do).

---

## 11. Checklist trước khi đóng plan

- [x] Bộ design system đã chốt — Davinci (D8 §13).
- [x] Visual identity đã chốt — single-canvas, tone Badge, Breadcrumbs (D9 §13).
- [ ] Pin version Davinci packages trong `package.json` ở Phase 0.
- [ ] Chốt single vs multi-replica admin → cập nhật §9.1.
- [x] Chốt chart library — Recharts (D6 §13).
- [ ] Chốt cách generate TypeScript type từ Go struct → §13 D7 (default `tygo`).
- [ ] Xác nhận schema `BatchRunDetail` / `BatchRunSummary` / `OverviewResponse` / `NamespaceDashboardResponse` / `PhaseEntry` trong [internal/admin/types.go](internal/admin/types.go) trước khi code Phase 1.
- [ ] Mở migration tickets 013, 014, 015.
- [ ] Mở issue cho từng endpoint mới (mỗi phase ~5–8 issue).
- [ ] Confirm performance budget §12.2 với team (đặc biệt 900KB tạm — đo lại sau Phase 0).

---

## 12. Test & Operational targets

### 12.1 Test strategy

**Backend**
- **SSE handler:** `httptest.NewServer` + đọc response stream, parse `event:` lines, assert sequence. Helper `internal/admin/sse/ssetest.ReadEvents(t, resp, n)` để test ergonomic.
- **Aggregate endpoints** (`/overview`, `/dashboard`, `/batch-runs/stats`): integration test với postgres + redis test container; verify shape + nội dung với fixture data.
- **Eventbus:** unit test concurrent fan-out 100 subscribers / 10k events — no drop, ordered per topic.
- **Cron cancel:** integration test trigger run → set `cancel_requested = true` mid-phase → assert run end với `error_message='operator_cancelled'`.
- **Retention task:** integration test seed row `started_at = now() - 100 days` → run retention → assert DELETE; row `now() - 40 days` → assert `log_lines IS NULL` nhưng row còn.
- **JSON shape contract:** mọi struct trong `internal/admin/types.go` có `TestJSONRoundtrip<TypeName>` đảm bảo `json.Marshal → json.Unmarshal` không mất field.

**Frontend**
- **SSE hook:** Vitest + `EventSourcePolyfill` mock. Assert hook state thay đổi theo event sequence được mock (phase_started → phase_completed → run_completed).
- **Component:** React Testing Library cho primitives (`StatusToken`, `PhaseStrip`, `LogLineViewer`) và pages chính (`FleetOverviewPage`, `BatchRunDetailPage`).
- **Smoke URL:** `tests/urls.test.mjs` (đã có) chặn raw `fetch(`.
- **E2E:** Playwright smoke cho golden path (login → fleet → mở namespace → trigger run → thấy SSE log line). Chạy tay trước cut-over Phase 4, không bắt buộc CI.

### 12.2 Performance budget

| Target | Threshold | Đo bằng |
|---|---|---|
| `GET /overview` p95 latency | < 500ms với 10 ns | k6 / wrk |
| `GET /namespaces/{ns}/dashboard` p95 | < 300ms | k6 / wrk |
| `GET /events/summary?window=1h` p95 | < 800ms với 1M events trong cửa sổ | k6 |
| `GET /catalog/backlog-history?window=24h` p95 | < 200ms | k6 |
| SSE event-to-client latency p95 | < 1s | probe tự viết (timestamp event → timestamp UI nhận) |
| `web/admin/dist/` total gzipped | < 900KB tạm thời (đo Davinci impact ở Phase 0, siết về 600KB ở Phase 4) | `du -b dist/ \| awk` trong CI |
| Concurrent SSE connections single replica | ≥ 100 ổn định | stress test Phase 4 |

Vi phạm budget → block PR, mở issue trước khi merge phase tiếp.

### 12.3 Self-observability admin plane

Metric mới ở `cmd/admin` (riêng namespace với metric data-plane §3.8):

- `codohue_admin_sse_connections_active{stream}` — Gauge, `stream ∈ {ops, batch_run, catalog, events}`.
- `codohue_admin_sse_dropped_total{stream, reason}` — Counter, `reason ∈ {backpressure, client_slow, server_shutdown}`.
- `codohue_admin_sse_reconnects_total{stream}` — Counter.
- `codohue_admin_eventbus_publish_total{kind}` — Counter.
- `codohue_admin_eventbus_subscribers{kind}` — Gauge.
- `codohue_admin_sse_orphan_groups_reaped_total` — Counter (janitor §3.4).

Plot trên Grafana ngoài; **không** proxy vào `/metrics/summary` — tách concern: operator monitor data plane qua admin UI, admin plane chính nó monitor qua Prometheus + Grafana như mọi infra khác.

---

## 13. Decisions log

Quyết định nhỏ đã chốt, ghi lại để tránh đặt câu hỏi lần nữa.

| # | Quyết định | Lý do |
|---|---|---|
| D1 | SSE 401 → UI redirect `/login?next=<current>` | Session 8h có thể expire mid-stream. `useServerStream` trigger global auth context khi nhận 401 từ EventSource, không retry. |
| D2 | Last-Event-ID resume defer Phase 4 (hoặc later) | MVP không cần; UI buffer client-side đủ. Implement khi user complaint "miss event". |
| D3 | Browser 6-SSE-per-host limit → yêu cầu HTTP/2 ở deploy | Tránh complexity multiplex single connection. Document trong deploy doc. Dev Vite proxy HTTP/2 không bắt buộc (1 tab × 4 stream < 6). |
| D4 | `POST /batch-runs/{id}/retry` reject rules | 409 nếu run gốc đang chạy. 422 nếu re-embed mà catalog disabled. 422 nếu namespace đã xóa. 404 nếu run gốc không tồn tại. |
| D5 | `only_state` use case | `all` (default) — re-embed mọi item, dùng khi đổi strategy. `embedded` — upgrade strategy version (v1 → v2). `failed` — retry sạch failed sau khi fix root cause, không đụng dead_letter. |
| D6 | Chart library: Recharts | Cost ~150KB gzipped chấp nhận được; ecosystem lớn; đủ line/area/donut/sparkline. Visx mạnh hơn nhưng phải tự assemble nhiều, dồn cost dev time. |
| D7 | TypeScript type generation: lựa chọn 2 cách, chốt Phase 0 | (a) `tygo` generate `.d.ts` từ Go struct, chạy trong `go generate ./...`; (b) viết tool nhỏ dùng `go/ast` đọc `internal/admin/types.go` rồi emit `.ts`. Mặc định nghiêng (a) — ít code maintain. |
| D8 | Bộ design system: **Davinci** (4 package nội bộ) | Đã có sẵn full primitives + AppShell layout + theme/dark/system + token system. Cắt ~3–5 ngày build primitives custom. Xem [web/admin/USAGE.md](web/admin/USAGE.md) cho install + [web/admin/DESIGN.md](web/admin/DESIGN.md) cho philosophy. |
| D9 | Visual identity: bỏ bracketed status tokens + bỏ PS1 prompt | Đi thuần Davinci philosophy (single-canvas, border-led, fill-for-intent). Status qua `<Badge tone>` / `<Alert variant>`. Location qua `<Breadcrumbs>`. Operator-console aesthetic (kubectl/dmesg/htop) bị hy sinh để nhất quán với phần còn lại của ecosystem Davinci. |

