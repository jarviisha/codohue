# Catalog — kiểm tra UI/UX và đề xuất tinh chỉnh

Ngày kiểm tra: 2026-09-30. Trạng thái: đã xác minh trên trình duyệt; CAT-01…06 và 7 đề xuất bổ sung đã triển khai (2026-09-30). Vùng chạm của bộ chọn khoảng thời gian dùng `size="lg"` của Astryx (28px) thay vì selector arbitrary, nên chưa đạt 44px.

## Phạm vi và phương pháp

- Trang: https://codohue.jarviisha.com/ns/bluesky/catalog, danh sách items và một trang chi tiết item.
- Công cụ: Playwright với Chromium, đăng nhập bằng tài khoản operator có quyền owner.
- Viewport: desktop 1440×1000, tablet 768×844, mobile 390×844. Mobile là kiểm tra viewport trên Chromium, chưa phải kiểm tra thiết bị cảm ứng thật.
- Đã thử: đăng nhập, đổi khoảng thời gian, mở/thu gọn dữ liệu biểu đồ, mở danh sách, tìm kiếm không có kết quả, xóa bộ lọc, mở chi tiết.
- Không thực hiện các thao tác thay đổi dữ liệu: Retry, Redrive, Re-embed, Delete. Chưa kiểm tra toàn diện bằng screen reader, dark mode hoặc đo contrast.
- Kết quả bên dưới lấy từ lượt chạy lại sau khi server ổn định. Độ trễ đăng nhập của lượt trước không được quy thành lỗi UI.

P1: chức năng hỏng hoặc thông tin vận hành gây hiểu nhầm. P2: cản trở thao tác hoặc khả năng truy cập. P3: tinh chỉnh trải nghiệm. Đây là mức ưu tiên sửa, không phải phân loại sự cố production.

## Tổng hợp

| ID | Mức | Vấn đề | Hướng xử lý |
| --- | --- | --- | --- |
| CAT-01 | P1 | Khoảng 7d trả HTTP 400 nhưng UI hiển thị biểu đồ trống | Chuẩn hóa duration, phân biệt lỗi và dữ liệu rỗng |
| CAT-02 | P1 | Nút Trigger re-embed bị cắt trên mobile | Cho nhóm hành động xuống dòng |
| CAT-03 | P2 | Header bảng chặn click thu gọn View chart data | Kiểm tra sticky header, stacking và vùng nhận click |
| CAT-04 | P2 | Không nhận biết khoảng thời gian đang chọn | Trạng thái chọn trực quan và ngữ nghĩa accessibility |
| CAT-05 | P2 | Bảng items khó đọc trên mobile | Ưu tiên thông tin, cải thiện ID dài và cuộn ngang |
| CAT-06 | P3 | Nhiều nút có vùng chạm nhỏ | Tăng vùng tương tác trên màn hình cảm ứng |

## Các vấn đề đã xác minh

### CAT-01 — Khoảng 7d bị lỗi và bị trình bày như không có dữ liệu

**Tái hiện:** mở Catalog, chọn `7d`, chờ request hoàn tất.

Request `GET /api/admin/v1/namespaces/bluesky/catalog/backlog-history?window=7d` trả HTTP 400:

```json
{"error":{"code":"invalid_request","message":"window: parse duration \"7d\": time: unknown unit \"d\" in duration \"7d\""}}
```

UI vẫn dựng biểu đồ với trục 0–4 và mô tả `0 recorded time points`, không thông báo lỗi. Khoảng `1h` và `24h` trả HTTP 200 và hiển thị dữ liệu trong lượt kiểm tra.

**Nguyên nhân phía frontend:** khi request lỗi, `history.data` là `undefined`, nên điều kiện `samples.length === 0` trong `CatalogStatusPage.tsx` sai và code rơi xuống nhánh vẽ biểu đồ với `[]`. Không có banner lỗi nào được render.

**Tác động:** người vận hành có thể hiểu sai rằng hệ thống không có lịch sử, trong khi request thất bại.

**Đề xuất:**

- Giữ nhãn `7d` nhưng gửi giá trị `168h`, phù hợp parser duration hiện tại; kiểm tra giới hạn cửa sổ phía server trước khi chốt sửa.
- Tách rõ các trạng thái loading, success có dữ liệu, success rỗng và error. Error có thông báo ngắn cùng nút `Try again`.
- Áp dụng cùng cách xử lý lỗi cho phần tổng hợp nguyên nhân thất bại.

