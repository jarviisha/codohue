# Feature Specification: Admin Pipeline Controls

**Feature Branch**: `feat/web-admin-dashboard`  
**Created**: 2026-05-03  
**Status**: Draft  
**Input**: Thêm 3 tính năng vào web/admin để admin có thể kiểm soát pipeline khuyến nghị: trigger batch thủ công, xem events gần đây, và inject test events.

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Trigger Batch Run Thủ Công (Priority: P1)

Người admin cần test nhanh pipeline sau khi thay đổi config namespace hoặc inject dữ liệu test. Hiện tại phải chờ cron tự chạy (mặc định 5 phút), làm chậm feedback loop.

Admin vào trang Namespaces hoặc Batch Runs, nhấn nút "Run now" cho một namespace, hệ thống khởi động batch job ngay lập tức và trả về kết quả khi xong.

**Why this priority**: Không có tính năng này, vòng lặp test pipeline (inject → batch → kiểm tra recommend) mất ít nhất 5 phút mỗi iteration, làm chậm development và debugging đáng kể.

**Independent Test**: Có thể test độc lập bằng cách nhấn "Run now" trên một namespace có sẵn và xác nhận batch run mới xuất hiện trong lịch sử Batch Runs sau khi hoàn thành.

**Acceptance Scenarios**:

1. **Given** namespace tồn tại và không có batch đang chạy, **When** admin nhấn "Run now", **Then** hệ thống bắt đầu batch job và hiển thị trạng thái đang chạy
2. **Given** batch đang được trigger, **When** hoàn thành, **Then** kết quả xuất hiện trong danh sách Batch Runs với đầy đủ phase breakdown
3. **Given** batch đang chạy trên namespace, **When** admin cố trigger thêm lần nữa, **Then** hệ thống từ chối và hiển thị thông báo rõ ràng
4. **Given** namespace không tồn tại hoặc lỗi khởi động, **When** trigger, **Then** hệ thống trả về lỗi mô tả rõ nguyên nhân

---

### User Story 2 — Xem Events Gần Đây (Priority: P2)

Admin muốn xác nhận events có đang vào database không, và kiểm tra distribution của action types (VIEW, LIKE, SKIP…) trước khi trigger batch.

Admin vào trang của một namespace, xem danh sách events gần nhất với: subject_id, object_id, action, weight, và thời gian xảy ra.

**Why this priority**: Khi recommend không ra kết quả, không biết nguyên nhân là do không có events hay do pipeline lỗi. Xem events giúp isolate vấn đề ngay lập tức.

**Independent Test**: Có thể test bằng cách inject một event qua API rồi kiểm tra nó xuất hiện trong danh sách events của namespace đó.

**Acceptance Scenarios**:

1. **Given** namespace có events trong DB, **When** admin mở trang events, **Then** hiển thị danh sách events theo thứ tự mới nhất trước
2. **Given** danh sách events, **When** admin xem, **Then** mỗi event hiển thị: subject_id, object_id, action, weight, occurred_at
3. **Given** namespace chưa có events, **When** mở trang, **Then** hiển thị trạng thái empty rõ ràng
4. **Given** namespace có nhiều events, **When** admin scroll xuống, **Then** tải thêm (phân trang hoặc infinite scroll), mặc định 50 events/trang
5. **Given** danh sách events, **When** admin lọc theo subject_id cụ thể, **Then** chỉ hiển thị events của subject đó

---

### User Story 3 — Inject Test Event Từ UI (Priority: P3)

Trong quá trình phát triển và debug, admin cần tạo interaction giả (ví dụ: user-1 đã VIEW item-42) mà không cần viết curl command hay script bên ngoài.

Admin vào trang Events của namespace, điền form: subject_id, object_id, action type, sau đó submit. Event được ghi vào database và hiển thị ngay trong danh sách.

**Why this priority**: Tiện ích cao cho development nhưng không blocking — admin vẫn có thể dùng curl hoặc main API trực tiếp để inject events. Đây là quality-of-life improvement.

**Independent Test**: Điền form inject event với subject_id="test-user-1", object_id="test-item-1", action="VIEW", submit và xác nhận event xuất hiện trong danh sách events của namespace.

**Acceptance Scenarios**:

1. **Given** form inject event với đầy đủ fields, **When** admin submit, **Then** event được ghi vào DB và xuất hiện đầu danh sách
2. **Given** form với action không hợp lệ, **When** submit, **Then** validation bắt lỗi trước khi gửi
3. **Given** form với namespace không có API key, **When** submit, **Then** admin server proxy sử dụng global API key
4. **Given** inject thành công, **When** admin trigger batch ngay sau đó, **Then** event vừa inject được tính vào vector computation

---

### Edge Cases

