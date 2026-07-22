package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

// nsConfigUpsertSvc is the slice of nsconfig.Service the adapter actually
// uses. Declared as an interface here (not the concrete *nsconfig.Service)
// so the adapter is unit-testable without standing up the real service.
type nsConfigUpsertSvc interface {
	Upsert(ctx context.Context, ns string, req *nsconfig.UpsertRequest) (*nsconfig.UpsertResponse, error)
	RotateAPIKey(ctx context.Context, ns string) (*nsconfig.RotateAPIKeyResponse, error)
}

// nsConfigAdapter bridges admin.Service (which must not import nsconfig per the
// constitution) and nsconfig.Service. Both DTOs are pointer-field, and nil is
// forwarded as nil so PATCH semantics survive down to the SQL.
type nsConfigAdapter struct {
	svc nsConfigUpsertSvc
}

func (a *nsConfigAdapter) Upsert(ctx context.Context, namespace string, req *admin.NamespaceUpsertRequest) (*admin.NamespaceUpsertResponse, error) {
	// Pointers pass straight through: nil carries "not supplied" all the way
	// to the SQL, where COALESCE leaves the column alone. Dereferencing here
	// is what used to turn an unsent field into a zero value and wipe it.
	nsReq := &nsconfig.UpsertRequest{
		ActionWeights:   req.ActionWeights,
		Lambda:          req.Lambda,
		Gamma:           req.Gamma,
		Alpha:           req.Alpha,
		MaxResults:      req.MaxResults,
		SeenItemsDays:   req.SeenItemsDays,
		ExcludeAuthored: req.ExcludeAuthored,
		DenseSource:     req.DenseSource,
		EmbeddingDim:    req.EmbeddingDim,
		DenseDistance:   req.DenseDistance,
		TrendingWindow:  req.TrendingWindow,
		TrendingTTL:     req.TrendingTTL,
		LambdaTrending:  req.LambdaTrending,
	}

	resp, err := a.svc.Upsert(ctx, namespace, nsReq)
	if err != nil {
		return nil, mapNsConfigError(err)
	}

	out := &admin.NamespaceUpsertResponse{
		Namespace: resp.Namespace,
		UpdatedAt: resp.UpdatedAt,
	}
	if resp.APIKey != "" {
		key := resp.APIKey
		out.APIKey = &key
	}
	return out, nil
}

// RotateAPIKey adapts nsconfig's key rotation to the admin DTO. nsconfig's
// not-found sentinel becomes (nil, nil) so the admin handler maps it to 404
// without importing the peer domain's errors.
func (a *nsConfigAdapter) RotateAPIKey(ctx context.Context, namespace string) (*admin.NamespaceKeyRotateResponse, error) {
	resp, err := a.svc.RotateAPIKey(ctx, namespace)
	if err != nil {
		if errors.Is(err, nsconfig.ErrNamespaceNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &admin.NamespaceKeyRotateResponse{Namespace: resp.Namespace, APIKey: resp.APIKey}, nil
}

// mapNsConfigError translates nsconfig validation sentinels into the admin
// domain's equivalents, preserving the detail message so the operator sees
// which field failed. Unknown errors pass through (→ 500).
func mapNsConfigError(err error) error {
	switch {
	case errors.Is(err, nsconfig.ErrCatalogViaUpsert):
		return fmt.Errorf("%w: %s", admin.ErrCatalogSourceViaUpsert, err.Error())
	case errors.Is(err, nsconfig.ErrEmbeddingDimLocked):
		return fmt.Errorf("%w: %s", admin.ErrEmbeddingDimLocked, err.Error())
	case errors.Is(err, nsconfig.ErrInvalidConfig):
		return fmt.Errorf("%w: %s", admin.ErrNamespaceConfigInvalid, err.Error())
	default:
		return err
	}
}
