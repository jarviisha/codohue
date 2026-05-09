package nsconfig

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/namespace"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 10

// ErrNamespaceNotFound is returned by catalog-related methods when the
// namespace row does not exist (caller is expected to call Upsert first).
var ErrNamespaceNotFound = errors.New("nsconfig: namespace not found")

type nsConfigRepository interface {
	Upsert(ctx context.Context, namespace string, req *UpsertRequest) (*namespace.Config, error)
	SetAPIKeyHash(ctx context.Context, namespace, hash string) error
	Get(ctx context.Context, namespace string) (*namespace.Config, error)
	UpsertCatalogConfig(ctx context.Context, namespace string, req *UpdateCatalogRequest) (*namespace.Config, error)
	ListCatalogEnabled(ctx context.Context) ([]*namespace.Config, error)
}

// Service provides business logic for managing namespace configuration.
//
// The registry field is the embedding-strategy registry used by
// UpdateCatalogConfig to validate (strategy_id, strategy_version) and assert
// the produced dimension matches the namespace's embedding_dim. It defaults
// to embedstrategy.DefaultRegistry(); tests in this package may overwrite it
// directly to inject a clean registry instance.
type Service struct {
	repo     nsConfigRepository
	registry *embedstrategy.Registry
}

// NewService creates a new Service with the given repository. The catalog
// strategy registry defaults to embedstrategy.DefaultRegistry().
func NewService(repo *Repository) *Service {
	return &Service{
		repo:     repo,
		registry: embedstrategy.DefaultRegistry(),
	}
}

// Upsert creates or updates the configuration for a namespace.
// On first creation, a namespace-scoped API key is generated and returned as
// plaintext in UpsertResponse.APIKey. The plaintext key is shown once only —
// subsequent updates will not return the key.
func (s *Service) Upsert(ctx context.Context, ns string, req *UpsertRequest) (*UpsertResponse, error) {
	cfg, err := s.repo.Upsert(ctx, ns, req)
	if err != nil {
		return nil, fmt.Errorf("upsert namespace config: %w", err)
	}

	resp := &UpsertResponse{
		Namespace: cfg.Namespace,
		UpdatedAt: cfg.UpdatedAt,
	}

	// If no API key exists for this namespace yet, generate one now.
	if cfg.APIKeyHash == "" {
		plaintext, hash, err := generateAPIKey()
		if err != nil {
			return nil, fmt.Errorf("generate api key: %w", err)
		}
		if err := s.repo.SetAPIKeyHash(ctx, ns, hash); err != nil {
			return nil, fmt.Errorf("store api key hash: %w", err)
		}
		resp.APIKey = plaintext
	}

	return resp, nil
}

// Get returns the configuration for a namespace, or nil if it does not exist.
func (s *Service) Get(ctx context.Context, ns string) (*namespace.Config, error) {
	cfg, err := s.repo.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get namespace config: %w", err)
	}
	return cfg, nil
}

// ListCatalogEnabled returns every namespace that currently has catalog
// auto-embedding enabled. Used by the embedder binary's namespace poller.
func (s *Service) ListCatalogEnabled(ctx context.Context) ([]*namespace.Config, error) {
	cfgs, err := s.repo.ListCatalogEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("list catalog-enabled namespaces: %w", err)
	}
	return cfgs, nil
}

// UpdateCatalogConfig persists the catalog auto-embedding configuration for a
// namespace. When req.Enabled is true, the (strategy_id, strategy_version)
// pair must resolve via the registry and the strategy's Dim() must equal the
// namespace's existing embedding_dim — otherwise *DimensionMismatchError or
// embedstrategy.ErrUnknownStrategy is returned and no DB write is performed.
//
// When req.Enabled is false, strategy fields are persisted as NULL regardless
// of what is in the request body.
//
// Returns ErrNamespaceNotFound if the namespace row does not exist.
func (s *Service) UpdateCatalogConfig(ctx context.Context, ns string, req *UpdateCatalogRequest) (*namespace.Config, error) {
	cfg, err := s.repo.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("load namespace config: %w", err)
	}
	if cfg == nil {
		return nil, ErrNamespaceNotFound
	}

	if req.Enabled {
		if req.StrategyID == "" || req.StrategyVersion == "" {
			return nil, fmt.Errorf("strategy_id and strategy_version are required when enabling catalog")
		}
		strategy, err := s.registry.Build(req.StrategyID, req.StrategyVersion, embedstrategy.Params(req.Params))
		if err != nil {
			return nil, err
		}
		if strategy.Dim() != cfg.EmbeddingDim {
			return nil, &DimensionMismatchError{
				StrategyDim:           strategy.Dim(),
				NamespaceEmbeddingDim: cfg.EmbeddingDim,
			}
		}
	}

	updated, err := s.repo.UpsertCatalogConfig(ctx, ns, req)
	if err != nil {
		return nil, fmt.Errorf("persist catalog config: %w", err)
	}
	if updated == nil {
		// The namespace existed at Get time but the UPDATE matched no rows —
		// extreme race or DB inconsistency. Surface it explicitly rather than
		// returning a nil config.
		return nil, ErrNamespaceNotFound
	}
	return updated, nil
}

// generateAPIKey creates a cryptographically random 32-byte key and returns
// both the hex-encoded plaintext and its bcrypt hash.
func generateAPIKey() (plaintext, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("read random bytes: %w", err)
	}
	plaintext = hex.EncodeToString(raw)

	hashBytes, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)
	if err != nil {
		return "", "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return plaintext, string(hashBytes), nil
}
