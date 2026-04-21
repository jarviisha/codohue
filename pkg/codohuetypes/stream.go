package codohuetypes

// Redis Streams contract for event ingestion. Clients publishing events via
// Redis must XADD to StreamName with a PayloadField containing a JSON-encoded
// EventPayload.
const (
	StreamName   = "codohue:events"
	PayloadField = "payload"
)
