package codohuetypes

// Redis Streams contract for event ingestion. Clients publishing events via
// Redis must XADD to StreamName with a PayloadField containing a JSON-encoded
// EventPayload.
const (
	StreamName   = "codohue:events"
	PayloadField = "payload"
	// NamespaceGenerationField is the additive lifecycle-generation envelope
	// field used by producers that encode stream values without JSON payloads.
	NamespaceGenerationField = "namespace_generation"
)

// CatalogStreamName is the durable transport for catalog content, symmetric
// with StreamName: XADD a PayloadField containing a JSON-encoded
// CatalogStreamItem. Neither stream is producer-trimmed — entries persist
// until the ingest worker consumes and acknowledges them, which is what makes
// the transport survive a Codohue outage of any duration.
const CatalogStreamName = "codohue:catalog"
