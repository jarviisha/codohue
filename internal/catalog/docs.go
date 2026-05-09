// Package catalog owns the data-plane catalog ingest path. It accepts raw
// client content via POST /v1/namespaces/{ns}/catalog, persists each item
// to the catalog_items table with a content hash, and publishes pending
// items to the per-namespace Redis Stream catalog:embed:{ns} for the
// embedder worker to consume.
package catalog
