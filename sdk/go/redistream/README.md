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
        OccurredAt: time.Now().UTC(),
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

## Namespace generations

Event and catalog payloads carry an additive `namespace_generation`. It is
omitted for generation-1 namespaces, so a producer written before the field
existed keeps working during the adoption window.

Stamp it once a namespace has been deleted and recreated: its generation is no
longer 1, and a generation-less envelope for it is stale — the consumer acks
and drops it rather than retrying, because no retry can make it valid. The
generation is returned at namespace provisioning; pass it through
`WithNamespaceGeneration` so every publish carries it.

## Retention

Producers here set no `MAXLEN` and the streams have no TTL. Trimming is the
server's job: a retention pass reads every consumer group's progress and
removes only entries below the slowest group's frontier, so an entry still
pending anywhere survives. A producer-side cap would delete unprocessed work
during an outage — exactly when it matters most.
