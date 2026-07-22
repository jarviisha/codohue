# Kế hoạch xử lý các vấn đề từ đợt audit 2026-07-22

Nguồn: audit toàn repo (6 nhóm domain, ~60 phát hiện, các phát hiện nặng đã được
xác minh trực tiếp trên code). Kế hoạch chia 6 phase theo thứ tự ưu tiên:
**mất dữ liệu thật → kẹt trạng thái vĩnh viễn → kết quả gợi ý sai → admin plane
→ bảo mật → config/contract**. Mỗi task ghi rõ file đích, cách sửa, tiêu chí
nghiệm thu (AC) và ước lượng (S ≤ nửa ngày, M ≤ 2 ngày, L > 2 ngày).

Ba mẫu lỗi gốc cần giữ trong đầu khi sửa từng task:

1. **Cơ chế cứu hộ được viện dẫn nhưng không tồn tại** (recovery sweep, PEL
   reaper, orphan-run sweeper) — phase 1–2 xây các lưới an toàn này thật.
2. **Hai process, một state, không khoá** (admin + cron cùng chạy
   `compute.Job`) — giải bằng Postgres advisory lock, task 2.4.
3. **Degrade rồi báo xanh** (nuốt lỗi upsert, dashboard stub OK, cache kết quả
   fallback) — nguyên tắc chung: *degrade thì được, nói dối về trạng thái thì
   không*.

---

## Phase 1 — Chặn mất dữ liệu (làm ngay, trước mọi thứ khác)

> **✅ HOÀN THÀNH 2026-07-22** — 1.1 `fix(idmap)`, 1.2 `fix(ingest)`,
> 1.3 `feat(embedder)`. Unit test + lint xanh; AC tích hợp (e2e với hạ tầng
> thật) sẽ chạy cùng đợt soak test sau Phase 2.

### 1.1 `id_mappings`: chuyển sang khóa composite `(namespace, entity_type, string_id)` — **L**

**Vấn đề (F1):** `migrations/001_initial.up.sql:27` khai `string_id TEXT PRIMARY KEY`
toàn cục; `internal/core/idmap/repository.go:36` conflict trên `string_id`.
Hai namespace / hai entity_type dùng chung một row → xoá namespace A phá mapping
của namespace B; id vừa subject vừa object va nhau; delete catalog item không
xoá được point Qdrant.

**Cách sửa:**

- Migration `022_id_mappings_composite.up.sql`:
  - Drop PK cũ, thêm `PRIMARY KEY (namespace, entity_type, string_id)`;
    giữ `numeric_id BIGSERIAL UNIQUE`.
  - Row hiện hữu giữ nguyên (không tách được row bị share — namespace "mượn"
    row của namespace khác sẽ được cấp numeric id mới ở lần `GetOrCreate` sau).
- `idmap/repository.go`: `ON CONFLICT (namespace, entity_type, string_id) DO
  UPDATE ... RETURNING numeric_id`.
- **Bước hậu migration bắt buộc** (ghi vào migration notes + README): chạy
  full recompute cho mọi namespace + re-embed catalog để Qdrant dùng numeric id
  mới; point cũ dọn bằng task 3.6 (set-diff delete) hoặc recreate collection.
- Sửa CLAUDE.md nếu cần (hiện mô tả "scoped by (namespace, entity_type)" —
  sau task này mô tả mới đúng).

**AC:** test tích hợp: cùng `string_id` ở 2 namespace ra 2 numeric id khác nhau;
cùng string làm subject và object trong 1 namespace ra 2 numeric id; wipe
namespace A không ảnh hưởng lookup của B. Down migration khôi phục PK cũ (chấp
nhận fail nếu đã có row trùng `string_id` — ghi chú rõ trong file down).

### 1.2 Ingest worker: PEL recovery + NOGROUP recovery + backoff — **M**

**Vấn đề (F2, F8-ingest):** `internal/ingest/worker.go:69-91` chỉ đọc `">"`,
không XAUTOCLAIM; lỗi `handleMessage` → log + continue không ack → event mất
vĩnh viễn. NOGROUP (Redis restart không persistence) → hot-loop 100% CPU.
Consumer name hardcode `"worker-1"`.

**Cách sửa (mô phỏng theo `internal/embedder/worker.go:273-285` đã làm đúng):**

- Thêm reaper goroutine: `XAutoClaim(minIdle=60s, start="0")` định kỳ, đưa
  message pending quay lại xử lý; write-before-ack giữ nguyên.
- Nhánh lỗi `XReadGroup`: phát hiện NOGROUP → gọi lại `ensureGroup`
  (XGroupCreateMkStream) rồi tiếp tục; mọi lỗi khác backoff (1s, cap 30s).
- Consumer name lấy từ env/hostname như embedder (`CODOHUE_INGEST_REPLICA_NAME`,
  default hostname) để nhiều replica không giẫm PEL của nhau.
