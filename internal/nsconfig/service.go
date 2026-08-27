package nsconfig

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 10

// ErrNamespaceNotFound is returned by catalog-related methods when the
// namespace row does not exist (caller is expected to call Upsert first).
var ErrNamespaceNotFound = errors.New("nsconfig: namespace not found")

type nsConfigRepository interface {
	Upsert(ctx context.Context, namespace string, req *UpsertRequest) (*namespace.Config, error)
	UpsertWithCatalog(ctx context.Context, namespace string, req *UpsertRequest, catalogReq *UpdateCatalogRequest) (*namespace.Config, error)
	SetAPIKeyHash(ctx context.Context, namespace, hash string) (won bool, err error)
	ReplaceAPIKeyHash(ctx context.Context, namespace, hash string) (found bool, err error)
	Get(ctx context.Context, namespace string) (*namespace.Config, error)
	UpsertCatalogConfig(ctx context.Context, namespace string, req *UpdateCatalogRequest) (*namespace.Config, error)
	ListCatalogNamespaces(ctx context.Context) ([]*namespace.Config, error)
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

	// denseCollections backs the embedding_dim change guard. Optional —
	// wired by cmd/admin (the only place config writes happen); nil skips
	// the guard.
	denseCollections DenseCollectionChecker
	lifecycle        interface {
		Activate(context.Context, string) (*nslifecycle.NamespaceLifecycle, error)
		WithWriter(context.Context, string, func(context.Context, *nslifecycle.NamespaceLifecycle) error) error
	}
}

// DenseCollectionChecker reports whether a namespace's dense Qdrant
// collections already exist. Defined here (implemented at the wiring layer)
// so nsconfig never grows a Qdrant dependency.
type DenseCollectionChecker interface {
	DenseCollectionsExist(ctx context.Context, namespace string) (bool, error)
}

// SetDenseCollectionChecker wires the embedding_dim change guard. Safe to
// call once at startup before serving.
func (s *Service) SetDenseCollectionChecker(c DenseCollectionChecker) { s.denseCollections = c }

// SetLifecycleCoordinator fences config creation/update and controls recreation.
func (s *Service) SetLifecycleCoordinator(coordinator interface {
	Activate(context.Context, string) (*nslifecycle.NamespaceLifecycle, error)
	WithWriter(context.Context, string, func(context.Context, *nslifecycle.NamespaceLifecycle) error) error
}) {
	s.lifecycle = coordinator
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
	if err := validateUpsert(req); err != nil {
		return nil, err
	}
	if err := s.guardEmbeddingDimChange(ctx, ns, req); err != nil {
		return nil, err
	}
	if s.lifecycle != nil && nslifecycle.RequireNamespaceLease(ctx, ns) != nil {
		if _, err := s.lifecycle.Activate(ctx, ns); err != nil {
			return nil, err
		}
		var response *UpsertResponse
		err := s.lifecycle.WithWriter(ctx, ns, func(leased context.Context, _ *nslifecycle.NamespaceLifecycle) error {
			var upsertErr error
			response, upsertErr = s.upsertActive(leased, ns, req)
			return upsertErr
		})
		return response, err
	}
	return s.upsertActive(ctx, ns, req)
}

func (s *Service) upsertActive(ctx context.Context, ns string, req *UpsertRequest) (*UpsertResponse, error) {
	// dense_source=catalog rides the upsert only together with its strategy
	// (validateUpsert enforces that); the column itself is written by the
	// catalog half of the write so the registry validation runs the same rules
	// as the dedicated catalog endpoint.
	catalogRequested := req.DenseSource != nil && *req.DenseSource == codohuetypes.DenseSourceCatalog
	var catalogReq *UpdateCatalogRequest
	if catalogRequested {
		stripped := *req
		stripped.DenseSource = nil
		req = &stripped
		catalogReq = &UpdateCatalogRequest{
			Enabled:         true,
			StrategyID:      *req.CatalogStrategyID,
			StrategyVersion: *req.CatalogStrategyVersion,
			Params:          req.CatalogStrategyParams,
		}
		// Validated against the dimension this request WILL leave behind, not
		// the one on disk: the body can change embedding_dim and select a
		// strategy in the same call. Validating first is what makes the write
		// atomic — a rejected request must not have created the namespace.
		if err := s.validateCatalogStrategy(ctx, ns, catalogReq, req.EmbeddingDim); err != nil {
			return nil, err
		}
	}

	cfg, err := s.repo.UpsertWithCatalog(ctx, ns, req, catalogReq)
	if err != nil {
		return nil, fmt.Errorf("upsert namespace config: %w", err)
	}

	resp := &UpsertResponse{
		Namespace:  cfg.Namespace,
		Generation: cfg.Generation,
		UpdatedAt:  cfg.UpdatedAt,
	}

	// If no API key exists for this namespace yet, generate one now.
	if cfg.APIKeyHash == "" {
		plaintext, hash, err := generateAPIKey()
		if err != nil {
			return nil, fmt.Errorf("generate api key: %w", err)
		}
		won, err := s.repo.SetAPIKeyHash(ctx, ns, hash)
		if err != nil {
			return nil, fmt.Errorf("store api key hash: %w", err)
		}
		// Only the writer that actually landed the hash may hand out its
		// plaintext. A concurrent first-time Upsert that lost the race used
		// to return a key that was never stored — credentials that 401
		// forever.
		if won {
			resp.APIKey = plaintext
		}
	}

	return resp, nil
}

