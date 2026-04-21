package codohue

// Namespace groups the data-plane operations for a single namespace, binding
// the namespace identifier and its API key so callers don't repeat them on
// every call. Obtain one via Client.Namespace.
type Namespace struct {
	client    *Client
	namespace string
	apiKey    string
}

// Namespace returns a namespace-scoped wrapper around the client. apiKey is
// sent as the Bearer token on every call. Pass the global admin key when the
// namespace has no provisioned per-namespace key.
func (c *Client) Namespace(ns, apiKey string) *Namespace {
	return &Namespace{client: c, namespace: ns, apiKey: apiKey}
}

// Name returns the namespace identifier this wrapper is bound to.
func (n *Namespace) Name() string { return n.namespace }