- `cmd/api/main.go`: chờ worker goroutine thoát trước khi `closeRedisFn`/
  `closePoolFn` (WaitGroup / done channel, sửa thứ tự defer) — hiện event đã
  ghi DB vẫn có thể mất XACK khi shutdown.

**AC:** e2e: kill worker giữa batch → restart → event pending được xử lý đủ,
không mất, không double-insert (thêm idempotency check nếu cần — chấp nhận
at-least-once + INSERT thường vì bảng events không có unique key tự nhiên; ghi
rõ quyết định này vào docs.go của ingest). Unit test cho nhánh NOGROUP.

### 1.3 Hiện thực recovery sweep cho catalog/embedder — **M**

**Vấn đề (F3):** 7 comment viện dẫn "recovery sweep" không tồn tại
(`catalog/service.go:129`, `admin/catalog_ops_service.go:112,162,297,333`,
`embedder/service.go:286`). Row kẹt `pending` khi XADD fail; re-embed run treo
vĩnh viễn vì `CountStaleCatalogItems > 0`; mọi re-embed sau 409.

**Cách sửa:**

- Goroutine sweep mới trong `cmd/embedder` (cạnh backlog sampler, interval
  ~2 phút, per-namespace):
  - `SELECT ... FROM catalog_items WHERE state = 'pending' AND updated_at <
    NOW() - interval '5 minutes'` → XADD lại vào `catalog:embed:{ns}`.
  - Tương tự cho `in_flight` kẹt quá lâu (> 15 phút): reset về `pending` +
    republish (giải luôn hậu quả của F5/markDeadLetter nuốt lỗi).
- Sweep phải idempotent với consumer đang chạy: duplicate delivery đã được
  chống bởi task 2.3 (idempotency check theo content_hash).
- Xoá/sửa toàn bộ comment nói về sweep cho khớp hành vi thật.

**AC:** e2e: tắt Redis lúc catalog ingest (XADD fail, row pending) → bật lại →
sweep republish → item embedded trong ≤ 2 chu kỳ sweep. Re-embed run đang treo
vì item pending kẹt sẽ tự đóng sau khi sweep chạy.

---

## Phase 2 — Gỡ các trạng thái kẹt vĩnh viễn

> **✅ HOÀN THÀNH 2026-07-22** — 2.1+2.4 `fix(compute): cross-process run
> lock...`, 2.2+2.3 `fix(embedder): worker resilience...`, 2.5
> `fix(compute): svd padding...`. Unit test + lint xanh. Ghi chú lệch nhỏ so
> với plan: guard `runningBatch` sync.Map được **bỏ hẳn** (advisory lock phủ
> cả in-process); timeout manual run đặt 1h (const `manualRunTimeout`) thay
> vì "cấu hình được"; per-item timeout cho ProcessItem chưa làm (rủi ro đã
> giảm nhờ idempotency + hash guard).

### 2.1 Batch run lifecycle: async thật + orphan reaper + defer order — **M**

**Vấn đề (F6-cron, F6-admin):**
- `cmd/cron/main.go:48,54`: defer LIFO đóng pool trước cancel → SIGTERM giữa
  run không finalize được row → kẹt "running" 30 ngày.
- `internal/admin/service.go:673-677` + `handler.go:461`: "202 Accepted" nhưng
  chạy đồng bộ trên `r.Context()` + cap 10 phút → client disconnect giết run
  giữa chừng; `GetBatchRunLogs(...)[0]` có thể trỏ nhầm run của cron.

**Cách sửa:**

- `cmd/cron/main.go`: sắp lại thứ tự — cancel ctx, **join** job goroutine
  (WaitGroup), rồi mới close pool. Finalize dùng context tách
  (`context.WithoutCancel` + timeout riêng 10s) để UPDATE luôn tới được DB.
- `internal/compute/job.go`: mọi UPDATE finalize (`UpdateBatchRunLog`,
  `UpdateBatchRunPhases`) dùng detached context như trên — áp dụng cho cả
  đường cron lẫn đường admin.
- `CreateBatchRun` chuyển async thật: INSERT row "running" trước (lấy id
  chắc chắn — sửa luôn lỗi trả nhầm `logs[0]`), spawn goroutine chạy
  `RunNamespace` trên `context.Background()` + timeout cấu hình được, trả 202 +
  Location ngay. `runningBatch` sync.Map giữ để chống double-submit trong
  process; chống cross-process bằng task 2.4.
- Orphan reaper: lúc khởi động cron (và mỗi tick), `UPDATE batch_run_logs SET
  success=false, error='orphaned by process restart', completed_at=NOW()
  WHERE completed_at IS NULL AND started_at < NOW() - interval '1 hour'`
  (ngưỡng > timeout run tối đa).

**AC:** e2e: SIGTERM cron giữa phase → restart → row được đóng
(`orphaned...`), retry hoạt động lại (hết 409 vĩnh viễn). Admin tạo run rồi
đóng connection ngay → run vẫn chạy tới cùng, row hoàn tất. Response
`CreateBatchRun` trả đúng id của run vừa insert.

