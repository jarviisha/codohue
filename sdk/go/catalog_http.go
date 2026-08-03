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

// IngestCatalogBatch publishes up to codohuetypes.CatalogBatchMaxItems catalog
// items in one request. Items are validated independently — check the
// per-item results for rejections; one bad item does not fail the batch. Use
// this for repair passes: a full-corpus walk costs O(corpus/batch) requests.
func (n *Namespace) IngestCatalogBatch(ctx context.Context, items []codohuetypes.CatalogIngestRequest) (*codohuetypes.CatalogBatchIngestResponse, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("codohue: items is required")
	}
	if len(items) > codohuetypes.CatalogBatchMaxItems {
		return nil, fmt.Errorf("codohue: at most %d items per batch, got %d", codohuetypes.CatalogBatchMaxItems, len(items))
	}
	path := "/v1/namespaces/" + url.PathEscape(n.namespace) + "/catalog/batch"
	var out codohuetypes.CatalogBatchIngestResponse
	if err := n.client.do(ctx, http.MethodPost, path, n.apiKey, nil, codohuetypes.CatalogBatchIngestRequest{Items: items}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListCatalogObjects is the reconciliation read: which object ids the
// namespace already holds, ordered by updated_at ascending. Pass a non-zero
// changedSince (RFC3339) to fetch only objects updated after that instant, so
// a repair pass re-sends the gap instead of the whole corpus.
func (n *Namespace) ListCatalogObjects(ctx context.Context, changedSince string, limit, offset int) (*codohuetypes.CatalogObjectsResponse, error) {
	q := url.Values{}
	if changedSince != "" {
		q.Set("changed_since", changedSince)
	}
	if limit > 0 {
		q.Set("limit", fmt.Sprint(limit))
	}
	if offset > 0 {
		q.Set("offset", fmt.Sprint(offset))
	}
	path := "/v1/namespaces/" + url.PathEscape(n.namespace) + "/catalog/objects"
	var out codohuetypes.CatalogObjectsResponse
	if err := n.client.do(ctx, http.MethodGet, path, n.apiKey, q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