- Điều gì xảy ra nếu batch trigger nhưng Qdrant không khả dụng? → batch vẫn chạy Phase 1, Phase 2 fail gracefully, log error vào batch_run_logs
- Điều gì xảy ra nếu namespace có 0 subjects khi trigger batch? → batch chạy thành công với 0 vectors processed (không phải lỗi)
- Events list với namespace rất lớn (hàng triệu events)? → phân trang server-side, không load toàn bộ
- Inject event với object_id chưa tồn tại? → cho phép, ID mapping sẽ được tạo tự động khi batch chạy
- Concurrent batch triggers cho cùng namespace? → hệ thống chặn trigger thứ 2 trong khi cái đầu còn chạy

## Requirements *(mandatory)*

### Functional Requirements

**Batch Trigger**

- **FR-001**: Hệ thống PHẢI cung cấp endpoint để trigger batch run cho một namespace cụ thể theo yêu cầu
- **FR-002**: Hệ thống PHẢI chạy đủ 3 phases (Sparse, Dense, Trending) như cron job thông thường
- **FR-003**: Hệ thống PHẢI ghi kết quả vào `batch_run_logs` với đầy đủ phase breakdown giống cron-triggered runs
- **FR-004**: Hệ thống PHẢI ngăn chặn concurrent batch runs trên cùng một namespace
- **FR-005**: Admin UI PHẢI hiển thị nút "Run now" trên trang Namespaces và/hoặc trang chi tiết namespace
- **FR-006**: Admin UI PHẢI cập nhật hiển thị khi batch hoàn thành (polling hoặc refetch)

**Events Listing**

- **FR-007**: Hệ thống PHẢI cung cấp endpoint trả về danh sách events theo namespace, sắp xếp theo `occurred_at` giảm dần
- **FR-008**: Endpoint PHẢI hỗ trợ phân trang với tham số `limit` (mặc định 50, tối đa 200) và `offset`
- **FR-009**: Endpoint PHẢI hỗ trợ lọc tùy chọn theo `subject_id`
- **FR-010**: Mỗi event trong response PHẢI bao gồm: id, subject_id, object_id, action, weight, occurred_at
- **FR-011**: Admin UI PHẢI thêm tab hoặc trang "Events" trong navigation hoặc trong trang chi tiết namespace

**Event Injection**

- **FR-012**: Admin UI PHẢI cung cấp form để tạo event với các fields: subject_id, object_id, action type, (tùy chọn) occurred_at
- **FR-013**: Form PHẢI validate action type là một trong các giá trị hợp lệ của namespace (hoặc dùng danh sách mặc định: VIEW, LIKE, COMMENT, SHARE, SKIP)
- **FR-014**: Admin server PHẢI proxy request inject event đến main API (`POST /v1/namespaces/{ns}/events`) với authentication phù hợp
- **FR-015**: Sau khi inject thành công, danh sách events PHẢI tự động refresh để hiện event mới

### Key Entities

- **BatchTriggerRequest**: namespace (required)
- **BatchTriggerResponse**: batch_run_id, namespace, started_at, status
- **EventSummary**: id, namespace, subject_id, object_id, action, weight, occurred_at
- **EventsListResponse**: events ([]EventSummary), total, limit, offset
- **InjectEventRequest**: namespace, subject_id, object_id, action, (optional) occurred_at

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Admin có thể hoàn thành vòng lặp "inject event → trigger batch → kiểm tra recommend" trong dưới 2 phút (thay vì 5+ phút với cron)
- **SC-002**: Batch trigger hoàn thành trong thời gian tương đương cron-triggered batch cho cùng namespace (không có overhead đáng kể)
- **SC-003**: Events list tải và hiển thị trong dưới 1 giây cho namespace có đến 100,000 events (nhờ phân trang server-side)
- **SC-004**: Admin có thể xác định nguyên nhân "recommend không ra kết quả" trong dưới 30 giây bằng cách nhìn vào events list và batch history
- **SC-005**: Form inject event không yêu cầu admin biết bất kỳ thông tin xác thực nào ngoài session đang đăng nhập

## Assumptions

- Admin đã đăng nhập vào dashboard (session cookie hợp lệ) — authentication không thay đổi
- Batch trigger chạy synchronously theo request–response: admin chờ response, không cần polling mechanism (nếu batch chạy quá lâu sẽ xem xét async sau)
- Danh sách action types hợp lệ lấy từ `action_weights` của namespace config (VIEW, LIKE, COMMENT, SHARE, SKIP là mặc định)
- Event injection dùng thời điểm server hiện tại cho `occurred_at` nếu không được chỉ định
- Mobile UI không trong scope — admin dashboard dành cho desktop/laptop
- Không cần export CSV hay bulk operations trong version này
- Backend Go codebase đã có `internal/admin/` layer với handler/service/repository pattern — tính năng mới follow pattern này
- Cron job và admin-triggered batch dùng cùng code path (`internal/compute/job.go`) để đảm bảo consistency
