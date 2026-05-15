package nsconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/namespace"
)

// fakeRepo implements nsConfigRepository for testing.
type fakeRepo struct {
	upsertCfg               *namespace.Config
	upsertErr               error
	setAPIKeyHashErr        error
	setAPIKeyHashCalled     bool
	getCfg                  *namespace.Config
	getErr                  error
	upsertCatalogCfg        *namespace.Config
	upsertCatalogErr        error
	upsertCatalogCalledWith *UpdateCatalogRequest
	listCfgs                []*namespace.Config
	listErr                 error
}

func (f *fakeRepo) Upsert(_ context.Context, _ string, _ *UpsertRequest) (*namespace.Config, error) {
	return f.upsertCfg, f.upsertErr
}

func (f *fakeRepo) SetAPIKeyHash(_ context.Context, _, _ string) error {
	f.setAPIKeyHashCalled = true
	return f.setAPIKeyHashErr
}

func (f *fakeRepo) Get(_ context.Context, _ string) (*namespace.Config, error) {
	return f.getCfg, f.getErr
}

func (f *fakeRepo) UpsertCatalogConfig(_ context.Context, _ string, req *UpdateCatalogRequest) (*namespace.Config, error) {
	f.upsertCatalogCalledWith = req
	return f.upsertCatalogCfg, f.upsertCatalogErr
}

func (f *fakeRepo) ListCatalogEnabled(_ context.Context) ([]*namespace.Config, error) {
	return f.listCfgs, f.listErr
}

// stubStrategyT mirrors the embedstrategy test stub but lives in this package
// so service_test.go has zero cross-package coupling beyond the embedstrategy
// public API.
type stubStrategyT struct {
	id, version string
	dim         int
}

func (s *stubStrategyT) ID() string         { return s.id }
func (s *stubStrategyT) Version() string    { return s.version }
func (s *stubStrategyT) Dim() int           { return s.dim }
func (s *stubStrategyT) MaxInputBytes() int { return 0 }
func (s *stubStrategyT) Embed(_ context.Context, _ string) ([]float32, error) {
	return make([]float32, s.dim), nil
}

func TestNewService(t *testing.T) {
	repo := &Repository{}
	svc := NewService(repo)
	if svc == nil || svc.repo != repo {
		t.Fatal("expected NewService to wire repository")
	}
	if svc.registry == nil {
		t.Fatal("expected registry default to be embedstrategy.DefaultRegistry()")
	}
}

