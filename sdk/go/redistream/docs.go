// Package redistream provides Redis Streams producers for Codohue's two
// durable transports: Producer publishes behavioral events to
// codohue:events, and CatalogProducer publishes raw catalog content to
// codohue:catalog. Neither stream is producer-trimmed — entries persist
// until the server consumes and acknowledges them, so anything published
// while Codohue is unreachable is ingested on recovery with no retries.
//
// This package lives in a subpackage so that the core SDK does not force a
// dependency on github.com/redis/go-redis/v9. Only consumers that opt into
// Redis-based ingestion pull in that dependency.
package redistream
