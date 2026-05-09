package embedstrategy

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds (id, version) -> Factory bindings. Strategies self-register
// against the package-level DefaultRegistry from init().
type Registry struct {
	mu        sync.RWMutex
	factories map[strategyKey]Factory
	variants  map[strategyKey][]StrategyDescriptor
}

type strategyKey struct {
	id      string
	version string
}

// NewRegistry constructs an empty Registry. Most callers use DefaultRegistry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[strategyKey]Factory),
		variants:  make(map[strategyKey][]StrategyDescriptor),
	}
}

var defaultRegistry = NewRegistry()

// DefaultRegistry returns the process-singleton registry. V1 hashing strategy
// and future strategies register against this from init().
func DefaultRegistry() *Registry {
	return defaultRegistry
}

// Register associates (id, version) with a Factory. The factory is expected
// to return a Strategy at a fixed dimension; the descriptor surfaced through
// List is taken from a Strategy built with empty Params. For strategies
// whose dimension is parameterised by Params, use RegisterVariants instead.
//
// Registering twice for the same (id, version) panics — registrations are
// immutable for the life of the process to avoid silent cross-test pollution.
func (r *Registry) Register(id, version string, f Factory) {
	r.register(id, version, f, nil)
}

// RegisterVariants registers a Factory together with an explicit list of
// descriptor variants. Use this when a single (id, version) factory builds
// strategies at multiple dimensions (or other knobs) selected via Params —
// for example the V1 hashing strategy that supports several dims.
//
// Each descriptor's ID and Version MUST equal the id and version arguments;
// otherwise this panics. List returns the supplied descriptors verbatim
// rather than calling the factory with empty Params.
func (r *Registry) RegisterVariants(id, version string, f Factory, variants []StrategyDescriptor) {
	for _, d := range variants {
		if d.ID != id || d.Version != version {
			panic(fmt.Sprintf("embedstrategy: variant descriptor %s@%s does not match registration %s@%s", d.ID, d.Version, id, version))
		}
	}
	cloned := make([]StrategyDescriptor, len(variants))
	copy(cloned, variants)
	r.register(id, version, f, cloned)
}

func (r *Registry) register(id, version string, f Factory, variants []StrategyDescriptor) {
	if id == "" || version == "" {
		panic("embedstrategy: id and version must be non-empty")
	}
	if f == nil {
		panic("embedstrategy: factory must be non-nil")
	}
	key := strategyKey{id: id, version: version}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[key]; exists {
		panic(fmt.Sprintf("embedstrategy: duplicate registration for %s@%s", id, version))
	}
	r.factories[key] = f
	if variants != nil {
		r.variants[key] = variants
	}
}

// Build constructs a Strategy for the given (id, version, params). Returns
// ErrUnknownStrategy if (id, version) is not registered. Any error from the
// factory itself is returned as-is.
func (r *Registry) Build(id, version string, p Params) (Strategy, error) {
	r.mu.RLock()
	f, ok := r.factories[strategyKey{id: id, version: version}]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s@%s", ErrUnknownStrategy, id, version)
	}
	return f(p)
}

// Has reports whether (id, version) is registered.
func (r *Registry) Has(id, version string) bool {
	r.mu.RLock()
	_, ok := r.factories[strategyKey{id: id, version: version}]
	r.mu.RUnlock()
	return ok
}

// List returns every registered strategy variant as a descriptor, useful for
// the admin UI's available_strategies field. Strategies registered through
// RegisterVariants contribute one descriptor per variant; strategies
// registered through Register contribute a single descriptor whose Dim and
// MaxInputBytes are taken from a Strategy built with empty Params.
//
// Factories registered through Register that fail with empty Params (because
// they require credentials or other configuration) are skipped silently so a
// missing OpenAI key in tests does not break listing.
//
// The result is sorted by (id, version, dim) for stable output.
func (r *Registry) List() []StrategyDescriptor {
	r.mu.RLock()
	keys := make([]strategyKey, 0, len(r.factories))
	factories := make(map[strategyKey]Factory, len(r.factories))
	variants := make(map[strategyKey][]StrategyDescriptor, len(r.variants))
	for k, f := range r.factories {
		keys = append(keys, k)
		factories[k] = f
	}
	for k, v := range r.variants {
		cloned := make([]StrategyDescriptor, len(v))
		copy(cloned, v)
		variants[k] = cloned
	}
	r.mu.RUnlock()

	descriptors := make([]StrategyDescriptor, 0, len(keys))
	for _, k := range keys {
		if vs, ok := variants[k]; ok {
			descriptors = append(descriptors, vs...)
			continue
		}
		s, err := factories[k](Params{})
		if err != nil {
			continue
		}
		descriptors = append(descriptors, StrategyDescriptor{
			ID:            s.ID(),
			Version:       s.Version(),
			Dim:           s.Dim(),
			MaxInputBytes: s.MaxInputBytes(),
		})
	}

	sort.Slice(descriptors, func(i, j int) bool {
		if descriptors[i].ID != descriptors[j].ID {
			return descriptors[i].ID < descriptors[j].ID
		}
		if descriptors[i].Version != descriptors[j].Version {
			return descriptors[i].Version < descriptors[j].Version
		}
		return descriptors[i].Dim < descriptors[j].Dim
	})
	return descriptors
}
