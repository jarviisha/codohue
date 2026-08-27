package main

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/jarviisha/codohue/internal/objects"
)

// objectAttributionAdapter lets catalog ingest write an object's author inside
// its own transaction without importing the objects domain: catalog declares
// the method it needs, this adapter binds it to a transaction-scoped objects
// repository, and the objects service keeps owning the rules (trimming, empty
// means "unspecified", the lifecycle fence).
type objectAttributionAdapter struct {
	svc *objects.Service
}

func (a *objectAttributionAdapter) SetAuthorTx(ctx context.Context, tx pgx.Tx, namespace, objectID, authorSubjectID string) error {
	// tx is nil only on the pool-less path used by unit tests; fall back to
	// the service's own repository there.
	if tx == nil {
		return a.svc.SetAuthor(ctx, namespace, objectID, authorSubjectID)
	}
	return a.svc.SetAuthorWithRepo(ctx, objects.NewRepositoryTx(tx), namespace, objectID, authorSubjectID)
}
