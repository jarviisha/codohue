# Contract: Embedding Strategy Interface

**Feature**: 004-catalog-embedder
**Date**: 2026-05-09
**Package**: `internal/core/embedstrategy/`
**Realises**: FR-021 (forward-compat seam), SC-009 (V2 strategies are purely additive)

This is the single most important internal contract introduced by this feature. It is the seam through which V2 external-LLM strategies plug in without altering any other code. The shape MUST NOT change in a backwards-incompatible way after V1 ships.

---

## Package placement rationale

`internal/core/embedstrategy/` (a NEW subdirectory of the existing `internal/core/`).

The constitution permits any `internal/<domain>/` to import `internal/core/...`. By placing the contract here:

- `internal/embedder/` (V1 hashing strategy + future LLM strategies + worker) can import it.
- `internal/admin/` (validates `(strategy_id, strategy_version)` at namespace enable time and lists `available_strategies` for the UI) can import it.
- `internal/catalog/` (records the active strategy on each ingest) can import it.

No `internal/<domain>/` needs to import another peer domain. Cross-domain rule preserved.

---

## Public surface

```go
// Package embedstrategy defines the contract every embedding implementation
// honours. The V1 deterministic Go strategy and any future external-LLM
// strategy implement the same Strategy interface, registering themselves
// against the package-level Registry from init().
package embedstrategy

import (
    "context"
    "errors"
)

// Strategy is the V1 contract every embedding implementation honours.
// V2 external-LLM strategies satisfy the same interface — that is the
// FR-021 / SC-009 forward-compat seam.
type Strategy interface {
    // ID is the immutable identifier under which this strategy is registered.
    // Examples: "internal-hashing-ngrams", "external-openai-embedding-3".
    ID() string

    // Version is the immutable strategy-version identifier paired with ID.
    // Examples: "v1", "large", "small". Bumping Version() is the trigger
    // for a namespace-wide re-embed (FR-013).
    Version() string

    // Dim is the produced vector length. MUST equal namespace.embedding_dim
    // for a namespace that uses this strategy, otherwise enabling fails.
    Dim() int

    // MaxInputBytes is the per-strategy hard input cap. Returning 0 means
    // "no per-strategy cap; only the namespace-level catalog_max_content_bytes
    // applies". V1 hashing strategy returns 0. Future external-LLM strategies
    // return their provider's documented input cap.
    MaxInputBytes() int

    // Embed produces an L2-normalised dense vector for the given content.
    // The returned slice MUST have length Dim(). MUST be deterministic for
    // deterministic strategies (i.e. same content → same vector). External
    // strategies are not required to be deterministic but MUST still return
    // a vector of length Dim() on success.
    //
    // Errors MUST be one of the sentinel errors below or wrap a sentinel via
    // errors.Is so the worker can apply retry/dead-letter policy uniformly.
    Embed(ctx context.Context, content string) ([]float32, error)
}

// Params is the FR-007 extension slot.
//
// V1 hashing strategy reads NOTHING from Params; an empty map MUST work.
// Future external-LLM strategies read e.g.:
//   {"api_key_secret_ref": "vault://path", "model": "text-embedding-3-large", "timeout_ms": 5000}
//
// The map MUST be JSON-round-trippable so it can be persisted in
// namespace_configs.catalog_strategy_params (JSONB).
type Params map[string]any

// Factory builds a Strategy for a given Params. External strategies use
// Factory to resolve credentials / open HTTP clients at construction time
// rather than on every Embed call.
type Factory func(p Params) (Strategy, error)

// Registry holds (id, version) → Factory bindings. Strategies self-register
// from init() against the package-level DefaultRegistry().
type Registry struct {
    // unexported
}

// DefaultRegistry returns the process-singleton registry that V1 hashing
// strategy and future strategies register against from init().
func DefaultRegistry() *Registry

// Register associates (id, version) with a Factory. Calling Register twice
// for the same (id, version) panics — registrations are immutable for the
// life of the process.
func (r *Registry) Register(id, version string, f Factory)

// Build constructs a Strategy for the given (id, version, params). Returns
// ErrUnknownStrategy if (id, version) is not registered.
func (r *Registry) Build(id, version string, p Params) (Strategy, error)

// Has reports whether (id, version) is registered.
func (r *Registry) Has(id, version string) bool

// List returns every registered strategy as a descriptor, useful for the
// admin UI's available_strategies field.
func (r *Registry) List() []StrategyDescriptor

// StrategyDescriptor is a metadata-only view of a registered strategy,
// suitable for admin-UI listings.
type StrategyDescriptor struct {
    ID          string
    Version     string
    Dim         int
    MaxInputBytes int
    Description string
}

// Sentinel errors. These are the only errors the catalog/embedder layers
// reason about by identity; everything else is a wrapped Embed-time failure.
var (
    // ErrUnknownStrategy is returned by Registry.Build for an unregistered (id, version).
    ErrUnknownStrategy = errors.New("embedstrategy: unknown id/version")

    // ErrDimensionMismatch is returned by callers when a Strategy's Dim()
    // does not match the namespace's embedding_dim. Returned at admin enable
    // time and at runtime defence-in-depth.
    ErrDimensionMismatch = errors.New("embedstrategy: produced dim != namespace embedding_dim")

    // ErrZeroNorm is returned when the strategy produced a zero-norm vector
    // (typically because all tokens were filtered out). The catalog item is
    // moved to dead_letter on this error (no retry — content will not change
    // on retry).
    ErrZeroNorm = errors.New("embedstrategy: produced zero-norm vector")

    // ErrInputTooLarge is returned when content exceeds the strategy's
    // MaxInputBytes(). Should not normally fire because the catalog ingest
    // layer enforces catalog_max_content_bytes; this is defence in depth.
    ErrInputTooLarge = errors.New("embedstrategy: content exceeds strategy max input bytes")

    // ErrTransient indicates the failure is retryable. External-LLM
    // strategies wrap network / 5xx / rate-limit errors with ErrTransient.
    // V1 hashing strategy never returns this.
    ErrTransient = errors.New("embedstrategy: transient")
)
```

