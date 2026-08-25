package codohue

// Namespace groups the data-plane operations for a single namespace, binding
// the namespace identifier and its API key so callers don't repeat them on
// every call. Obtain one via Client.Namespace.
type Namespace struct {
	client     *Client
	namespace  string
	apiKey     string
	generation int64
}

// Namespace returns a namespace-scoped wrapper around the client. apiKey is
// sent as the Bearer token on every call. Pass the global admin key when the
// namespace has no provisioned per-namespace key.
func (c *Client) Namespace(ns, apiKey string) *Namespace {
	return &Namespace{client: c, namespace: ns, apiKey: apiKey}
}

// NamespaceWithOptions returns a namespace wrapper with additive lifecycle
// metadata. Existing callers can keep using Namespace and generation zero.
func (c *Client) NamespaceWithOptions(ns, apiKey string, opts ...NamespaceOption) *Namespace {
	n := &Namespace{client: c, namespace: ns, apiKey: apiKey}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

// Name returns the namespace identifier this wrapper is bound to.
func (n *Namespace) Name() string { return n.namespace }

// Generation returns the provisioned namespace lifecycle generation. Zero
// means the caller is using the legacy, generation-unaware construction path.
func (n *Namespace) Generation() int64 { return n.generation }
