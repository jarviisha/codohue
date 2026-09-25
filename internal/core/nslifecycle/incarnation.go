package nslifecycle

import (
	"context"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

// Incarnation identifies one incarnation of a namespace: the name plus the
// generation whose physical artifacts (Qdrant collections, Redis keys) it
// addresses. The "generation is at least 1" invariant lives here and nowhere
// else — construct one instead of clamping by hand.
type Incarnation struct {
	namespace  string
	generation int64
}

// NewIncarnation builds an Incarnation from a raw generation, which may come
// from an unvalidated source; anything below 1 addresses generation 1, the
// unqualified legacy names.
func NewIncarnation(ns string, generation int64) Incarnation {
	return Incarnation{namespace: ns, generation: generation}
}

// ConfigIncarnation resolves the incarnation a read path should address from
// namespace configuration. A nil config (config unavailable) addresses
// generation 1 rather than silently resolving to some other generation.
func ConfigIncarnation(ns string, cfg *namespace.Config) Incarnation {
	if cfg == nil {
		return Incarnation{namespace: ns}
	}
	return Incarnation{namespace: ns, generation: cfg.Generation}
}

// LeaseIncarnation returns the incarnation carried by the namespace
// lifecycle lease — the authority write paths must address. Read-only
// callers without a lease should use ConfigIncarnation instead.
func LeaseIncarnation(ctx context.Context, ns string) (Incarnation, bool) {
	generation, ok := LeaseGeneration(ctx, ns)
	if !ok {
		return Incarnation{}, false
	}
	return Incarnation{namespace: ns, generation: generation}, true
}

// Namespace returns the raw (logical) namespace name.
func (i Incarnation) Namespace() string { return i.namespace }

// Generation returns the generation, clamped to >= 1 — the single home of
// that invariant. The zero value therefore addresses generation 1.
func (i Incarnation) Generation() int64 {
	if i.generation < 1 {
		return 1
	}
	return i.generation
}

// PhysicalName derives the storage name for kind under this incarnation.
func (i Incarnation) PhysicalName(kind PhysicalKind) (string, error) {
	return PhysicalName(kind, i.namespace, i.Generation())
}

// MustPhysicalName is PhysicalName for call sites whose kind is already known
// valid; it panics only on an empty namespace or unknown kind.
func (i Incarnation) MustPhysicalName(kind PhysicalKind) string {
	return MustPhysicalName(kind, i.namespace, i.Generation())
}

// QdrantNamespace returns the namespace token used inside Qdrant collection
// names for this incarnation.
func (i Incarnation) QdrantNamespace() string {
	return QdrantNamespace(i.namespace, i.Generation())
}

// RedisNamespace returns the namespace token used inside Redis keys for this
// incarnation.
func (i Incarnation) RedisNamespace() string {
	return RedisNamespace(i.namespace, i.Generation())
}