func TestServiceUpsert_NewNamespace_ReturnsAPIKey(t *testing.T) {
	repo := &fakeRepo{
		upsertCfg: &namespace.Config{Namespace: "ns", APIKeyHash: ""},
	}
	svc := &Service{repo: repo, registry: embedstrategy.NewRegistry()}

	resp, err := svc.Upsert(context.Background(), "ns", &UpsertRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.APIKey == "" {
		t.Error("expected APIKey to be set on first upsert, got empty string")
	}
	if !repo.setAPIKeyHashCalled {
		t.Error("expected SetAPIKeyHash to be called")
	}
}

func TestServiceUpsert_ExistingNamespace_NoAPIKey(t *testing.T) {
	repo := &fakeRepo{
		upsertCfg: &namespace.Config{Namespace: "ns", APIKeyHash: "$2a$10$existinghash"},
	}
	svc := &Service{repo: repo, registry: embedstrategy.NewRegistry()}

	resp, err := svc.Upsert(context.Background(), "ns", &UpsertRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.APIKey != "" {
		t.Errorf("expected empty APIKey for existing namespace, got %q", resp.APIKey)
	}
	if repo.setAPIKeyHashCalled {
		t.Error("expected SetAPIKeyHash NOT to be called when hash already exists")
	}
}

func TestServiceUpsert_RepoError(t *testing.T) {
	repo := &fakeRepo{upsertErr: errors.New("db error")}
	svc := &Service{repo: repo, registry: embedstrategy.NewRegistry()}

	if _, err := svc.Upsert(context.Background(), "ns", &UpsertRequest{}); err == nil {
		t.Error("expected error from repo.Upsert, got nil")
	}
}

func TestServiceUpsert_SetAPIKeyHashError(t *testing.T) {
	repo := &fakeRepo{
		upsertCfg:        &namespace.Config{Namespace: "ns", APIKeyHash: ""},
		setAPIKeyHashErr: errors.New("db error"),
	}
	svc := &Service{repo: repo, registry: embedstrategy.NewRegistry()}

	if _, err := svc.Upsert(context.Background(), "ns", &UpsertRequest{}); err == nil {
		t.Error("expected error from SetAPIKeyHash, got nil")
	}
}

func TestServiceGet_ReturnsConfig(t *testing.T) {
	want := &namespace.Config{Namespace: "ns", Lambda: 0.05, MaxResults: 20}
	svc := &Service{repo: &fakeRepo{getCfg: want}, registry: embedstrategy.NewRegistry()}

	got, err := svc.Get(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestServiceGet_UnknownNamespace_ReturnsNil(t *testing.T) {
	svc := &Service{repo: &fakeRepo{getCfg: nil}, registry: embedstrategy.NewRegistry()}

	got, err := svc.Get(context.Background(), "unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown namespace, got %v", got)
	}
}

func TestServiceGet_RepoError(t *testing.T) {
	svc := &Service{repo: &fakeRepo{getErr: errors.New("db error")}, registry: embedstrategy.NewRegistry()}

	if _, err := svc.Get(context.Background(), "ns"); err == nil {
		t.Error("expected error from repo.Get, got nil")
	}
}

func TestGenerateAPIKey(t *testing.T) {
	plaintext, hash, err := generateAPIKey()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plaintext) != 64 {
		t.Fatalf("expected 64-char plaintext key, got %d", len(plaintext))
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
}

// --- UpdateCatalogConfig ---------------------------------------------------

func newServiceWithRegistry(repo nsConfigRepository) (*Service, *embedstrategy.Registry) {
	reg := embedstrategy.NewRegistry()
	svc := &Service{repo: repo, registry: reg}
	return svc, reg
}

func TestServiceUpdateCatalogConfig_NamespaceNotFound(t *testing.T) {
	repo := &fakeRepo{getCfg: nil}
	svc, _ := newServiceWithRegistry(repo)

	_, err := svc.UpdateCatalogConfig(context.Background(), "missing", &UpdateCatalogRequest{Enabled: false})
	if !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected ErrNamespaceNotFound, got %v", err)
	}
}

func TestServiceUpdateCatalogConfig_GetError(t *testing.T) {
	repo := &fakeRepo{getErr: errors.New("db down")}
	svc, _ := newServiceWithRegistry(repo)

	_, err := svc.UpdateCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{})
	if err == nil {
		t.Fatal("expected error from Get, got nil")
	}
}

func TestServiceUpdateCatalogConfig_EnableRequiresStrategy(t *testing.T) {
	repo := &fakeRepo{getCfg: &namespace.Config{Namespace: "ns", EmbeddingDim: 128, DenseStrategy: "byoe"}}
	svc, _ := newServiceWithRegistry(repo)

	_, err := svc.UpdateCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{Enabled: true})
	if err == nil {
		t.Fatal("expected error when enabling without strategy_id/version")
	}
}

func TestServiceUpdateCatalogConfig_UnknownStrategy(t *testing.T) {
	repo := &fakeRepo{getCfg: &namespace.Config{Namespace: "ns", EmbeddingDim: 128, DenseStrategy: "byoe"}}
	svc, _ := newServiceWithRegistry(repo)

	_, err := svc.UpdateCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{
		Enabled: true, StrategyID: "missing", StrategyVersion: "v1",
	})
	if !errors.Is(err, embedstrategy.ErrUnknownStrategy) {
		t.Fatalf("expected ErrUnknownStrategy, got %v", err)
	}
}