**Khối lượng dữ liệu 7d:** sampler ghi khoảng 30 giây/mẫu, nên 7d là khoảng 20.160 mẫu. Toàn bộ đi vào Recharts và vào bảng trong `<details>` (bảng luôn nằm trong DOM kể cả khi đóng), và dữ liệu tải lại mỗi 60 giây. Phương án: thêm tham số `bucket` (đã được mô tả ở `ARCHITECTURE.md` nhưng handler chưa hỗ trợ) để server gộp mẫu, mỗi bucket giữ nguyên mẫu có tổng backlog lớn nhất (không lấy `MAX()` riêng từng series, vì biểu đồ xếp chồng sẽ cộng ra tổng không có thật), nên vẫn giữ được đỉnh backlog; 24h dùng bucket 5m, 7d dùng 30m (khoảng 288 và 336 điểm).

**Nghiệm thu:** cả ba khoảng tải thành công; 7d tương ứng 604800 giây; lỗi mạng/HTTP không trở thành trạng thái rỗng; thử lại tải đúng khoảng đã chọn.

**Mã liên quan:** [CatalogStatusPage.tsx](../web/admin/src/pages/ns/catalog/CatalogStatusPage.tsx), [catalog.ts](../web/admin/src/services/catalog.ts), [catalog_history_handler.go](../internal/admin/catalog_history_handler.go).

### CAT-02 — Nhóm hành động bị cắt trên mobile

**Tái hiện:** mở đầu trang Catalog ở viewport 390×844, khi có nút Redrive dead-letter.

Ba nút nằm trên cùng một hàng. `Trigger re-embed` bắt đầu tại x≈356px, rộng ≈132px và kết thúc tại x≈488px, vượt viewport 390px. [Ảnh bằng chứng](assets/catalog-ui-ux-audit-2026-09-30/mobile-top.png).

**Tác động:** nhãn và vùng bấm của hành động quan trọng không hiển thị đầy đủ. Chỉ kiểm tra `document.scrollWidth` là chưa đủ: container có thể cắt nội dung mà không làm toàn trang tràn ngang.

**Đề xuất:** bật wrap cho chính nhóm nút; ở màn hình hẹp dùng bố cục dọc hoặc lưới phù hợp, bảo đảm nhãn dài vẫn đọc được. Giữ thứ tự tab khớp thứ tự hiển thị.

**Nghiệm thu:** tại 320, 360, 390 và 768px, cả ba nút đều nhìn thấy và thao tác được; kiểm tra cả trường hợp có/không có dead-letter và nhãn đang xử lý.

**Mã liên quan:** nhóm hành động header trong [CatalogStatusPage.tsx](../web/admin/src/pages/ns/catalog/CatalogStatusPage.tsx).

### CAT-03 — Bảng dữ liệu chặn thao tác thu gọn

**Tái hiện đã quan sát:** chọn `7d`, mở `View chart data`, sau đó bấm lại để thu gọn ở desktop.

Playwright tái hiện timeout hai lần; log cho biết ô header `In flight` chặn pointer event tại điểm click của summary. Có thể thu gọn bằng focus rồi Enter. [Ảnh trạng thái mở](assets/catalog-ui-ux-audit-2026-09-30/chart-overlap.png); ảnh này bổ sung ngữ cảnh, bằng chứng chặn click đến từ log Playwright.

**Lưu ý:** `Table` của `@astryxdesign/core` không có sticky header (chỉ AppShell và ChatLayout dùng `position:sticky`), nên giả thuyết sticky header không đúng. Lượt tái hiện diễn ra khi 7d đang lỗi, tức bảng rỗng; nguyên nhân cần tìm lại sau khi CAT-01 được sửa, với bảng có dữ liệu thật.

**Nguyên nhân (đã xác minh khi sửa):** rule `containerBleed` của Astryx Table đặt `margin-top: calc(-1 * var(--container-padding-block-start))` khi scroll wrapper là `:first-child`. Bảng trong `<details>` thừa hưởng biến padding 12px của Card, nên bị kéo lên 12px và đè lên nửa dưới `<summary>`. Sửa bằng cách reset biến này về `0px` trên Stack bọc bảng.

**Đề xuất:** chỉ render bảng khi `<details>` đang mở (bỏ luôn hàng nghìn hàng khỏi DOM khi đóng); nếu vẫn bị chặn, dùng `elementFromPoint` tại vị trí summary để tìm phần tử đè lên rồi sửa đúng chỗ đó. Không khắc phục bằng cách ép click trong bài test.

