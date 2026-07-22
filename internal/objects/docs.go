// Package objects stores per-object metadata that is independent of how (or
// whether) an object is embedded. It is the home for facts about the object
// itself — currently which subject authored it — so those facts work under
// every dense_source, not only under catalog auto-embedding.
//
// It deliberately does not overlap with internal/catalog: catalog_items holds
// embedding input and its lifecycle, this package holds attribution.
package objects
