package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

// nsCatalogConfigSvc is the slice of nsconfig.Service this adapter calls.
// Declared as an interface so the cmd/admin tests can drive error mapping
// without constructing the real service.
type nsCatalogConfigSvc interface {
	Get(ctx context.Context, ns string) (*namespace.Config, error)
	UpdateCatalogConfig(ctx context.Context, ns string, req *nsconfig.UpdateCatalogRequest) (*namespace.Config, error)
}

// catalogConfigAdapter bridges admin.Service to nsconfig.Service +
// embedstrategy.DefaultRegistry for the US2 catalog config endpoints.
// It lives in cmd/admin (the wiring layer) rather than internal/admin so
// the admin domain need not import nsconfig or embedstrategy directly —
// both would be cross-domain imports forbidden by the constitution.
type catalogConfigAdapter struct {
	nsSvc    nsCatalogConfigSvc
	registry *embedstrategy.Registry
}

func newCatalogConfigAdapter(nsSvc nsCatalogConfigSvc, registry *embedstrategy.Registry) *catalogConfigAdapter {
	return &catalogConfigAdapter{nsSvc: nsSvc, registry: registry}
}

// GetCatalog returns the current catalog config for a namespace. nil result
// when the namespace does not exist; lets the admin handler return 404.
func (a *catalogConfigAdapter) GetCatalog(ctx context.Context, namespace string) (*admin.NamespaceCatalogConfig, error) {
	cfg, err := a.nsSvc.Get(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get namespace: %w", err)
	}
	if cfg == nil {
		return nil, nil
	}
	return &admin.NamespaceCatalogConfig{
		Namespace:       cfg.Namespace,
		Enabled:         cfg.CatalogEnabled,
		StrategyID:      cfg.CatalogStrategyID,
		StrategyVersion: cfg.CatalogStrategyVersion,
		Params:          cfg.CatalogStrategyParams,
		EmbeddingDim:    cfg.EmbeddingDim,
		MaxAttempts:     cfg.CatalogMaxAttempts,
		MaxContentBytes: cfg.CatalogMaxContentBytes,
		UpdatedAt:       cfg.UpdatedAt,
	}, nil
}

// UpdateCatalog applies the request via nsconfig.Service.UpdateCatalogConfig
// and translates the typed errors:
//   - nsconfig.DimensionMismatchError → admin.CatalogDimensionMismatch
//   - nsconfig.ErrNamespaceNotFound   → nil result + nil error
//   - embedstrategy.ErrUnknownStrategy → returned as-is so the handler maps
//     it to 400 via its default branch
func (a *catalogConfigAdapter) UpdateCatalog(ctx context.Context, namespace string, req *admin.NamespaceCatalogUpdateRequest) (*admin.NamespaceCatalogConfig, error) {
	nsReq := &nsconfig.UpdateCatalogRequest{
		Enabled: req.Enabled,
		Params:  req.Params,
	}
	if req.StrategyID != nil {
		nsReq.StrategyID = *req.StrategyID
	}
	if req.StrategyVersion != nil {
		nsReq.StrategyVersion = *req.StrategyVersion
	}
	if req.MaxAttempts != nil {
		nsReq.MaxAttempts = *req.MaxAttempts
	}
	if req.MaxContentBytes != nil {
		nsReq.MaxContentBytes = *req.MaxContentBytes
	}

	cfg, err := a.nsSvc.UpdateCatalogConfig(ctx, namespace, nsReq)
	if err != nil {
		var dimErr *nsconfig.DimensionMismatchError
		var conflictErr *nsconfig.DenseStrategyConflictError
		switch {
		case errors.Is(err, nsconfig.ErrNamespaceNotFound):
			return nil, nil
		case errors.As(err, &dimErr):
			return nil, &admin.CatalogDimensionMismatch{
				StrategyDim:           dimErr.StrategyDim,
				NamespaceEmbeddingDim: dimErr.NamespaceEmbeddingDim,
			}
		case errors.As(err, &conflictErr):
			return nil, &admin.CatalogStrategyConflict{
				DenseStrategy:  conflictErr.DenseStrategy,
				CatalogEnabled: conflictErr.CatalogEnabled,
			}
		default:
			return nil, err
		}
	}

	return &admin.NamespaceCatalogConfig{
		Namespace:       cfg.Namespace,
		Enabled:         cfg.CatalogEnabled,
		StrategyID:      cfg.CatalogStrategyID,
		StrategyVersion: cfg.CatalogStrategyVersion,
		Params:          cfg.CatalogStrategyParams,
		EmbeddingDim:    cfg.EmbeddingDim,
		MaxAttempts:     cfg.CatalogMaxAttempts,
		MaxContentBytes: cfg.CatalogMaxContentBytes,
		UpdatedAt:       cfg.UpdatedAt,
	}, nil
}

// GetCatalogStrategy implements admin.catalogStrategyPicker by looking up the
// namespace's active (strategy_id, strategy_version) pair via nsconfig.Service.
// enabled=false is returned for both "namespace missing" and "catalog disabled"
// so the caller can map to a single 404 (FR-008).
func (a *catalogConfigAdapter) GetCatalogStrategy(ctx context.Context, namespace string) (strategyID, strategyVersion string, enabled bool, err error) {
	cfg, err := a.nsSvc.Get(ctx, namespace)
	if err != nil {
		return "", "", false, fmt.Errorf("get namespace: %w", err)
	}
	if cfg == nil || !cfg.CatalogEnabled {
		return "", "", false, nil
	}
	return cfg.CatalogStrategyID, cfg.CatalogStrategyVersion, true, nil
}

// AvailableStrategies returns every registered strategy variant whose Dim
// matches the namespace's embedding_dim. The admin UI uses this to render
// the strategy picker with only admissible options. When namespaceEmbeddingDim
// is 0, all variants are returned (used by tests / unconfigured namespaces).
func (a *catalogConfigAdapter) AvailableStrategies(namespaceEmbeddingDim int) []admin.CatalogStrategyDescriptor {
	all := a.registry.List()
	out := make([]admin.CatalogStrategyDescriptor, 0, len(all))
	for _, d := range all {
		if namespaceEmbeddingDim > 0 && d.Dim != namespaceEmbeddingDim {
			continue
		}
		out = append(out, admin.CatalogStrategyDescriptor{
			ID:            d.ID,
			Version:       d.Version,
			Dim:           d.Dim,
			MaxInputBytes: d.MaxInputBytes,
			Description:   d.Description,
		})
	}
	return out
}
