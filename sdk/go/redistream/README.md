# Codohue Go Redis Streams Producer

Go producer for publishing Codohue behavioral events into Redis Streams for
high-throughput ingestion.

Module path: `github.com/jarviisha/codohue/sdk/go/redistream`

This module targets Go `1.24.13` and depends on
`github.com/redis/go-redis/v9`. If you only need the HTTP API client, use
`github.com/jarviisha/codohue/sdk/go` instead.

## Install

```bash
go get github.com/jarviisha/codohue/sdk/go/redistream
```

Shared wire types live in
`github.com/jarviisha/codohue/pkg/codohuetypes`.

## Quick start

```go
package main

import (
    "context"
    "time"

    "github.com/redis/go-redis/v9"

    "github.com/jarviisha/codohue/pkg/codohuetypes"
    "github.com/jarviisha/codohue/sdk/go/redistream"
)

func main() {
    ctx := context.Background()
    rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

    p := redistream.NewProducer(rdb)

    _, _ = p.Publish(ctx, codohuetypes.EventPayload{
        Namespace: "feed",
        SubjectID: "user-123",
        ObjectID:  "item-a",
        Action:    codohuetypes.ActionView,
        Timestamp: time.Now().UTC(),
    })
}
```

The producer publishes to the server ingest contract:

- stream: `codohue:events`
- field: `payload`
- value: JSON-encoded `codohuetypes.EventPayload`

These are exported as `codohuetypes.StreamName` and
`codohuetypes.PayloadField`.

## Batch publish

`PublishBatch` sends events sequentially and returns partial IDs if one `XADD`
fails. This lets callers resume from the last successfully published event.

## Development

This module lives inside the main Codohue repo under `sdk/go/redistream/`.
Its local `go.mod` replaces `github.com/jarviisha/codohue/pkg/codohuetypes`
with the in-repo module during local development.
