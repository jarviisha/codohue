// Package embedder owns the V1 in-process deterministic embedding strategy
// (feature hashing trick + character n-grams) and the worker that drains the
// per-namespace Redis Streams catalog queue, embeds each item, and upserts
// the dense vector into Qdrant. Future external-LLM strategies plug in via
// the embedstrategy.Registry without touching this package's worker code.
package embedder
