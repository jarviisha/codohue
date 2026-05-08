package nsconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

// fakeRepo implements nsConfigRepository for testing.
type fakeRepo struct {
	upsertCfg           *namespace.Config
	upsertErr           error
	setAPIKeyHashErr    error
	setAPIKeyHashCalled bool
	getCfg              *namespace.Config
	getErr              error
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

func TestNewService(t *testing.T) {
	repo := &Repository{}
	svc := NewService(repo)
	if svc == nil || svc.repo != repo {
		t.Fatal("expected NewService to wire repository")
	}
}

func TestServiceUpsert_NewNamespace_ReturnsAPIKey(t *testing.T) {
	repo := &fakeRepo{
		upsertCfg: &namespace.Config{Namespace: "ns", APIKeyHash: ""},
	}
	svc := &Service{repo: repo}

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
	svc := &Service{repo: repo}

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
	svc := &Service{repo: repo}

	if _, err := svc.Upsert(context.Background(), "ns", &UpsertRequest{}); err == nil {
		t.Error("expected error from repo.Upsert, got nil")
	}
}

func TestServiceUpsert_SetAPIKeyHashError(t *testing.T) {
	repo := &fakeRepo{
		upsertCfg:        &namespace.Config{Namespace: "ns", APIKeyHash: ""},
		setAPIKeyHashErr: errors.New("db error"),
	}
	svc := &Service{repo: repo}

	if _, err := svc.Upsert(context.Background(), "ns", &UpsertRequest{}); err == nil {
		t.Error("expected error from SetAPIKeyHash, got nil")
	}
}

func TestServiceGet_ReturnsConfig(t *testing.T) {
	want := &namespace.Config{Namespace: "ns", Lambda: 0.05, MaxResults: 20}
	svc := &Service{repo: &fakeRepo{getCfg: want}}

	got, err := svc.Get(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestServiceGet_UnknownNamespace_ReturnsNil(t *testing.T) {
	svc := &Service{repo: &fakeRepo{getCfg: nil}}

	got, err := svc.Get(context.Background(), "unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown namespace, got %v", got)
	}
}

func TestServiceGet_RepoError(t *testing.T) {
	svc := &Service{repo: &fakeRepo{getErr: errors.New("db error")}}

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