### 2.2 Embedder worker: ensureGroup retry, NOGROUP, ensured-cache, XTrim — **M**

**Vấn đề (F4-boot, F4-nogroup, F9-stream, F10-cache):**
- `embedder/worker.go:210-213`: `ensureGroup` fail một lần → consumer chết,
  ns kẹt trong `w.cancels`, không bao giờ hồi phục.
- `worker.go:224-247`: NOGROUP retry mù, không re-create group.
- `embedder/service.go:419-440`: cache `ensured` không invalidate → wipe +
  recreate namespace → dead-letter toàn bộ đến khi restart.
- Không có `XTrim`/`MaxLen` → stream phình vô hạn; `stream_len` của backlog
  sampler thành tổng all-time.

**Cách sửa:**

- `consumeStream`: khi `ensureGroup` fail → retry với backoff trong vòng lặp
  thay vì return; nếu return thì phải xoá ns khỏi `w.cancels` để refresh
  khởi động lại.
- Nhánh lỗi XReadGroup: match NOGROUP → gọi `ensureGroup` lại rồi tiếp tục.
- `ensured` map: xoá entry khi upsert Qdrant fail vì collection-not-found
  (hoặc đơn giản hơn: TTL 5 phút cho cache). Key cache gộp cả
  `embedding_dim` để đổi dim là ensure lại.
- XADD ở `catalog/service.go` + `admin/catalog_ops_service.go` thêm
  `MaxLen ~ 100_000` (approximate trim). Backlog sampler đổi `XLEN` sang
  `XPENDING`-based hoặc `XLEN - entries-acked` — đơn giản nhất: dùng lag của
  consumer group (`XINFO GROUPS` → `lag`) làm số backlog.

**AC:** e2e: start embedder trước Redis → Redis lên sau → consumer tự hồi phục,
item được xử lý. Xoá + tạo lại namespace không cần restart embedder. Stream
sau 200k ingest giữ ≤ ~100k entry. Metric backlog về 0 khi tiêu thụ hết.

### 2.3 Embedder ProcessItem: idempotency + không nuốt lỗi dead-letter — **M**

**Vấn đề (F5, F8-attempts, F9-markembedded, F12):**
- `markDeadLetter` nuốt lỗi DB rồi vẫn `OutcomeDeadLetter` → ACK → row kẹt
  `in_flight` không endpoint nào cứu.
- Không có short-circuit "đã embedded đúng hash+strategy" + `attempt_count`
  không reset → duplicate delivery đủ nhiều biến item khỏe thành dead_letter.
- `MarkEmbedded` không guard `content_hash` → vector stale được ghi nhận
  `embedded` vĩnh viễn.
- `markFailed` lúc shutdown dùng ctx đã cancel → ghi state chắc chắn fail.

**Cách sửa:**

- Đầu `ProcessItem`: nếu row đã `embedded` với cùng `content_hash` +
  `strategy_id/version` đích → `OutcomeSkipped` (ACK, không tăng attempt).
- `markDeadLetter` fail → trả outcome **không ACK** (thêm `OutcomeRetryLater`
  với `ShouldAck() == false`) để entry ở lại PEL cho reaper; sweep 1.3 là lưới
  cuối.
- `MarkEmbedded ... WHERE id = $1 AND content_hash = $2` — hash đổi giữa
  chừng thì bỏ qua (row đang `pending` chờ republish của bản mới).
- `MarkEmbedded` reset `attempt_count = 0`.
- Mọi state-write trong nhánh shutdown/cancel dùng
  `context.WithoutCancel(ctx)` + timeout ngắn.

**AC:** unit test: deliver cùng entry 10 lần → item vẫn `embedded`,
`attempt_count` không tích lũy. Test hash-đổi-giữa-chừng: MarkEmbedded không
đè `pending` của content mới. Test markDeadLetter DB-fail → entry không ACK.

### 2.4 Khoá liên process per-namespace cho compute — **M**

**Vấn đề (F: admin-vs-cron race, delete-vs-cron resurrection):** guard chỉ là
`sync.Map` trong process admin; cron là process khác → hai full recompute
interleave, vector mixed-generation; xoá namespace xong cron tick đang bay
upsert lại collection mồ côi.

**Cách sửa:**

- Postgres advisory lock theo namespace:
  `pg_try_advisory_lock(hashtext('codohue:compute:' || ns))` bọc quanh
  `RunNamespace` (cả cron tick lẫn admin manual run). Không lấy được lock →
  cron skip ns tick này (log); admin trả 409 "run already in progress".
- `DeleteNamespace` (admin) lấy **cùng lock** trước khi wipe → không bao giờ
  wipe giữa lúc recompute, và tick sau delete không thấy ns trong
  `namespace_configs` nữa nên không tái tạo collection.

**AC:** e2e: POST batch-run trong lúc cron tick đang chạy cùng ns → 409.
Delete namespace trong lúc tick chạy → chờ lock, wipe sạch, không còn
collection Qdrant mồ côi sau đó.

### 2.5 Compute: SVD padding, propagate lỗi phase, đúng lambda, neg-sampling — **M**