func TestServiceUpdateCatalogConfig_DimensionMismatch(t *testing.T) {
	repo := &fakeRepo{getCfg: &namespace.Config{Namespace: "ns", EmbeddingDim: 128, DenseStrategy: "byoe"}}
	svc, reg := newServiceWithRegistry(repo)
	reg.Register("hash", "v1", func(_ embedstrategy.Params) (embedstrategy.Strategy, error) {
		return &stubStrategyT{id: "hash", version: "v1", dim: 64}, nil
	})

	_, err := svc.UpdateCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{
		Enabled: true, StrategyID: "hash", StrategyVersion: "v1",
	})
	var dimErr *DimensionMismatchError
	if !errors.As(err, &dimErr) {
		t.Fatalf("expected *DimensionMismatchError, got %v", err)
	}
	if dimErr.StrategyDim != 64 || dimErr.NamespaceEmbeddingDim != 128 {
		t.Errorf("unexpected dims: %+v", dimErr)
	}
	if repo.upsertCatalogCalledWith != nil {
		t.Error("expected UpsertCatalogConfig NOT to be called on dim mismatch")
	}
}

func TestServiceUpdateCatalogConfig_EnableSuccess(t *testing.T) {
	want := &namespace.Config{Namespace: "ns", EmbeddingDim: 128, CatalogEnabled: true, CatalogStrategyID: "hash", CatalogStrategyVersion: "v1"}
	repo := &fakeRepo{
		getCfg:           &namespace.Config{Namespace: "ns", EmbeddingDim: 128, DenseStrategy: "byoe"},
		upsertCatalogCfg: want,
	}
	svc, reg := newServiceWithRegistry(repo)
	reg.Register("hash", "v1", func(_ embedstrategy.Params) (embedstrategy.Strategy, error) {
		return &stubStrategyT{id: "hash", version: "v1", dim: 128}, nil
	})

	got, err := svc.UpdateCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{
		Enabled: true, StrategyID: "hash", StrategyVersion: "v1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
	if repo.upsertCatalogCalledWith == nil {
		t.Fatal("expected UpsertCatalogConfig to be called")
	}
}

func TestServiceUpdateCatalogConfig_DisableSkipsValidation(t *testing.T) {
	repo := &fakeRepo{
		getCfg:           &namespace.Config{Namespace: "ns", EmbeddingDim: 128, CatalogEnabled: true},
		upsertCatalogCfg: &namespace.Config{Namespace: "ns", EmbeddingDim: 128, CatalogEnabled: false},
	}
	svc, _ := newServiceWithRegistry(repo)
	// No strategy registered — should still succeed because disable skips
	// strategy validation.

	cfg, err := svc.UpdateCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{Enabled: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CatalogEnabled {
		t.Error("expected CatalogEnabled = false after disable")
	}
}

func TestServiceUpdateCatalogConfig_RepoUpsertError(t *testing.T) {
	repo := &fakeRepo{
		getCfg:           &namespace.Config{Namespace: "ns", EmbeddingDim: 128, DenseStrategy: "byoe"},
		upsertCatalogErr: errors.New("db error"),
	}
	svc, reg := newServiceWithRegistry(repo)
	reg.Register("hash", "v1", func(_ embedstrategy.Params) (embedstrategy.Strategy, error) {
		return &stubStrategyT{id: "hash", version: "v1", dim: 128}, nil
	})

	_, err := svc.UpdateCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{
		Enabled: true, StrategyID: "hash", StrategyVersion: "v1",
	})
	if err == nil {
		t.Fatal("expected error from UpsertCatalogConfig, got nil")
	}
}

func TestServiceUpdateCatalogConfig_RepoUpsertReturnsNil(t *testing.T) {
	repo := &fakeRepo{
		getCfg:           &namespace.Config{Namespace: "ns", EmbeddingDim: 128, DenseStrategy: "byoe"},
		upsertCatalogCfg: nil, // simulates no rows updated despite Get success
	}
	svc, reg := newServiceWithRegistry(repo)
	reg.Register("hash", "v1", func(_ embedstrategy.Params) (embedstrategy.Strategy, error) {
		return &stubStrategyT{id: "hash", version: "v1", dim: 128}, nil
	})

	_, err := svc.UpdateCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{
		Enabled: true, StrategyID: "hash", StrategyVersion: "v1",
	})
	if !errors.Is(err, ErrNamespaceNotFound) {
		t.Fatalf("expected ErrNamespaceNotFound, got %v", err)
	}
}

