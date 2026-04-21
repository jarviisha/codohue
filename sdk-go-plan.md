# Kế hoạch xây dựng Go SDK Client cho Codohue

Nhánh làm việc: `feat/sdk-go`

## 1. Quyết định đã chốt

| # | Chủ đề | Lựa chọn |
|---|---|---|
| 1 | Vị trí module | **(c)** Subdir cùng repo, `go.mod` riêng (`sdk/go/`) + `go.work` ở root cho local dev |
| 2 | Chia sẻ types | **(A)** Tạo `pkg/codohuetypes` làm single source of truth; refactor server handlers để dùng |
| 3 | Phạm vi SDK | Chỉ data-plane — **không** bao gồm admin API (`PUT /v1/config/namespaces/{ns}`) |
| 4 | Ingest | Hỗ trợ **cả HTTP và Redis Streams** (producer Redis ở subpackage riêng để tách dep) |

## 2. Phạm vi API cần bọc

| Nhóm | Endpoint | Auth | Module SDK |
|---|---|---|---|
| Ingest (HTTP) | `POST /v1/namespaces/{ns}/events` | namespace key | `sdk/go` |
| Ingest (Streams) | `XADD codohue:events * payload {json}` | — | `sdk/go/redistream` |
| Recommend | `GET /v1/namespaces/{ns}/recommendations` | namespace key | `sdk/go` |
| Rank | `POST /v1/namespaces/{ns}/rank` | namespace key | `sdk/go` |
| Trending | `GET /v1/namespaces/{ns}/trending` | namespace key | `sdk/go` |
| BYOE object | `POST /v1/namespaces/{ns}/objects/{id}/embedding` | namespace key | `sdk/go` |
| BYOE subject | `POST /v1/namespaces/{ns}/subjects/{id}/embedding` | namespace key | `sdk/go` |
| Delete object | `DELETE /v1/namespaces/{ns}/objects/{id}` | namespace key | `sdk/go` |
| Health | `GET /healthz`, `GET /ping` | none | `sdk/go` |

## 3. Contract Redis Streams

- Stream name: `codohue:events`
- Message field: `payload` (JSON của `EventPayload`)
- Consumer group đã được server tạo: `codohue-ingest`
- Producer chỉ cần `XADD` — không đụng consumer group

## 4. File layout

```
pkg/codohuetypes/               # NEW — module gốc github.com/jarviisha/codohue
  docs.go
  event.go                      # EventPayload, Action, action constants
  recommend.go                  # Response, RankRequest/Response, RankedItem,
                                # TrendingResponse/Item, EmbeddingRequest
  errors.go                     # ErrorDetail, ErrorResponse
                                # (dịch từ internal/core/httpapi)
  stream.go                     # const StreamName="codohue:events"
                                # const PayloadField="payload"

sdk/go/                         # NEW — module github.com/jarviisha/codohue/sdk/go
  go.mod
  README.md
  docs.go                       # Package codohue
  client.go                     # Client, New, base URL, HTTP client
  options.go                    # WithHTTPClient, WithTimeout, WithUserAgent,
                                # WithRetries, WithRequestHook
  errors.go                     # APIError + sentinel errors
  transport.go                  # do(), auth header, retry/backoff,
                                # ErrorResponse → APIError mapping
  namespace.go                  # type Namespace; c.Namespace(ns, key) *Namespace
  recommend.go                  # Recommend, Rank, Trending
  embedding.go                  # StoreObjectEmbedding, StoreSubjectEmbedding,
                                # DeleteObject
  ingest_http.go                # IngestEvent qua HTTP (namespace-scoped)
  health.go                     # Ping, Healthz
  *_test.go                     # httptest-based unit test

sdk/go/redistream/              # subpackage riêng — chỉ package này pull go-redis
  docs.go
  producer.go                   # Producer, NewProducer(redisClient, opts...),
                                # Publish(ctx, EventPayload), PublishBatch
  producer_test.go

go.work                         # NEW — use . và ./sdk/go cho local dev
```

## 5. Deps-leak check

- `pkg/codohuetypes` chỉ xài stdlib (`time`, `encoding/json`) → khi `sdk/go` import, **không** kéo theo `pgx/qdrant/redis` từ module gốc nhờ Go lazy module loading.
- `github.com/redis/go-redis/v9` **chỉ** là dep của `sdk/go/redistream`, user nào không dùng Stream producer sẽ không bị pull.

## 6. Refactor server (do chọn A)

