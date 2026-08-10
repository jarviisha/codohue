# Báo cáo audit Codohue - 2026-08-06

## 1. Phạm vi và kết luận

Đợt audit này tập trung vào các luồng có rủi ro cao của Codohue:

- cách ly multi-tenant và xác thực;
- recommendation cache và tính đúng đắn của kết quả;
- catalog auto-embedding và vòng đời re-embed;
- batch compute, reset và xóa namespace;
- ingest validation và tính toàn vẹn dữ liệu;
- giới hạn tài nguyên HTTP;
- tài liệu, SDK và công cụ vận hành.

Không phát hiện lỗi biên dịch hay test failure trên các luồng thông thường. Tuy
nhiên, có **4 vấn đề mức High**, **5 vấn đề mức Medium** và một nhóm sai lệch
giữa contract với tài liệu. Các lỗi High cần được xử lý trước khi coi hệ thống
đủ an toàn để vận hành production.

## 2. Tóm tắt mức độ ưu tiên

| ID | Mức độ | Vấn đề | Ảnh hưởng chính |
|---|---|---|---|
| F-01 | High | Recommendation cache có thể va chạm giữa namespace | Lộ dữ liệu recommendation chéo tenant |
| F-02 | High | Re-embed có thể báo thành công khi item vẫn pending | Trạng thái vận hành sai, rebuild Qdrant không hoàn tất |
| F-03 | High | Xóa namespace mất khả năng retry sau lỗi partial | Dữ liệu Redis/Qdrant bị bỏ lại vĩnh viễn |
| F-04 | High | Reset app không khóa compute đang chạy | Dữ liệu/collection bị tái tạo sau reset |
| F-05 | Medium | Ingest fallback khi đọc config lỗi | Event sai weight, chấp nhận namespace ma |
| F-06 | Medium | Dense phase lỗi nhưng batch vẫn success | Dashboard báo xanh sai sự thật |
| F-07 | Medium | Xóa catalog item nuốt lỗi Qdrant | Dense vector mồ côi vẫn được recommend |
| F-08 | Medium | Login limiter có thể khóa toàn bộ admin | Từ chối dịch vụ trên admin plane |
| F-09 | Medium | JSON request body không có hard limit | Cạn bộ nhớ/CPU |
| F-10 | Low | README, SDK docs và eval script dùng contract cũ | Quickstart và tooling chạy sai |

### Trạng thái remediation

Các finding dưới đây đã được xử lý trên nhánh `fix/audit-remediation`:

| ID | Trạng thái | Thay đổi chính |
|---|---|---|
| F-01 | Đã xử lý | Cache key dùng Base64URL cho từng thành phần và cache hit xác minh lại namespace/subject |
| F-02 | Đã xử lý | Item được re-drive bị xóa version cũ; watcher dùng target version đã lưu trên batch run |
| F-03 | Đã xử lý | Delete vẫn chạy cleanup khi config đã mất, cho phép retry sau partial failure |
| F-04 | Đã xử lý | Compute/delete giữ shared maintenance lock; reset giữ exclusive lock đến khi cleanup hoàn tất |
| F-05 | Đã xử lý | Lỗi config được trả về để retry; namespace không tồn tại bị reject thay vì dùng default weight |
| F-06 | Đã xử lý | Dense phase lỗi làm aggregate run thất bại nhưng không ngăn trending phase chạy |
| F-07 | Đã xử lý | Qdrant được xóa trước PostgreSQL; lỗi external cleanup được trả về để retry |
| F-08 | Đã xử lý | Credential đúng bỏ qua failed-attempt bucket, tránh lockout qua IP proxy dùng chung |
| F-09 | Đã xử lý | Strict JSON decoder có hard cap 8 MiB và kiểm tra trailing payload đầy đủ |
| F-10 | Đã xử lý | README, eval script và Redis Streams SDK docs đã dùng contract hiện hành |

## 3. Findings chi tiết

### F-01 - Recommendation cache có thể va chạm giữa namespace

**Mức độ:** High
**Thành phần:** `internal/recommend`, `internal/nsconfig`

Cache key được ghép trực tiếp bằng dấu `:`:

```text
rec:{namespace}:{subject_id}:limit={limit}:offset={offset}
```

Trong khi đó, server không validate bộ ký tự của namespace. Ví dụ hai request:

```text
namespace="a",   subject_id="b:c"
namespace="a:b", subject_id="c"
```

đều tạo key:

