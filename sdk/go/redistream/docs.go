// Package redistream provides a Redis Streams producer for publishing
// behavioral events to the Codohue ingest stream.
//
// This package lives in a subpackage so that the core SDK does not force a
// dependency on github.com/redis/go-redis/v9. Only consumers that opt into
// Redis-based ingestion pull in that dependency.
package redistream