**Vấn đề (F7-svd, F8-phase1, F9-phase3, F18):**
- `compute/dense.go:230,242`: vector SVD ngắn hơn `embedding_dim` → mọi
  namespace nhỏ fail phase 2 mọi tick.
- `compute/service.go:80-107`: nuốt toàn bộ lỗi upsert, run vẫn xanh;
  `upsertObjectVectors` bỏ dở batch còn lại vẫn báo đủ count.
- `job.go:241-256`: lỗi phase 3 không vào `success`.
- `dense.go:154-159`: negative sampling loại trùng target thay vì context;
  `dense.go:212`: SVD dùng `defaultLambda` thay vì `cfg.Lambda`.

**Cách sửa:**

- SVD: pad vector bằng 0 lên đủ `embeddingDim` (`vec := make([]float32,
  embeddingDim)` rồi fill `rank` phần tử đầu).
- Phase 1: đếm `failedUpserts`; nếu **toàn bộ** upsert fail (hoặc tỉ lệ fail
  100% một loại) → return error để phase đỏ. Per-item fail lẻ tẻ giữ hành vi
  continue (đã có test codify), nhưng count trả về phải là số **thành công**.
- Phase 3 lỗi → fold vào `success` (hoặc nếu quyết giữ "trending là phụ" thì
  ghi rõ điều đó vào CLAUDE.md + hiển thị badge riêng trên UI — chọn 1,
  đừng để im lặng).
- `sgdUpdate` negative: skip khi `negIdx == ctxIdx` (giữ thêm skip target nếu
  muốn, vô hại). SVD dùng `cfg.Lambda`.

**AC:** unit: namespace 10 subject / dim 64 → phase 2 svd thành công, vector
64 chiều. Mock Qdrant fail toàn bộ → `phase1_ok=false`. Test lambda: đổi
`cfg.Lambda` thấy score SVD đổi.

---

## Phase 3 — Recommend trả kết quả đúng

### 3.1 `hybridCold`: sửa offset ở nhánh degraded — **S**

**Vấn đề (F11):** `recommend/service.go:614-622` trả thẳng `cfResp`/
`popularResp` build với `Offset:0, Limit:offset+limit` → sai trang, và bị
cache dưới key của trang đúng.

**Cách sửa:** tách helper `paginate(resp, offset, limit)` áp cho cả 3 nhánh
(blend đã đúng); nhánh degraded gọi helper trước khi return, set lại
`Offset`/`Limit`/`Total` trung thực.

**AC:** unit test cho cả hai nhánh degraded với `offset>0` — item đầu tiên
phải là rank offset+1, response ghi đúng offset/limit.

### 3.2 Không cache response degraded (hoặc TTL ngắn) — **S**

**Vấn đề (F17):** Qdrant chớp 1 giây → response `fallback_popular` bị cache
5 phút dưới key thường.

**Cách sửa:** `Recommend` chỉ cache khi `resp.Source` là source "chính danh"
của tier đó (CF/hybrid/hybrid_cold); source fallback-do-lỗi → skip cache
hoặc TTL 15s. Cần phân biệt "fallback vì cold-start" (cache được) với
"fallback vì lỗi hạ tầng" (không cache) — thêm field nội bộ `degraded bool`
trên Response (không ra wire).

**AC:** test: lỗi Qdrant 1 lần → request sau (Qdrant sống lại) trả CF chứ
không dính cache popular.

### 3.3 Trending/popular pagination: không fall-through khi hết trang — **S**

**Vấn đề (F12):** `service.go:725-747` nhầm "trang rỗng" với "trending
unavailable" → page qua cuối ZSET rơi sang popular, item lặp.

**Cách sửa:** phân biệt bằng tổng số phần tử: nếu `ZCard > 0` (trending có
data) mà trang rỗng → trả trang rỗng (kết thúc pagination). Chỉ fallback
popular khi ZSET không tồn tại/lỗi/rỗng hoàn toàn. Áp cả nhánh authored-filter
(`entries = nil` tại 716-719).

**AC:** test: ZSET 25 item, `offset=30` → items rỗng, không gọi popular.

### 3.4 `Rank` trả đủ candidate — **S**

**Vấn đề (F13):** candidate không có điểm sparse biến mất khỏi response.

**Cách sửa:** sau khi score từ Qdrant, mọi candidate không có mặt trong
`scored` được thêm với `Score: 0` (nhất quán với `rankFallback`), giữ nguyên
thứ tự sau các item có điểm.

**AC:** gửi 500 candidate trong đó 470 chưa từng ingest → response 500 item.

### 3.5 `window_hours` trung thực — **S**

**Vấn đề (F14):** chỉ có một ZSET theo window cấu hình; param chỉ đổi nhãn.

**Cách sửa (chọn 1, đề xuất a):**
a. Bỏ param khỏi handler + docs; response ghi window thực từ config.
b. Hoặc giữ param nhưng response ghi `window_hours` = window thực của ZSET và
   thêm trường `requested_window_hours` — không nói dối nữa.