// RotateAPIKey replaces the namespace's API key with a fresh one and returns
// the new plaintext (shown once, like creation). The old key stops working
// immediately — this is the escape hatch for a leaked or lost key, which
// previously required manual SQL or a full namespace wipe.
// Returns ErrNamespaceNotFound when the namespace does not exist.
func (s *Service) RotateAPIKey(ctx context.Context, ns string) (*RotateAPIKeyResponse, error) {
	if s.lifecycle != nil && nslifecycle.RequireNamespaceLease(ctx, ns) != nil {
		var response *RotateAPIKeyResponse
		err := s.lifecycle.WithWriter(ctx, ns, func(leased context.Context, _ *nslifecycle.NamespaceLifecycle) error {
			var rotateErr error
			response, rotateErr = s.rotateAPIKeyActive(leased, ns)
			return rotateErr
		})
		return response, err
	}
	return s.rotateAPIKeyActive(ctx, ns)
}

func (s *Service) rotateAPIKeyActive(ctx context.Context, ns string) (*RotateAPIKeyResponse, error) {
	plaintext, hash, err := generateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("generate api key: %w", err)
	}
	found, err := s.repo.ReplaceAPIKeyHash(ctx, ns, hash)
	if err != nil {
		return nil, fmt.Errorf("replace api key hash: %w", err)
	}
	if !found {
		return nil, ErrNamespaceNotFound
	}
	return &RotateAPIKeyResponse{Namespace: ns, APIKey: plaintext}, nil
}

// validateCatalogStrategy builds the requested strategy and checks it against
// the namespace's *effective* embedding dimension — the one this request will
// leave behind, which is not necessarily the one currently stored.
//
// requestedDim is the embedding_dim from the same request body (nil when the
// caller is not changing it). Resolution order: the request, then the stored
// row, then the schema default for a namespace that does not exist yet.
// Disabling catalog skips validation entirely: the strategy fields are nulled.
func (s *Service) validateCatalogStrategy(ctx context.Context, ns string, req *UpdateCatalogRequest, requestedDim *int) error {
	if !req.Enabled {
		return nil
	}
	if req.StrategyID == "" || req.StrategyVersion == "" {
		return fmt.Errorf("strategy_id and strategy_version are required when enabling catalog")
	}
	strategy, err := s.registry.Build(req.StrategyID, req.StrategyVersion, embedstrategy.Params(req.Params))
	if err != nil {
		return err
	}

	effectiveDim := schemaEmbeddingDim
	switch {
	case requestedDim != nil && *requestedDim > 0:
		effectiveDim = *requestedDim
	default:
		current, err := s.repo.Get(ctx, ns)
		if err != nil {
			return fmt.Errorf("load namespace config: %w", err)
		}
		if current != nil && current.EmbeddingDim > 0 {
			effectiveDim = current.EmbeddingDim
		}
	}

	if strategy.Dim() != effectiveDim {
		return &DimensionMismatchError{
			StrategyDim:           strategy.Dim(),
			NamespaceEmbeddingDim: effectiveDim,
		}
	}
	return nil
}

// guardEmbeddingDimChange rejects an embedding_dim change while the
// namespace's dense collections exist. Without a wired checker (data plane,
// tests) the guard is skipped — creation-time defaults are unaffected.
func (s *Service) guardEmbeddingDimChange(ctx context.Context, ns string, req *UpsertRequest) error {
	if req == nil || req.EmbeddingDim == nil || s.denseCollections == nil {
		return nil
	}
	current, err := s.repo.Get(ctx, ns)
	if err != nil {
		return fmt.Errorf("load current config: %w", err)
	}
	if current == nil || current.EmbeddingDim == *req.EmbeddingDim {
		return nil // new namespace, or a no-op change
	}
	exists, err := s.denseCollections.DenseCollectionsExist(ctx, ns)
	if err != nil {
		return fmt.Errorf("check dense collections: %w", err)
	}
	if exists {
		return ErrEmbeddingDimLocked
	}
	return nil
}

// Get returns the configuration for a namespace, or nil if it does not exist.
func (s *Service) Get(ctx context.Context, ns string) (*namespace.Config, error) {
	cfg, err := s.repo.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get namespace config: %w", err)
	}
	return cfg, nil
}

// ListCatalogNamespaces returns every namespace whose dense_source is
// "catalog". Used by the embedder binary's namespace poller.
func (s *Service) ListCatalogNamespaces(ctx context.Context) ([]*namespace.Config, error) {
	cfgs, err := s.repo.ListCatalogNamespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list catalog namespaces: %w", err)
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
	// Upsert already holds the lease when it calls this for a one-request
	// catalog provisioning, so the guard below reuses it instead of
	// re-acquiring; the standalone admin catalog endpoint takes its own.
	if s.lifecycle != nil && nslifecycle.RequireNamespaceLease(ctx, ns) != nil {
		var updated *namespace.Config
		err := s.lifecycle.WithWriter(ctx, ns, func(leased context.Context, _ *nslifecycle.NamespaceLifecycle) error {
			var updateErr error
			updated, updateErr = s.updateCatalogConfigActive(leased, ns, req)
			return updateErr
		})
		return updated, err
	}
	return s.updateCatalogConfigActive(ctx, ns, req)
}

func (s *Service) updateCatalogConfigActive(ctx context.Context, ns string, req *UpdateCatalogRequest) (*namespace.Config, error) {
	cfg, err := s.repo.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("load namespace config: %w", err)
	}
	if cfg == nil {
		return nil, ErrNamespaceNotFound
	}

	if err := s.validateCatalogStrategy(ctx, ns, req, nil); err != nil {
		return nil, err
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