---

## Lifecycle expectations

| Phase | What happens |
|---|---|
| Process boot | Each strategy implementation's `init()` calls `embedstrategy.DefaultRegistry().Register(id, version, factory)`. The set of registered strategies is fixed for the life of the process. |
| Namespace enable | Admin handler resolves `(strategy_id, strategy_version)` against the registry, calls `Build(id, ver, params)`, asserts `Strategy.Dim() == namespace.embedding_dim`. Persists the profile. |
| Per-item embed | Embedder worker calls `Build(id, ver, params)` once at namespace activation (caches per `(id,ver,paramsHash)`), then calls `Strategy.Embed(ctx, content)` per item. Cache eviction on namespace config change. |
| Re-embed | Identical to per-item embed; same cached `Strategy` instance handles the bulk re-publish. |
| Process restart | Cache is empty; rebuilds from registry on first item per namespace. No persisted state. |

---

## Error semantics for the worker

The embedder worker maps strategy errors to `catalog_items.state` transitions as follows:

| Returned error (or wrapped) | Worker action |
|---|---|
| `nil` (success) | `state='embedded'`, write Qdrant point, increment success counter. |
| `errors.Is(err, ErrZeroNorm)` | `state='dead_letter'` immediately. No retry — same content will produce the same zero vector. |
| `errors.Is(err, ErrInputTooLarge)` | `state='dead_letter'` immediately. Should not occur in practice (caught at ingest). |
| `errors.Is(err, ErrDimensionMismatch)` | `state='dead_letter'` immediately. Indicates strategy + namespace misconfiguration; operator must fix. |
| `errors.Is(err, ErrTransient)` | Increment `attempt_count`. If `attempt_count >= max_attempts` → `dead_letter`. Else → `failed` then re-publish on next sweep. |
| Any other error (including `ctx.Err()` cancellation) | Treat as transient. |

This mapping is uniform across V1 hashing and future external-LLM strategies; that uniformity is the FR-010 "uniform retry / dead-letter contract across strategies" requirement.

---

## V1 hashing strategy concrete contract

To pin down what V1 ships:

| Property | Value |
|---|---|
| `ID()` | `"internal-hashing-ngrams"` |
| `Version()` | `"v1"` |
| `Dim()` | reads from Params; valid values: 64, 128, 256, 512 (powers of two; chosen at registration time per dim variant — see below) |
| `MaxInputBytes()` | 0 (no per-strategy cap; namespace-level cap applies) |
| `Embed` determinism | bitwise-identical output for identical input |

Because operators select `(id, version)` and dim must match `namespace.embedding_dim`, the V1 hashing strategy registers ONE Factory under `("internal-hashing-ngrams", "v1")` and the Factory reads `Params["dim"]` to construct a Strategy at the requested dim. The admin `List()` call expands a single registered Factory into multiple `StrategyDescriptor`s — one per valid dim — so the UI can present the matching dim option for any namespace's `embedding_dim`. (Concretely: registration is `(id, version, factory)`, and `List()` calls a hook on the factory to enumerate valid dims; this hook is the only V1-specific extension to the contract.)

V2 external-LLM strategies typically expose ONE dim per `(id, version)` (e.g. `("external-openai-embedding-3","large")` is fixed at 3072) and so do not need the dim-enumeration hook — their `List()` entry is a single descriptor.

---

## Backwards compatibility commitments

V2 changes that are **allowed** without breaking V1 code:

- Registering new `(id, version)` pairs.
- Adding new exported types in `embedstrategy` so long as existing types are unchanged.
- Adding new sentinel errors so long as the error-to-state mapping above remains a superset.
- Adding new optional methods on `Strategy` via type-assertion extension interfaces (e.g. `BatchEmbedder` for strategies that can embed N items in one call).

V2 changes that are **forbidden**:

- Changing the signature of any method on `Strategy` listed above.
- Changing the meaning of any sentinel error.
- Removing exported types or exported fields.
- Reordering or renaming `StrategyDescriptor` fields.

These constraints concretise FR-021's "purely additive change" requirement and SC-009's "no migration of existing namespace records or catalog records is required for namespaces that stay on a previously-active strategy".
