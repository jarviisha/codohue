package nsconfig

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/jarviisha/codohue/internal/core/namespace"
	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 10

type nsConfigRepository interface {
	Upsert(ctx context.Context, namespace string, req *UpsertRequest) (*namespace.Config, error)
	SetAPIKeyHash(ctx context.Context, namespace, hash string) error
	Get(ctx context.Context, namespace string) (*namespace.Config, error)
}

// Service provides business logic for managing namespace configuration.
type Service struct {
	repo nsConfigRepository
}

// NewService creates a new Service with the given repository.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// Upsert creates or updates the configuration for a namespace.
// On first creation, a namespace-scoped API key is generated and returned as
// plaintext in UpsertResponse.APIKey. The plaintext key is shown once only —
// subsequent updates will not return the key.
func (s *Service) Upsert(ctx context.Context, namespace string, req *UpsertRequest) (*UpsertResponse, error) {
	cfg, err := s.repo.Upsert(ctx, namespace, req)
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
		if err := s.repo.SetAPIKeyHash(ctx, namespace, hash); err != nil {
			return nil, fmt.Errorf("store api key hash: %w", err)
		}
		resp.APIKey = plaintext
	}

	return resp, nil
}

// Get returns the configuration for a namespace, or nil if it does not exist.
func (s *Service) Get(ctx context.Context, namespace string) (*namespace.Config, error) {
	cfg, err := s.repo.Get(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get namespace config: %w", err)
	}
	return cfg, nil
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