**Nghiệm thu:** mở/đóng bằng chuột và Enter/Space đều được; header không che summary khi cuộn; kiểm tra bảng có dữ liệu, rỗng và sau lỗi tải. Nguyên nhân CSS chính xác cần xác minh trong bước sửa.

**Mã liên quan:** [TimeSeriesChart.tsx](../web/admin/src/components/charts/TimeSeriesChart.tsx) và component Table đang sử dụng.

### CAT-04 — Bộ chọn thời gian thiếu trạng thái chọn

**Tái hiện:** lần lượt chọn `1h`, `24h`, `7d`.

Cả ba nút dùng cùng kiểu hiển thị; không có `aria-pressed` hoặc `aria-selected` trong DOM đã kiểm tra. Người dùng không biết khoảng nào đang áp dụng. [Ảnh desktop](assets/catalog-ui-ux-audit-2026-09-30/desktop.png).

**Đề xuất:** dùng lại `SegmentedControl` (đã dùng ở `EventsPage.tsx`) với nhãn `Backlog time range`. Component này là radio group nên có sẵn trạng thái chọn cả về hiển thị lẫn ngữ nghĩa, không cần tự thêm `aria-pressed`. Có thể lưu lựa chọn vào query string để reload/chia sẻ giữ được ngữ cảnh.

**Nghiệm thu:** luôn có đúng một lựa chọn active; cập nhật ngay khi thao tác; trạng thái được đọc đúng bằng công nghệ hỗ trợ. Nếu thêm query string, Back/Forward và giá trị không hợp lệ phải có hành vi rõ ràng.

### CAT-05 — Bảng catalog items khó quét trên mobile

**Tái hiện:** chọn `Browse items`, đổi viewport sang 390×844.

Object ID dài bị chia thành nhiều dòng trong cột hẹp; vùng nhìn đầu tiên chỉ hiện Object, Author, State, các cột thao tác nằm ngoài vùng này. Đây là vấn đề khả dụng, chưa kết luận rằng các cột còn lại không thể truy cập. [Ảnh bằng chứng](assets/catalog-ui-ux-audit-2026-09-30/items-mobile.png).

**Đề xuất:**

- Ưu tiên dạng card trên mobile: Object, State, Updated; thông tin phụ và thao tác đặt ở hàng kế tiếp hoặc menu có nhãn.
- Nếu giữ bảng, quy định chiều rộng tối thiểu hợp lý và chỉ dẫn có thể cuộn ngang; tránh nén tất cả cột bằng nhau.
- Hiển thị ID rút gọn có chủ đích, cung cấp xem/copy đầy đủ; tên truy cập của link vẫn đủ để phân biệt item, không phụ thuộc tooltip hover.
- Giữ thao tác Delete tách biệt với mở chi tiết và Retry.

**Nghiệm thu:** đọc được trạng thái, mở chi tiết và truy cập hành động bằng touch/keyboard ở 390px; ID dài không làm hàng tăng chiều cao quá mức; desktop vẫn phục vụ việc so sánh nhiều item.

**Mã liên quan:** [CatalogItemsPage.tsx](../web/admin/src/pages/ns/catalog/CatalogItemsPage.tsx).

### CAT-06 — Vùng tương tác nhỏ trên màn hình cảm ứng

Nhiều nút, gồm bộ chọn thời gian và nhóm hành động Catalog, đo được chiều cao 28px; `Browse items` cao 32px. Đây là cơ hội cải thiện touch UX, không tự động đồng nghĩa vi phạm WCAG AA.

**Đề xuất:** đặt mục tiêu vùng chạm tối thiểu 44×44px cho thiết bị cảm ứng, có khoảng cách phù hợp; có thể tăng vùng tương tác mà không làm mọi nút desktop lớn hơn.

**Nghiệm thu:** vùng chạm không chồng lấn; focus ring rõ và không bị cắt; kiểm tra thêm trên thiết bị thật hoặc chế độ touch emulation.

## Đề xuất tinh chỉnh bổ sung

Các mục sau là đề xuất thiết kế, chưa phải lỗi chức năng đã tái hiện. Cả 7 mục đã được triển khai (2026-09-30):

