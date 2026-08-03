// Package ingest accepts behavioral events from Redis Streams and HTTP,
// validates them, and persists them to the events table in PostgreSQL.
//
// It also hosts the CatalogWorker, which consumes the durable client-facing
// codohue:catalog stream and hands each item to the catalog domain through a
// narrow interface implemented by a cmd/api adapter — the import rule keeps
// this package and internal/catalog apart. Both workers share the same
// at-least-once mechanics: consumer groups, an idle-entry reaper, and
// ack-after-persist (permanently invalid entries are acked and recorded
// rather than left to clog the PEL).
package ingest
