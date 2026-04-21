package codohue

import (
	"context"
	"net/http"
	"net/url"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// IngestEvent publishes a single behavioral event to the HTTP ingest endpoint.
// The namespace on the payload is overridden to match this wrapper's namespace,
// so callers may leave EventPayload.Namespace empty.
//
// For high-throughput ingestion, prefer the Redis Streams producer in the
// github.com/jarviisha/codohue/sdk/go/redistream subpackage.
func (n *Namespace) IngestEvent(ctx context.Context, event codohuetypes.EventPayload) error {
	event.Namespace = n.namespace
	path := "/v1/namespaces/" + url.PathEscape(n.namespace) + "/events"
	return n.client.do(ctx, http.MethodPost, path, n.apiKey, nil, event, nil)
}
