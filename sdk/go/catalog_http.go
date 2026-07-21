package codohue

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// IngestCatalog publishes a single catalog item to the catalog auto-embedding
// pipeline. Content is the only field that feeds the embedder; Metadata is
// stored verbatim alongside the row. The server returns 202 Accepted on
// success — the dense vector is upserted asynchronously by the embedder
// worker.
//
// The namespace's dense_source must be "catalog"; otherwise the server
// returns 404 with code "namespace_not_enabled" (mapped to ErrNotFound).
func (n *Namespace) IngestCatalog(ctx context.Context, req codohuetypes.CatalogIngestRequest) error {
	if req.ObjectID == "" {
		return fmt.Errorf("codohue: object_id is required")
	}
	path := "/v1/namespaces/" + url.PathEscape(n.namespace) + "/catalog"
	return n.client.do(ctx, http.MethodPost, path, n.apiKey, nil, req, nil)
}