Lưu ý wire contract: đổi kiểu/field cần cập nhật golden snapshot có chủ đích.

**AC:** response luôn phản ánh window thật của dữ liệu.

### 3.6 Xoá point rời cửa sổ 90 ngày (set-diff delete) — **M**

**Vấn đề (F19):** full recompute chỉ upsert, không xoá → subject/object hết
hạn giữ vector đông cứng vĩnh viễn trong Qdrant.

**Cách sửa:** cuối phase 1 (và phase 2 cho dense do cron sở hữu): lấy danh
sách point id hiện có trong collection (scroll ids), diff với tập id vừa
upsert, `Delete` phần thừa. Với `dense_source="catalog"` **không** xoá
`{ns}_objects_dense` (embedder sở hữu) — chỉ dọn `{ns}_subjects_dense`.
Đây cũng là bước dọn point mồ côi sau migration 022 (task 1.1).

**AC:** e2e: object có toàn event > 90 ngày → sau tick, point biến mất khỏi
`{ns}_objects`; object catalog vẫn còn trong `{ns}_objects_dense`.

### 3.7 γ-freshness đối xứng cho dense + seen-filter cho trending trong hybridCold — **M**

**Vấn đề (F15, F16):** không producer dense nào ghi `created_at` payload →
item dense-only không bao giờ decay. `hybridCold` lọc seen cho phần CF nhưng
không lọc phần trending.

**Cách sửa:**
- Thêm `created_at` vào payload dense ở cả 3 producer: `compute/dense.go`
  (~396-400), `embedder/service.go` (~458-463; catalog_items có timestamp),
  BYOE (`recommend/service.go:238-242`, nhận `object_created_at` optional
  trên request — cân nhắc wire contract).
- `fallbackTrending`/`fallbackPopular` trong đường hybridCold nhận thêm tập
  seen-ids (đã build sẵn cho CF) và drop trước khi page, giống cách đang làm
  với authored-ids.

**AC:** test hybrid: 2 item cùng tuổi, một chỉ về từ dense — điểm sau decay
tương đương. Test hybridCold: item vừa tương tác không xuất hiện ở phần
trending.

---

## Phase 4 — Admin plane

### 4.1 Sửa scoping + Location + hai lỗi SSE có sẵn test sai — **S**

- **F20:** `admin/handler.go` `GetBatchRuns`: đọc `chi.URLParam(r, "ns")`
  trước, fallback query param (route `/batch-runs` toàn cục vẫn dùng query).
  Sửa test chỉ test query param.
- **F23:** `CreateBatchRun` + `TriggerReEmbed` đổi Location về
  `/api/admin/v1/batch-runs/{id}` (khớp `RetryBatchRun`).
- **F21:** `streamRun` nhận flag `closeOnTerminal bool`; `StreamOps` truyền
  false. Sửa test "StreamOps closes on terminal kinds" đang đóng đinh hành vi
  sai.
- **F22-race:** `StreamBatchRun` đổi thứ tự: subscribe trước → check snapshot
  terminal (nếu terminal thì unsubscribe + 204) → stream.

**AC:** curl `/namespaces/foo/batch-runs` chỉ ra run của foo. Ops stream sống
qua nhiều run completion. Run complete đúng lúc subscribe không bị treo stream.

### 4.2 Cầu nối sự kiện cron → admin (hoặc hạ cấp docs) — **L**

**Vấn đề (F22):** observer chỉ gắn vào Job trong process admin; run của cron
vô hình với mọi SSE stream; comment `cmd/admin/main.go:83` sai sự thật.

**Cách sửa (đề xuất a):**
a. Bridge qua Redis pub/sub (mẫu có sẵn: events-tail `codohue:events-tail:*`):
   cron publish `batch_run.*`/`phase_*`/`log_line` lên
   `codohue:batchrun-events`; admin subscribe và bơm vào eventbus nội bộ.
b. Hoặc ngắn hạn: docs + UI nói rõ live stream chỉ áp dụng run manual; stream
   của run cron trả 204 ngay thay vì heartbeat rỗng vô hạn.

**AC (a):** mở `/batch-runs/{id}/stream` cho run cron đang chạy → nhận
`phase_started/completed`, `run_completed`, stream đóng đúng lúc.

### 4.3 Namespace delete/reset triệt để — **S**

**Vấn đề (F24):** `TruncateAllNamespaceData`/`ClearNamespaceData` bỏ sót
`catalog_backlog_samples`; race với cron xử lý ở task 2.4.

**Cách sửa:** thêm `DELETE FROM catalog_backlog_samples WHERE namespace=$1`
vào cả hai transaction; rà lại checklist mọi bảng có cột namespace (events,
objects, catalog_items, batch_run_logs, id_mappings, namespace_configs,
catalog_backlog_samples) + Redis keys + consumer group + 4 collection Qdrant.

**AC:** e2e wipe: sau delete, mọi bảng/keys/collection về 0; tạo lại cùng tên
→ backlog-history rỗng.

