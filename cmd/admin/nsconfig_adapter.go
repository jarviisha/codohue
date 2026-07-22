package main

import (
	"context"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

// nsConfigUpsertSvc is the slice of nsconfig.Service the adapter actually
// uses. Declared as an interface here (not the concrete *nsconfig.Service)
// so the adapter is unit-testable without standing up the real service.
type nsConfigUpsertSvc interface {
	Upsert(ctx context.Context, ns string, req *nsconfig.UpsertRequest) (*nsconfig.UpsertResponse, error)
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
		return nil, err
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