```text
rec:a:b:c:limit=20:offset=0
```

Cache hit được deserialize và trả thẳng, không đối chiếu lại `Namespace` và
`SubjectID` trong response. Tenant thứ hai vì vậy có thể nhận recommendation đã
cache của tenant thứ nhất.

**Vị trí:**

- [`internal/recommend/service.go:291`](internal/recommend/service.go#L291)
- [`internal/recommend/service.go:1453`](internal/recommend/service.go#L1453)
- [`internal/nsconfig/service.go:72`](internal/nsconfig/service.go#L72)

**Khuyến nghị:**

1. Validate namespace theo một slug format rõ ràng, ví dụ `^[a-z0-9][a-z0-9_-]{0,62}$`.
2. Dùng cache key có encoding không mơ hồ, ví dụ hash `(namespace, subject_id)`
   hoặc length-prefix từng thành phần.
3. Khi đọc cache, xác minh response namespace/subject khớp request trước khi trả.
4. Thêm test với namespace và subject ID chứa dấu `:`.

### F-02 - Re-embed có thể đóng run quá sớm

**Mức độ:** High
**Thành phần:** `internal/admin`, `internal/embedder`

Khi operator gọi re-embed với `only_state=all`, `embedded` hoặc `failed`, query
reset item về `pending` nhưng giữ nguyên `strategy_version`. Completion watcher
chỉ đếm `pending|in_flight|failed` nếu version của item khác version hiện tại.

Với một same-version rebuild, watcher có thể thấy `stale=0` ngay sau reset và
đánh dấu batch run thành công trước khi worker xử lý bất kỳ item nào. Đây chính
là đường rebuild sau khi mất dữ liệu Qdrant, nên false-success có thể để
collection rỗng trong khi dashboard báo xanh.

Ngoài ra, production watcher join với `namespace_configs` hiện tại thay vì dùng
`target_strategy_id` và `target_strategy_version` đã đóng băng trên batch run.
Nếu config đổi giữa run, watcher sẽ theo dõi sai target.

**Vị trí:**

- [`internal/admin/catalog_ops_repository.go:188`](internal/admin/catalog_ops_repository.go#L188)
- [`internal/admin/catalog_ops_repository.go:200`](internal/admin/catalog_ops_repository.go#L200)
- [`internal/embedder/reembed_watcher.go:200`](internal/embedder/reembed_watcher.go#L200)
- [`internal/embedder/reembed_watcher.go:221`](internal/embedder/reembed_watcher.go#L221)

**Khuyến nghị:**

1. Cho watcher đọc target version từ chính row trong `batch_run_logs`.
2. Theo dõi tập item thuộc run, không suy diễn completion chỉ từ version hiện tại.
3. Tối thiểu, mọi item `pending|in_flight|failed` được reset bởi run phải được
   tính là outstanding bất kể version.
4. Thêm E2E cho `only_state=all` với target version không đổi và Qdrant collection rỗng.

### F-03 - Xóa namespace mất khả năng retry sau partial failure

**Mức độ:** High
**Thành phần:** `internal/admin`

`clearNamespaceEverywhere` xóa PostgreSQL trước, bao gồm `namespace_configs`,
sau đó mới xóa Redis và Qdrant. Nếu Redis hoặc Qdrant lỗi, endpoint trả 500 nhưng
namespace config đã biến mất. Lần retry sau dừng ngay ở pre-check và trả 404,
không tiếp tục dọn dữ liệu còn sót.

**Kịch bản:**

1. Operator gọi xóa namespace.
2. PostgreSQL transaction commit thành công.
3. Qdrant tạm thời unavailable.
4. Endpoint trả 500; collection vẫn còn.
5. Operator retry; endpoint trả 404 vì config đã bị xóa.

**Vị trí:**

- [`internal/admin/service.go:880`](internal/admin/service.go#L880)
- [`internal/admin/service.go:1018`](internal/admin/service.go#L1018)
- [`internal/admin/service.go:1059`](internal/admin/service.go#L1059)

**Khuyến nghị:**

- Dùng tombstone/deletion state và quy trình cleanup có thể retry; hoặc
- cho phép delete endpoint cleanup Redis/Qdrant kể cả khi config không còn; và
- ghi durable cleanup job/outbox trước khi xóa owner row trong PostgreSQL.

### F-04 - Reset app không đồng bộ với compute đang chạy

**Mức độ:** High
**Thành phần:** `internal/admin`, `internal/compute`

`ResetApp` truncate các bảng và xóa Redis/Qdrant mà không lấy compute advisory
lock. Cron hoặc manual run đang chạy có thể tiếp tục upsert vector/collection sau
khi reset hoàn tất. Batch row của run cũng đã bị truncate, nên finalize sau đó
không còn nơi để ghi trạng thái.

**Vị trí:**

- [`internal/admin/service.go:915`](internal/admin/service.go#L915)
- [`internal/admin/service.go:927`](internal/admin/service.go#L927)
- [`internal/compute/job.go:219`](internal/compute/job.go#L219)

**Khuyến nghị:** thêm global maintenance advisory lock mà mọi compute run phải
giữ shared/exclusive theo quy ước, hoặc lấy toàn bộ namespace locks trước reset
và chỉ truncate sau khi các run đã drain.

### F-05 - Ingest nuốt lỗi namespace config

**Mức độ:** Medium
**Thành phần:** `internal/ingest`

`resolveWeight` fallback sang default action weights khi:

- query namespace config gặp lỗi hạ tầng; hoặc
- namespace không tồn tại.

Hệ quả là event có thể được ghi với weight sai so với config của tenant. Redis
Streams producer cũng có thể tạo event cho namespace ma vì bảng `events` không
có foreign key tới `namespace_configs`. Unit test hiện đang đóng đinh hành vi
fallback khi DB lỗi.

**Vị trí:**

- [`internal/ingest/service.go:110`](internal/ingest/service.go#L110)
- [`internal/ingest/service_test.go:123`](internal/ingest/service_test.go#L123)
- [`migrations/001_initial.up.sql:10`](migrations/001_initial.up.sql#L10)

**Khuyến nghị:** phân biệt rõ ba trạng thái `config found`, `namespace missing`
và `infrastructure error`. Chỉ dùng default weights khi namespace tồn tại nhưng
không override action đó; hai trường hợp còn lại phải reject/retry.

### F-06 - Dense phase lỗi nhưng batch vẫn success

**Mức độ:** Medium
**Thành phần:** `internal/compute`, admin dashboard

Phase 2 failure được ghi vào phase result nhưng không fold vào `runErr`. Nếu
phase 1 và phase 3 thành công, `batch_run_logs.success` vẫn là `true`. Điều này
mâu thuẫn với tài liệu nói rằng all-green nghĩa là mọi phase đã chạy đều thành công.

**Vị trí:**

- [`internal/compute/job.go:334`](internal/compute/job.go#L334)
- [`internal/compute/job.go:369`](internal/compute/job.go#L369)
- [`ARCHITECTURE.md:205`](ARCHITECTURE.md#L205)

**Khuyến nghị:** vẫn cho phase 3 tiếp tục sau phase 2 failure, nhưng aggregate
run phải `success=false`; hoặc thêm trạng thái `degraded` riêng và hiển thị rõ
trên API/UI.

### F-07 - Xóa catalog item để lại vector mồ côi

**Mức độ:** Medium
**Thành phần:** `internal/admin`

Catalog row bị xóa khỏi PostgreSQL trước khi numeric ID được lookup và Qdrant
point được xóa. Lỗi lookup/Qdrant chỉ được log; endpoint vẫn trả 204. Khi retry,
row không còn nên code return ngay và không thể phục hồi `object_id` để cleanup.

**Vị trí:**

- [`internal/admin/catalog_ops_service.go:355`](internal/admin/catalog_ops_service.go#L355)
- [`internal/admin/catalog_ops_service.go:360`](internal/admin/catalog_ops_service.go#L360)
- [`internal/admin/catalog_ops_service.go:370`](internal/admin/catalog_ops_service.go#L370)
- [`internal/admin/catalog_ops_service.go:382`](internal/admin/catalog_ops_service.go#L382)

**Khuyến nghị:** resolve numeric ID trước, xóa Qdrant trước khi xóa row, hoặc ghi
durable cleanup task. Không trả success nếu cleanup bắt buộc chưa thành công.

### F-08 - Login rate limiter có thể khóa toàn bộ admin plane

**Mức độ:** Medium
**Thành phần:** `internal/admin`

Limiter được kiểm tra trước khi credential được validate. Sau khi bucket cạn,
credential đúng cũng bị trả 429. `clientIP` chỉ dùng `RemoteAddr`; khi chạy sau
reverse proxy, mọi operator thường cùng chia sẻ IP của proxy. Năm lần login sai
có thể khóa tất cả operator cho tới khi bucket refill.

**Vị trí:**

- [`internal/admin/handler.go:104`](internal/admin/handler.go#L104)
- [`internal/admin/session.go:189`](internal/admin/session.go#L189)
- [`internal/admin/session.go:233`](internal/admin/session.go#L233)

**Khuyến nghị:** chỉ tin forwarded headers từ danh sách trusted proxies, thêm
global + per-principal limiter, và cho credential đúng vượt qua failed-attempt
bucket hoặc dùng cooldown ngắn có bounded exponential delay.

### F-09 - JSON request body không có giới hạn kích thước

**Mức độ:** Medium
**Thành phần:** HTTP data plane và admin plane

`DecodeStrict` đọc JSON trực tiếp từ body mà không qua `http.MaxBytesReader`.
Catalog content limit chỉ được kiểm tra sau khi string đã được decode và cấp
phát. Embedding vector cũng có thể làm server cấp phát một slice rất lớn trước
khi dimension validation chạy.

**Vị trí:**

- [`internal/core/httpapi/httpapi.go:26`](internal/core/httpapi/httpapi.go#L26)
- [`internal/catalog/handler.go:57`](internal/catalog/handler.go#L57)
- [`internal/catalog/service.go:112`](internal/catalog/service.go#L112)
- [`internal/recommend/handler.go:211`](internal/recommend/handler.go#L211)

**Khuyến nghị:** đặt route-specific body caps trước decode. Catalog batch cap
cần tính theo `max_items * max_content_bytes` cộng metadata overhead; embedding
cap theo configured dimension cộng một margin nhỏ.

### F-10 - Contract và tài liệu bị sai lệch

**Mức độ:** Low
**Thành phần:** README, SDK docs, evaluation tooling

Các ví dụ vẫn dùng contract trước dense-source unification:

- README gửi `dense_strategy`, trong khi strict decoder chỉ nhận `dense_source`;
- README vẫn mô tả `catalog_enabled=true`;
- `scripts/eval.py` gửi `dense_strategy`, làm `make eval` fail với HTTP 400;
- ví dụ Redis dùng `timestamp` thay cho `occurred_at`, nên timestamp bị bỏ qua
  và ingest thay bằng thời gian hiện tại;
- Redis SDK README khởi tạo field `Timestamp`, field này không tồn tại trên
  `codohuetypes.EventPayload`.

**Vị trí:**

- [`README.md:111`](README.md#L111)
- [`README.md:129`](README.md#L129)
- [`README.md:201`](README.md#L201)
- [`scripts/eval.py:537`](scripts/eval.py#L537)
- [`scripts/eval.py:608`](scripts/eval.py#L608)
- [`sdk/go/redistream/README.md:42`](sdk/go/redistream/README.md#L42)

## 4. Khoảng trống của test suite

Test suite hiện tại chưa bao phủ các fault/race scenario sau:

- cache key collision với namespace/subject chứa delimiter;
- re-embed `only_state=all` ở cùng strategy version;
- strategy config đổi trong khi re-embed run đang mở;
- Redis/Qdrant failure sau khi PostgreSQL delete đã commit;
- reset trong lúc cron/manual compute đang upsert;
- namespace config lookup fail trong ingest;
- phase 2 fail nhưng phase 3 thành công;
- login brute-force khi admin nằm sau reverse proxy;
- oversized authenticated JSON body.

## 5. Kết quả xác minh

Tại thời điểm audit:

| Check | Kết quả |
|---|---|
| `make test` | Pass |
| `make test-race` | Pass |
| `make test-e2e` | Pass |
| `make lint` | Pass, 0 issues |
| `make web-admin-lint` | Pass |
| `make web-admin-test` | Pass |
| `make compose-check` | Pass |
| Migration version | 23 |
| Git working tree trước report | Clean |

E2E pass xác nhận happy paths hoạt động, nhưng không phủ định các findings trên
vì chúng nằm ở fault-injection, same-version rebuild, cross-tenant key encoding
và concurrent lifecycle paths.

## 6. Thứ tự remediation đề xuất

1. **Tenant isolation:** F-01.
2. **Lifecycle truthfulness:** F-02, F-04, F-06.
3. **Retryable deletion:** F-03, F-07.
4. **Data integrity:** F-05.
5. **Security/resource hardening:** F-08, F-09.
6. **Contract cleanup:** F-10.