### 4.4 Dashboard ngừng bịa số liệu — **M**

**Vấn đề (F25):** `dashboard_service.go` hardcode `EmbedderHeartbeat{OK:true}`,
`Catalog{}`, `TrendingTTLSec: 0`.

**Cách sửa:** heartbeat embedder thật (đơn giản nhất: embedder SET key Redis
`codohue:embedder:heartbeat` TTL 90s mỗi 30s; admin GET); nối
`s.catalogBacklog` và TTL probe (đều đã có trong Service) vào
`GetNamespaceDashboard`/`GetOverview` per-ns. Nếu chưa kịp làm heartbeat thật,
trả `null`/`unknown` thay vì `true`.

**AC:** tắt embedder → overview báo embedder down trong ≤ 90s. Dashboard ns
có backlog/TTL khác 0 khi thực tế khác 0.

### 4.5 Re-embed: hiện thực `only_state` hoặc gỡ khỏi docs; sort/include_summary — **M**

**Vấn đề (F26, F-reembed-noop):** handler không decode body; re-embed cùng
strategy version reset 0 row → "thành công" no-op (use case rebuild-sau-mất-
Qdrant bất khả thi); `?sort=`/`?include_summary=` bị lờ.

**Cách sửa:** decode body `{"only_state":"all|embedded|failed"}` bằng
DecodeStrict; `SelectAndResetStaleCatalogItems` nhận filter state và bỏ điều
kiện `strategy_version <> target` khi `only_state="all"` (đây chính là đường
rebuild). Hiện thực `sort` (whitelist như `subjectOrderBy`) và
`include_summary`, hoặc xoá cả hai khỏi CLAUDE.md. Fix nhỏ kèm theo (F-watcher
race): `InsertReembedRun` + `SelectAndReset...` gộp một transaction để watcher
không đóng run vừa tạo.

**AC:** re-embed `only_state=all` với strategy version không đổi → toàn bộ
item được re-embed thật. Golden/handler test cho body mới.

### 4.6 Vá nhỏ admin — **S**

- **F-cancel-TOCTOU** `repository.go:320-340`: dùng CommandTag của UPDATE
  (`RowsAffected()==0` → 409), bỏ SELECT trước.