- Mục 3: rút gọn còn 2 dòng, có "View full error" và nút Copy; không làm bảng ánh xạ lỗi sang câu dễ hiểu, vì danh sách pattern sẽ lỗi thời khi backend đổi message.
- Mục 4: Sample object dẫn tới danh sách items tìm theo object ID (`?q=`), không cần API lookup mới.
- Mục 6: dùng dialog xác nhận thường (nêu namespace, số item hoặc strategy đích), không yêu cầu gõ tên namespace.
- Mục 7: sửa trong `TimeSeriesChart` dùng chung: trục X theo thời gian thật, tự thêm ngày khi dữ liệu trải quá 24h, ghi timezone; biểu đồ Catalog dùng đường bậc thang (`stepAfter`).


1. **Đưa Browse items lên gần tiêu đề.** Đây là lối vào tác vụ thường dùng nhưng hiện nằm dưới biểu đồ và bảng lỗi; có thể giữ thêm liên kết cuối trang.
2. **Cho phép đi từ thống kê tới danh sách đã lọc.** Failed và Dead-letter nên dẫn tới trạng thái tương ứng, giữ ngữ cảnh namespace.
3. **Rút gọn thông báo kỹ thuật.** Hiển thị tóm tắt như “Không kết nối được kho vector”, cho mở/copy lỗi đầy đủ khi điều tra; tránh để chuỗi RPC dài chiếm phần lớn bảng.
4. **Liên kết Sample object tới chi tiết phù hợp.** Cần dùng lookup/ID đúng với route hiện tại, không ghép object URI trực tiếp vào route yêu cầu ID nội bộ.
5. **Giải thích chỉ số vận hành.** Bổ sung mô tả dễ hiểu cho XLEN, PEL, Failed và Dead-letter, có thể truy cập bằng keyboard/touch.
6. **Làm rõ tác động của hành động hàng loạt.** Đề xuất bước xác nhận cho Redrive/Re-embed, nêu namespace và phạm vi tác động. Chưa bấm các hành động này để kiểm tra hành vi trên production.
7. **Kiểm tra cách biểu diễn thời gian.** Với 24h/7d, thêm ngày và timezone khi cần; xác minh trục thời gian phản ánh khoảng cách mẫu và khoảng mất dữ liệu, tránh tạo cảm giác các mẫu luôn cách đều.

## Thứ tự triển khai và kiểm tra hồi quy

1. Sửa CAT-01 và CAT-02: khôi phục khoảng 7d, phản hồi lỗi rõ ràng, bảo đảm hành động không bị cắt.
2. Sửa CAT-03 và CAT-04: tương tác biểu đồ, trạng thái lựa chọn, keyboard.
3. Sửa CAT-05 và CAT-06; sau đó áp dụng tinh chỉnh bổ sung theo nhu cầu vận hành.

Kiểm tra Playwright cần bao gồm:

- Ba khoảng thời gian, response thành công, lỗi HTTP/network và retry; dữ liệu rỗng khác với lỗi.
- Mở/đóng bảng dữ liệu nhiều lần bằng chuột và keyboard, trước/sau khi cuộn.
- Responsive ở 320/360/390/768/1440px; kiểm tra bounding box và clipping của từng hành động, không chỉ document overflow.
- Tìm kiếm, xóa bộ lọc, lọc trạng thái, phân trang, mở chi tiết và quay lại giữ ngữ cảnh.
- Nếu sửa hành động ghi dữ liệu: dùng môi trường test hoặc mock, không chạy bulk operation trên namespace production để xác minh UI.

Khi triển khai, chạy các kiểm tra theo [AGENTS.md](../AGENTS.md) và [Makefile](../Makefile). Nếu thay đổi hợp đồng API, cập nhật [ARCHITECTURE.md](../ARCHITECTURE.md); chuyển nhãn `7d` sang tham số `168h` là thay đổi chỉ ở frontend; thêm `bucket` là thay đổi API admin nội bộ (không thuộc `pkg/codohuetypes`), cần cập nhật `ARCHITECTURE.md`.

## Những phần hoạt động tốt trong lượt kiểm tra

- Đăng nhập và tải dữ liệu Catalog thành công sau khi server ổn định.
- Biểu đồ 1h và 24h nhận dữ liệu thành công.
- Mở danh sách, tìm kiếm không có kết quả, xóa bộ lọc và mở một trang chi tiết hoạt động.
- Không ghi nhận exception JavaScript qua `pageerror` trong lượt kiểm tra responsive và danh sách. Điều này không phủ nhận lỗi HTTP 400 ở CAT-01.

Ảnh bằng chứng được lưu cùng tài liệu trong [assets/catalog-ui-ux-audit-2026-09-30](assets/catalog-ui-ux-audit-2026-09-30/). Không lưu mật khẩu, cookie hay session đăng nhập trong repository.
