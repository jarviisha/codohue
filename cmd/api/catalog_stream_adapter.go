package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jarviisha/codohue/internal/catalog"
	"github.com/jarviisha/codohue/internal/ingest"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// catalogStreamAdapter feeds stream-delivered catalog content into
// catalog.Service. It lives here because the import rule forbids
// internal/ingest from importing the peer catalog domain — same pattern as
// cmd/admin/nsconfig_adapter.go. Its one real job beyond field mapping is
// error classification: catalog validation errors become
// ingest.ErrCatalogItemRejected (permanent → acked off the stream), anything
// else stays transient (left pending for redelivery).
type catalogStreamAdapter struct {
	svc *catalog.Service
}

func (a *catalogStreamAdapter) IngestStreamItem(ctx context.Context, item *codohuetypes.CatalogStreamItem) error {
	_, err := a.svc.Ingest(ctx, item.Namespace, &catalog.IngestRequest{
		ObjectID:        item.ObjectID,
		Content:         item.Content,
		AuthorSubjectID: item.AuthorSubjectID,
		Metadata:        item.Metadata,
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, catalog.ErrInvalidRequest),
		errors.Is(err, catalog.ErrEmptyContent),
		errors.Is(err, catalog.ErrContentTooLarge),
		errors.Is(err, catalog.ErrNamespaceNotFound),
		errors.Is(err, catalog.ErrNamespaceNotEnabled):
		return fmt.Errorf("%w: %v", ingest.ErrCatalogItemRejected, err)
	default:
		// Includes "row persisted but embed-stream publish failed": leaving
		// the entry pending redelivers it, and the content-hash short-circuit
		// makes the retry a no-op upsert while the embedder's recovery sweep
		// re-publishes the row.
		return err
	}
}
