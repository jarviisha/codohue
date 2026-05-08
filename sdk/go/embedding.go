package codohue

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// StoreObjectEmbedding stores a dense vector (BYOE) for an object in this namespace.
// A dimension mismatch returns an error that matches ErrDimMismatch via errors.Is.
func (n *Namespace) StoreObjectEmbedding(ctx context.Context, objectID string, vector []float32) error {
	return n.storeEmbedding(ctx, "objects", objectID, vector)
}

// StoreSubjectEmbedding stores a dense vector (BYOE) for a subject in this namespace.
// A dimension mismatch returns an error that matches ErrDimMismatch via errors.Is.
func (n *Namespace) StoreSubjectEmbedding(ctx context.Context, subjectID string, vector []float32) error {
	return n.storeEmbedding(ctx, "subjects", subjectID, vector)
}

// DeleteObject removes an object from all Qdrant collections for this namespace.
// The operation is idempotent: deleting a non-existent object is not an error.
func (n *Namespace) DeleteObject(ctx context.Context, objectID string) error {
	if objectID == "" {
		return fmt.Errorf("codohue: objectID is required")
	}
	path := fmt.Sprintf("/v1/namespaces/%s/objects/%s",
		url.PathEscape(n.namespace), url.PathEscape(objectID))
	return n.client.do(ctx, http.MethodDelete, path, n.apiKey, nil, nil, nil)
}

func (n *Namespace) storeEmbedding(ctx context.Context, entity, id string, vector []float32) error {
	if id == "" {
		return fmt.Errorf("codohue: id is required")
	}
	if len(vector) == 0 {
		return fmt.Errorf("codohue: vector is required")
	}
	body := codohuetypes.EmbeddingRequest{Vector: vector}
	path := fmt.Sprintf("/v1/namespaces/%s/%s/%s/embedding",
		url.PathEscape(n.namespace), entity, url.PathEscape(id))
	return n.client.do(ctx, http.MethodPut, path, n.apiKey, nil, body, nil)
}