- **F-ILIKE** `catalog_ops_repository.go:380-382`: escape `%_\` như
  `escapeLikePrefix` đã có cho subjects.
- **F-pagination-ties**: thêm tiebreaker `, id DESC` vào ORDER BY của catalog
  items và events list.
- **F-eventbus-doubleclose** `eventbus/bus.go`: `Close()` đánh dấu closed +
  cancel của subscriber check flag (mutex chung) trước khi `close(s.ch)`.
- **F-proxy-escape** `service.go:390,589,740`: `url.PathEscape` cho `ns`,
  `subjectID` khi build URL proxy.

**AC:** unit test từng điểm; cancel run vừa complete trả 409.

---

## Phase 5 — Bảo mật

### 5.1 So sánh constant-time + rate limit login — **S**

**Vấn đề (F27, F30):** `admin/handler.go:100` và `auth/auth.go:37` so key bằng
`==`; login không rate-limit; mỗi token sai tốn 1 SELECT + bcrypt ~60ms không
giới hạn.

**Cách sửa:** `subtle.ConstantTimeCompare` (so hash SHA-256 hai vế để tránh
lộ độ dài); rate limit login theo IP (token bucket in-memory là đủ cho admin
plane); data plane: cache negative-auth ngắn (LRU 30s theo hash token) để chặn
spam bcrypt.

**AC:** test 401 vẫn đúng; benchmark: 100 request token sai không tạo 100
bcrypt compare.

### 5.2 Session: secret riêng, Secure cookie, logout thu hồi — **M**

**Vấn đề (F28):** JWT ký bằng chính admin API key; cookie thiếu `Secure`;
logout không thu hồi.

**Cách sửa:** secret HMAC random sinh lúc boot (env override
`CODOHUE_ADMIN_SESSION_SECRET` cho multi-replica; sinh mới mỗi boot đồng nghĩa
restart = logout all — chấp nhận được cho admin plane, ghi rõ); cookie
`Secure: true` khi request qua TLS (hoặc env `CODOHUE_ADMIN_INSECURE_COOKIE`
cho dev http); logout ghi `jti` vào denylist in-memory TTL = thời gian còn lại
của token.

**AC:** token cũ sau logout → 401; token không thể dùng brute-force offline ra
API key.

### 5.3 Xoay vòng API key namespace — **M**

**Vấn đề (F29):** `SetAPIKeyHash ... WHERE api_key_hash IS NULL` → không
rotate được key lộ; race tạo đồng thời trả plaintext chưa lưu.

**Cách sửa:** endpoint admin mới `POST /api/admin/v1/namespaces/{ns}/api-key`
(202/200, trả plaintext một lần) ghi đè hash vô điều kiện; `Upsert` check
`CommandTag.RowsAffected()` của `SetAPIKeyHash` — thua race thì **không** trả
plaintext (đọc lại row, trả response không kèm key).

**AC:** rotate xong key cũ 401, key mới 200; hai Upsert đồng thời chỉ một bên
nhận plaintext.

### 5.4 Quy tắc fallback global key khớp docs — **S**

**Vấn đề (F-auth-docs):** docs nói global key chỉ fallback khi ns chưa có key
riêng; code accept vô điều kiện.

**Cách sửa — ĐÃ CHỐT (2026-07-22):** làm theo docs — namespace đã có
`api_key_hash` thì global key bị **từ chối** trên data plane (giảm blast
radius khi admin key lộ). Sửa `auth/auth.go:36-39` để chỉ fallback khi
`api_key_hash IS NULL`. Thông báo thay đổi hành vi trong CHANGELOG; làm
**sau** 5.3 (rotate key) để operator lỡ mất key riêng còn đường cứu.

**AC:** ns có key riêng → global key 401 trên data plane; admin plane không
đổi.

---

## Phase 6 — Config, validation, contract trung thực

### 6.1 Validation nsconfig + chặn transition nguy hiểm — **M**

**Vấn đề (F33-occurred, F34, F-nsconfig-validate):**
- `PUT /namespaces/{ns}` flip được `dense_source='catalog'` không qua
  validation của `UpdateCatalogConfig` → namespace wedged.
- `embedding_dim`/`dense_distance` đổi tự do khi vector đã tồn tại → mọi
  upsert fail vĩnh viễn, không 400.
- alpha/gamma/lambda/trending/max_results không range-check;
  `json.Decode` thường thay vì DecodeStrict.

**Cách sửa:** trong `nsconfig.Service.Upsert`:
- `dense_source="catalog"` chỉ hợp lệ qua `UpdateCatalogConfig` (Upsert trả
  422 hướng dẫn dùng endpoint catalog) — hoặc Upsert tự chạy cùng validation.
- Đổi `embedding_dim` khi collection dense đã tồn tại → 409 kèm hướng dẫn
  (xoá collection / re-embed trước). Cần inject checker (đếm points) qua
  interface để giữ import rule.
- Range check: `0 <= alpha <= 1`, `lambda > 0`, `gamma >= 0`,
  `trending_ttl > 0`, `max_results > 0`, `seen_items_days > 0` → 400.
- `admin/handler.go:189` đổi sang `httpapi.DecodeStrict`.
- Kèm (F-disable-reset): `nsconfig/repository.go:299-306` disable catalog
  không reset `catalog_max_attempts`/`catalog_max_content_bytes` về default —
  giữ giá trị cũ (COALESCE như các cột khác).

**AC:** bảng test 400/409/422 cho từng field; gõ nhầm tên field → 400 thay vì
silent no-op.

### 6.2 Ingest validation: `occurred_at` — **S**

**Vấn đề (F33):** timestamp tương lai → `e^{+λ·days}` = Inf đầu độc vector;
omitted → năm 0001, event vô hình.

**Cách sửa:** trong `ingest.Service`: omitted/zero → default `time.Now()`;
tương lai quá 5 phút (skew cho phép) → 400 (HTTP) / drop + log (stream);
phòng thủ chiều sâu: `compute` clamp `daysSince = max(daysSince, 0)`.

**AC:** event tương lai bị chặn; event thiếu occurred_at vẫn tính vào cửa sổ
90 ngày; freshness không bao giờ > 1.

### 6.3 Nối hoặc gỡ các knob/field chết — **M**

**Vấn đề (F31, F32):**
- `CODOHUE_EMBED_MAX_ATTEMPTS`, `CODOHUE_CATALOG_MAX_CONTENT_BYTES` parse
  xong bỏ; compose dev/prod không truyền các biến retention.
- `EventPayload.Metadata` trong wire contract + SDK nhưng server vứt bỏ.

**Cách sửa:**
- Hai env trên trở thành default khi cột ns-level NULL: đổi
  `catalog_max_attempts`/`catalog_max_content_bytes` thành nullable
  (migration 023), code đọc `COALESCE(ns-level, env, hardcoded)` — đúng lời
  hứa "global default, per-ns override". Truyền env còn thiếu vào
  `docker-compose.yml` + `docker-compose.prod.yml` (thêm `env_file: .env` như
  app.yml).
- Metadata — **ĐÃ CHỐT (2026-07-22): gỡ khỏi wire contract.** Xoá field
  `Metadata` khỏi `pkg/codohuetypes/event.go`, khỏi SDK (`sdk/go` +
  `sdk/go/redistream`), regenerate golden snapshot (`go test
  ./pkg/codohuetypes/... -run Golden -update`), ghi CHANGELOG là breaking
  change có chủ đích. Lưu ý: sau khi gỡ, `DecodeStrict` sẽ **400** với client
  còn gửi `metadata` — đó là hành vi mong muốn (fail loudly thay vì âm thầm
  vứt bỏ); transport Redis Streams thì bỏ qua field lạ nên client cũ không
  vỡ đường stream.

**AC:** set env → thấy hiệu lực ở ns không override; golden snapshot của
`EventPayload` không còn `metadata`; HTTP ingest với body chứa `metadata` trả
400; test SDK cập nhật.

### 6.4 Vá vận hành nhỏ — **S**

- **F35:** `LoadCron` validate `BATCH_INTERVAL_MINUTES > 0` (0 = lỗi config
  rõ ràng, hoặc định nghĩa 0 = disable và skip ticker — chọn 1, docs theo).
- **F36-make:** sửa Makefile: `migrate create -ext sql -dir $(MIGRATIONS_DIR)
  -seq $(NAME)`.
- **F-embedder-shutdown:** `cmd/embedder/main.go:189-210` bỏ double-drain
  `workerDone` (nhận một lần, nhớ kết quả) — hết stall 10s giả.
- **F-gowork:** thêm `examples/geminipump`, `examples/loadgen` vào
  `GO_MODULES` trong Makefile (hoặc ghi chú rõ exclusion trong CLAUDE.md).
- **F-recommend-limit:** handler recommend/trending cap `limit`/`offset`
  (ví dụ limit ≤ 1000, offset ≤ 100000) → hết đường overflow.
- **F-buildSeenItemsFilter N+1:** thêm `GetOrCreateBatch` cho idmap (một
  INSERT ... UNNEST) — 5000 authored ids không còn là 5000 round-trip.
  (Kèm: các đường **đọc/xoá** dùng `Lookup` thay vì `GetOrCreate` để thôi
  ghi rác vào `id_mappings`.)
- **F-021-note:** ghi chú rolling-deploy vào `migrations/021` README (deploy
  binary trước, migrate sau); không cần sửa SQL vì đã ship.

---

## Thứ tự thực hiện & gom PR đề xuất

Mỗi PR một chủ đề, giữ diff review được; task trong cùng PR có phụ thuộc nhau.

| # | PR | Task | Ghi chú phụ thuộc |
|---|----|------|-------------------|
| 1 | `fix(idmap): composite key` | 1.1 | Chạy trước để các phase sau recompute một lần trên id đúng |
| 2 | `fix(ingest): PEL recovery + shutdown drain` | 1.2 | Độc lập |
| 3 | `feat(embedder): recovery sweep` | 1.3 | Nên đi sau 2.3 (idempotency) hoặc gộp chung |
| 4 | `fix(embedder): worker resilience + idempotency` | 2.2, 2.3 | Gộp được với PR 3 |
| 5 | `fix(compute): batch-run lifecycle` | 2.1, 2.4 | Advisory lock dùng chung cho cả delete-namespace |
| 6 | `fix(compute): svd/phase errors/lambda` | 2.5 | Độc lập |
| 7 | `fix(recommend): pagination + cache + rank` | 3.1–3.5 | Nhỏ, gom một PR |
| 8 | `feat(compute): stale point cleanup` | 3.6 | Sau PR 1 (dọn point mồ côi hậu migration) |
| 9 | `fix(recommend): freshness symmetry` | 3.7 | Đụng payload 3 producer — PR riêng |
| 10 | `fix(admin): scoping + SSE + wipe` | 4.1, 4.3, 4.6 | Độc lập |
| 11 | `feat(admin): cron event bridge` | 4.2 | Lớn nhất phía admin, làm sau 5 |
| 12 | `feat(admin): dashboard thật + re-embed only_state` | 4.4, 4.5 | Heartbeat cần thay đổi ở embedder |
| 13 | `fix(auth): hardening` | 5.1–5.4 | Một PR bảo mật |
| 14 | `feat(nsconfig): validation` | 6.1, 6.2 | Kèm test bảng 400/409 |
| 15 | `chore(config): dead knobs + ops fixes` | 6.3, 6.4 | Dọn dẹp cuối |

**Ước lượng tổng:** ~2.5–4 tuần một người. Phase 1 + PR 5 (~1 tuần đầu) loại
bỏ toàn bộ lớp mất-dữ-liệu và kẹt-vĩnh-viễn; phần còn lại hạ dần theo mức độ
lộ ra với người dùng.

## Nguyên tắc nghiệm thu chung

- Mỗi fix có test đóng đinh hành vi đúng (unit hoặc e2e theo tầng); các test
  hiện đang codify hành vi sai (StreamOps-closes, phase1-continues-on-total-
  failure) phải sửa cùng PR.
- Thay đổi wire contract (3.5, 6.3-metadata) đi qua quy trình golden snapshot:
  regenerate + commit có chủ đích, ghi CHANGELOG.
- Thay đổi hành vi bảo mật (5.4) và semantics env (6.4 interval=0) ghi rõ
  trong commit body.
- Sau khi xong Phase 1–2: chạy `make test-e2e-heavy` + soak docker compose
  với kill -9 ngẫu nhiên từng binary 30 phút — không còn row `running` mồ côi,
  không mất event, không item kẹt `pending`/`in_flight`.