func TestServiceUpdateCatalogConfig_DenseStrategyConflict(t *testing.T) {
	// Trying to enable catalog while dense_strategy is item2vec/svd must be
	// rejected — both pipelines would write to {ns}_objects_dense.
	cases := []string{"item2vec", "svd", "", "unknown_value"}
	for _, ds := range cases {
		t.Run(ds, func(t *testing.T) {
			repo := &fakeRepo{getCfg: &namespace.Config{
				Namespace:     "ns",
				EmbeddingDim:  128,
				DenseStrategy: ds,
			}}
			svc, _ := newServiceWithRegistry(repo)
			_, err := svc.UpdateCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{
				Enabled: true, StrategyID: "hash", StrategyVersion: "v1",
			})
			var conflictErr *DenseStrategyConflictError
			if !errors.As(err, &conflictErr) {
				t.Fatalf("expected *DenseStrategyConflictError, got %v", err)
			}
			if conflictErr.DenseStrategy != ds {
				t.Errorf("DenseStrategy = %q, want %q", conflictErr.DenseStrategy, ds)
			}
			if repo.upsertCatalogCalledWith != nil {
				t.Error("expected UpsertCatalogConfig NOT to be called on conflict")
			}
		})
	}
}

func TestServiceUpsert_BlocksWhenCatalogEnabledAndStrategyIsCronTrained(t *testing.T) {
	// Once catalog is on, the operator cannot flip dense_strategy back to a
	// value that has cron train Phase 2 — the embedder would lose to cron's
	// next tick.
	repo := &fakeRepo{getCfg: &namespace.Config{
		Namespace:      "ns",
		EmbeddingDim:   128,
		DenseStrategy:  "byoe",
		CatalogEnabled: true,
	}}
	svc, _ := newServiceWithRegistry(repo)
	_, err := svc.Upsert(context.Background(), "ns", &UpsertRequest{
		DenseStrategy: "item2vec",
	})
	var conflictErr *DenseStrategyConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected *DenseStrategyConflictError, got %v", err)
	}
	if !conflictErr.CatalogEnabled {
		t.Error("expected CatalogEnabled=true on the error")
	}
}

// --- ListCatalogEnabled ----------------------------------------------------

func TestServiceListCatalogEnabled_PassThrough(t *testing.T) {
	want := []*namespace.Config{
		{Namespace: "a", CatalogEnabled: true},
		{Namespace: "b", CatalogEnabled: true},
	}
	repo := &fakeRepo{listCfgs: want}
	svc, _ := newServiceWithRegistry(repo)

	got, err := svc.ListCatalogEnabled(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0].Namespace != "a" || got[1].Namespace != "b" {
		t.Errorf("unexpected list: %+v", got)
	}
}

func TestServiceListCatalogEnabled_RepoError(t *testing.T) {
	repo := &fakeRepo{listErr: errors.New("db down")}
	svc, _ := newServiceWithRegistry(repo)
	if _, err := svc.ListCatalogEnabled(context.Background()); err == nil {
		t.Fatal("expected error from repo.ListCatalogEnabled, got nil")
	}
}

func TestServiceListCatalogEnabled_EmptyResult(t *testing.T) {
	repo := &fakeRepo{listCfgs: nil}
	svc, _ := newServiceWithRegistry(repo)

	got, err := svc.ListCatalogEnabled(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil/empty when no enabled namespaces, got %+v", got)
	}
}