| File | Thay đổi |
|---|---|
| `internal/ingest/types.go` | `EventPayload` → alias/re-export từ `codohuetypes`. Giữ domain `Event` (có `Weight`, `ID`) riêng. |
| `internal/recommend/types.go` | `Response`, `RankRequest/Response`, `RankedItem`, `TrendingResponse/Item`, `EmbeddingRequest` → dịch sang `codohuetypes`. File này còn lại `Request` (query params) + source constants. |
| `internal/core/httpapi/httpapi.go` | `ErrorDetail`, `ErrorResponse` → dịch sang `codohuetypes`. Giữ lại `WriteJSON`/`WriteError`. |
| `internal/nsconfig/types.go` | **Không đụng** — admin không thuộc SDK. |

Test hiện có sẽ cập nhật import, không đổi assertion. `make test` phải pass sau refactor.

## 7. API shape của SDK

```go
import (
    "github.com/jarviisha/codohue/pkg/codohuetypes"
    "github.com/jarviisha/codohue/sdk/go"
    "github.com/jarviisha/codohue/sdk/go/redistream"
)

// Khởi tạo client
c, _ := codohue.New("http://localhost:2001",
    codohue.WithTimeout(5*time.Second),
    codohue.WithRetries(2),
)

// Namespace-scoped wrapper
ns := c.Namespace("feed", "ns-api-key")

// Data plane
rec, err := ns.Recommend(ctx, "user-1", codohue.WithLimit(20))
rank, err := ns.Rank(ctx, codohuetypes.RankRequest{
    SubjectID:  "user-1",
    Candidates: []string{"a", "b"},
})
tr, err := ns.Trending(ctx,
    codohue.WithWindowHours(24),
    codohue.WithLimit(50),
)
err = ns.StoreObjectEmbedding(ctx, "item-1", []float32{...})
err = ns.StoreSubjectEmbedding(ctx, "user-1", []float32{...})
err = ns.DeleteObject(ctx, "item-1")
err = ns.IngestEvent(ctx, codohuetypes.EventPayload{...}) // qua HTTP

// Redis Streams producer (subpackage riêng)
p := redistream.NewProducer(redisClient)
err = p.Publish(ctx, codohuetypes.EventPayload{...})
err = p.PublishBatch(ctx, []codohuetypes.EventPayload{...})
```

### Errors

```go
var apiErr *codohue.APIError
if errors.As(err, &apiErr) {
    // apiErr.Status, apiErr.Code, apiErr.Message
}

if errors.Is(err, codohue.ErrUnauthorized) { ... }
```

Sentinel được định nghĩa:
- `ErrUnauthorized` — 401
- `ErrBadRequest` — 400 chung
- `ErrNotFound` — 404
- `ErrDimMismatch` — 400 code `embedding_dimension_mismatch`

### Options

- Client-level: `WithHTTPClient`, `WithTimeout`, `WithUserAgent`, `WithRetries(n)`, `WithRequestHook(func(*http.Request))` (cho tracing/metrics — không ép OpenTelemetry).
- Query-level: functional options cho các endpoint có query params (`WithLimit`, `WithOffset`, `WithWindowHours`).

### Resilience

- Retry tự động cho idempotent reads (GET) với jittered backoff, default **2 lần**.
- Ingest/Rank/Embedding/Delete: **không** auto-retry (an toàn hơn, user chủ động).
- Mọi method nhận `context.Context` — tôn trọng cancel/deadline.

## 8. Thứ tự thực hiện

| Bước | Mô tả | Tiêu chí hoàn thành |
|---|---|---|
| 1 | Tạo `pkg/codohuetypes` + move types + cập nhật server handlers/tests | `make test` pass, `make lint` pass |
| 2 | Tạo `go.work` và `sdk/go/go.mod` | `go build ./...` từ root và từ `sdk/go` đều OK |
| 3 | `sdk/go`: client + options + errors + transport + endpoint `Recommend` + test | `httptest` round-trip xanh |
| 4 | Bổ sung Rank, Trending, IngestEvent HTTP, BYOE, DeleteObject, Health | Mỗi endpoint có unit test |
| 5 | `sdk/go/redistream`: Producer + test | Test publish → assert XADD đúng format |
| 6 | `sdk/go/README.md` + example ngắn | README đủ để user onboard |

## 9. Testing

- Unit: `httptest.Server` cho mỗi endpoint — assert method, path, auth header, request body, response mapping.
- Stream producer: mock `XAdder` interface hoặc miniredis.
- E2E (optional): tái dùng `make up-infra`, test client nói chuyện với API thật.

## 10. Release

- Tag root module: `v0.x.y` (nếu có thay đổi ở `pkg/codohuetypes`).
- Tag SDK module: `sdk/go/v0.x.y` (convention Go multi-module).
- Version đầu tiên dự kiến: `sdk/go/v0.1.0`.
