package main

import (
	"context"
	"errors"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

// nsConfigAdapter bridges admin.Service (which must not import nsconfig per the
// constitution) and nsconfig.Service. It maps the admin pointer-field DTOs to
// nsconfig's value-field DTOs, treating nil pointers as Go zero values to match
// the prior JSON round-trip behavior.
type nsConfigAdapter struct {
	svc *nsconfig.Service
}

func (a *nsConfigAdapter) Upsert(ctx context.Context, namespace string, req *admin.NamespaceUpsertRequest) (*admin.NamespaceUpsertResponse, error) {
	nsReq := &nsconfig.UpsertRequest{
		ActionWeights: req.ActionWeights,
	}
	if req.Lambda != nil {
		nsReq.Lambda = *req.Lambda
	}
	if req.Gamma != nil {
		nsReq.Gamma = *req.Gamma
	}
	if req.Alpha != nil {
		nsReq.Alpha = *req.Alpha
	}
	if req.MaxResults != nil {
		nsReq.MaxResults = *req.MaxResults
	}
	if req.SeenItemsDays != nil {
		nsReq.SeenItemsDays = *req.SeenItemsDays
	}
	if req.DenseStrategy != nil {
		nsReq.DenseStrategy = *req.DenseStrategy
	}
	if req.EmbeddingDim != nil {
		nsReq.EmbeddingDim = *req.EmbeddingDim
	}
	if req.DenseDistance != nil {
		nsReq.DenseDistance = *req.DenseDistance
	}
	if req.TrendingWindow != nil {
		nsReq.TrendingWindow = *req.TrendingWindow
	}
	if req.TrendingTTL != nil {
		nsReq.TrendingTTL = *req.TrendingTTL
	}
	if req.LambdaTrending != nil {
		nsReq.LambdaTrending = *req.LambdaTrending
	}

	resp, err := a.svc.Upsert(ctx, namespace, nsReq)
	if err != nil {
		var conflictErr *nsconfig.DenseStrategyConflictError
		if errors.As(err, &conflictErr) {
			return nil, &admin.CatalogStrategyConflict{
				DenseStrategy:  conflictErr.DenseStrategy,
				CatalogEnabled: conflictErr.CatalogEnabled,
			}
		}
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
