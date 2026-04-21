// Package codohue is the Go client SDK for the Codohue recommendation engine.
//
// Construct a Client with New and obtain a namespace-scoped wrapper with
// Client.Namespace to invoke data-plane endpoints such as Recommend, Rank,
// Trending, IngestEvent, StoreObjectEmbedding, StoreSubjectEmbedding, and
// DeleteObject.
//
// For Redis Streams-based event ingestion, use the sibling subpackage
// github.com/jarviisha/codohue/sdk/go/redistream.
package codohue
