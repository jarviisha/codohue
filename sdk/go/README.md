# Codohue Go HTTP SDK

Go HTTP client for the [Codohue](../../README.md) recommendation engine.
Covers the data-plane HTTP endpoints: recommend, rank, trending, ingest, BYOE
embeddings, delete object, and health.

Admin endpoints (namespace config upsert) are intentionally **not** wrapped by
this SDK — those are operator-facing and live on a separate key tier.

## Install

```bash
go get github.com/jarviisha/codohue/sdk/go
```

Module path: `github.com/jarviisha/codohue/sdk/go`. Wire types shared with the
server live in `github.com/jarviisha/codohue/pkg/codohuetypes`.

This module targets Go `1.24.13`. The server application in the repo root
tracks Go `1.26.1` separately.

## Quick start

### HTTP client

```go
package main

import (
    "context"
    "log"
    "time"

    codohue "github.com/jarviisha/codohue/sdk/go"
    "github.com/jarviisha/codohue/pkg/codohuetypes"
)

func main() {
    c, err := codohue.New("http://localhost:2001",
        codohue.WithTimeout(5*time.Second),
        codohue.WithRetries(2),
    )
    if err != nil { log.Fatal(err) }

    ns := c.Namespace("feed", "your-namespace-api-key")
    ctx := context.Background()

    // Recommendations
    rec, err := ns.Recommend(ctx, "user-123", codohue.WithLimit(20))
    if err != nil { log.Fatal(err) }
    log.Printf("items: %v (source=%s)", rec.Items, rec.Source)

    // Rank candidates
    rk, _ := ns.Rank(ctx, "user-123", []string{"item-a", "item-b", "item-c"})
    for _, it := range rk.Items {
        log.Printf("%s: %.4f", it.ObjectID, it.Score)
    }

    // Trending
    tr, _ := ns.Trending(ctx,
        codohue.WithWindowHours(24),
        codohue.WithLimit(50),
    )
    _ = tr

    // HTTP ingest (single event). For bulk traffic prefer the Streams producer.
    _ = ns.IngestEvent(ctx, codohuetypes.EventPayload{
        SubjectID: "user-123",
        ObjectID:  "item-a",
        Action:    codohuetypes.ActionLike,
        Timestamp: time.Now().UTC(),
    })

    // BYOE embeddings
    _ = ns.StoreObjectEmbedding(ctx, "item-a", []float32{0.1, 0.2, 0.3 /* … */})
    _ = ns.StoreSubjectEmbedding(ctx, "user-123", []float32{ /* … */ })

    // Idempotent delete
    _ = ns.DeleteObject(ctx, "item-a")
}
```

### Redis Streams producer

For bulk event ingestion, use the separate Redis Streams producer module:

```bash
go get github.com/jarviisha/codohue/sdk/go/redistream
```

That module is documented separately and is the only SDK module that pulls
`github.com/redis/go-redis/v9`.

## Errors

Non-2xx responses become `*codohue.APIError`:

```go
rec, err := ns.Recommend(ctx, "user-1")
var apiErr *codohue.APIError
if errors.As(err, &apiErr) {
    log.Printf("status=%d code=%s: %s", apiErr.Status, apiErr.Code, apiErr.Message)
}
```

Sentinels (match with `errors.Is`):

| Sentinel | Triggers on |
|---|---|
| `ErrUnauthorized` | HTTP 401 |
| `ErrNotFound` | HTTP 404 |
| `ErrBadRequest` | any 4xx |
| `ErrDimMismatch` | code `embedding_dimension_mismatch` |
| `ErrDegraded` | `Healthz()` returned parsed degraded health on HTTP 503 |

`Healthz()` has one special case: when the server replies with a parseable
degraded health body, the SDK returns both the populated `*HealthStatus` and
an `*APIError` that matches `ErrDegraded`. This preserves the per-component
health details while still surfacing the degraded status as an error.

## Client options

| Option | Purpose |
|---|---|
| `WithHTTPClient(*http.Client)` | Inject a custom HTTP client (transport, mTLS, fakes) |
| `WithTimeout(time.Duration)` | Set timeout on the default HTTP client |
| `WithUserAgent(string)` | Override the `User-Agent` header |
| `WithRetries(n int)` | Retry idempotent GETs up to `n` times on 5xx / network errors (default: 2) |
| `WithRequestHook(func(*http.Request))` | Run just before each request — good for tracing headers |

Retries apply only to GET. Mutations (POST/PUT/DELETE) are never auto-retried.

## Authentication

Every namespace-scoped call sends `Authorization: Bearer <apiKey>`. Use the
per-namespace API key issued by your admin when the namespace was provisioned;
when a namespace has no per-namespace key, the global
`RECOMMENDER_API_KEY` works as a fallback.

## Development

This module lives inside the main Codohue repo under `sdk/go/` with its own
`go.mod`. A repo-root `go.work` wires it together with the root module for
local development:

```bash
# from repo root
go build ./...            # builds the server module
cd sdk/go && go test ./... # runs SDK tests
```

The `replace github.com/jarviisha/codohue/pkg/codohuetypes => ../../pkg/codohuetypes`
directive in `sdk/go/go.mod` resolves the shared wire-types module locally. It
is scoped to this module's own builds and does not affect downstream
consumers.
