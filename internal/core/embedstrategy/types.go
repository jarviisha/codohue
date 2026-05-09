package embedstrategy

import (
	"context"
	"errors"
)

// Strategy is the contract every embedding implementation honours. The V1
// deterministic Go strategy and any future external-LLM strategy implement
// this same interface; the latter is the FR-021 / SC-009 forward-compat seam.
type Strategy interface {
	// ID returns the immutable identifier under which this strategy is registered.
	ID() string

	// Version returns the immutable strategy-version identifier paired with ID.
	// Bumping Version is the trigger for a namespace-wide re-embed.
	Version() string

	// Dim returns the produced vector length. Must equal the namespace's
	// embedding_dim for the strategy to be usable for that namespace.
	Dim() int

	// MaxInputBytes returns the per-strategy hard input cap. Returning 0 means
	// "no per-strategy cap; only the namespace-level catalog_max_content_bytes
	// applies".
	MaxInputBytes() int

	// Embed produces an L2-normalised dense vector for the given content. The
	// returned slice has length Dim() on success. Errors are or wrap one of
	// the sentinel errors below so callers can apply retry / dead-letter
	// policy uniformly.
	Embed(ctx context.Context, content string) ([]float32, error)
}

// Params is the FR-007 extension slot. V1 hashing strategy reads only the
// optional "dim" key. Future external-LLM strategies read e.g. credentials
// references and provider model identifiers.
//
// The map must be JSON-round-trippable so it can be persisted in
// namespace_configs.catalog_strategy_params (JSONB).
type Params map[string]any

// Factory builds a Strategy for a given Params. External strategies use
// Factory to resolve credentials / open HTTP clients at construction time
// rather than on every Embed call.
type Factory func(p Params) (Strategy, error)

// StrategyDescriptor is a metadata-only view of a registered strategy variant.
// Returned by Registry.List for admin-UI listings. Each descriptor represents
// one concrete (id, version, dim) variant a strategy can produce — strategies
// whose dimension is parameterised expand into one descriptor per supported
// variant via Registry.RegisterVariants.
type StrategyDescriptor struct {
	ID            string
	Version       string
	Dim           int
	MaxInputBytes int
	Description   string
}

// Sentinel errors. These are the only errors the catalog/embedder layers
// reason about by identity; everything else is a wrapped Embed-time failure.
var (
	// ErrUnknownStrategy is returned by Registry.Build for an unregistered (id, version).
	ErrUnknownStrategy = errors.New("embedstrategy: unknown id/version")

	// ErrDimensionMismatch is returned by callers when a Strategy's Dim() does
	// not match the namespace's embedding_dim.
	ErrDimensionMismatch = errors.New("embedstrategy: produced dim != namespace embedding_dim")

	// ErrZeroNorm is returned when the strategy produced a zero-norm vector.
	// The catalog item is moved to dead_letter on this error — content will
	// not change on retry.
	ErrZeroNorm = errors.New("embedstrategy: produced zero-norm vector")

	// ErrInputTooLarge is returned when content exceeds the strategy's
	// MaxInputBytes(). Defence in depth: catalog ingest normally enforces the
	// per-namespace cap before reaching the strategy.
	ErrInputTooLarge = errors.New("embedstrategy: content exceeds strategy max input bytes")

	// ErrTransient indicates the failure is retryable (network, rate-limit,
	// 5xx). External-LLM strategies wrap such errors with ErrTransient. V1
	// hashing strategy never returns this.
	ErrTransient = errors.New("embedstrategy: transient")
)
