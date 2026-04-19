package nsconfig

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	u := os.Getenv("DATABASE_URL")
	if u == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := pgxpool.New(context.Background(), u)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func cleanupNS(t *testing.T, db *pgxpool.Pool, ns string) {
	t.Helper()
	t.Cleanup(func() {
		db.Exec(context.Background(), //nolint:errcheck // test cleanup, failure is not critical
			`DELETE FROM namespace_configs WHERE namespace = $1`, ns)
	})
}

func TestRepositoryUpsert_Create(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "nsconfig_test")

	repo := NewRepository(db)
	req := &UpsertRequest{
		ActionWeights: map[string]float64{"VIEW": 1.0, "LIKE": 5.0},
		Lambda:        0.05,
		Gamma:         0.02,
		MaxResults:    20,
		DenseStrategy: "disabled",
	}

	cfg, err := repo.Upsert(context.Background(), "nsconfig_test", req)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if cfg.Namespace != "nsconfig_test" {
		t.Errorf("Namespace: got %q, want %q", cfg.Namespace, "nsconfig_test")
	}
	if cfg.Lambda != req.Lambda {
		t.Errorf("Lambda: got %v, want %v", cfg.Lambda, req.Lambda)
	}
	if cfg.ActionWeights["LIKE"] != 5.0 {
		t.Errorf("ActionWeights[LIKE]: got %.1f, want 5.0", cfg.ActionWeights["LIKE"])
	}
}

func TestRepositoryUpsert_Update_PreservesAPIKeyHash(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "nsconfig_test_preserve")
	ctx := context.Background()

	repo := NewRepository(db)

	// First upsert — no hash yet.
	cfg, err := repo.Upsert(ctx, "nsconfig_test_preserve", &UpsertRequest{Lambda: 0.01})
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	if cfg.APIKeyHash != "" {
		t.Fatalf("expected no APIKeyHash after initial upsert")
	}

	// Write a hash.
	if err := repo.SetAPIKeyHash(ctx, "nsconfig_test_preserve", "fakehash"); err != nil {
		t.Fatalf("SetAPIKeyHash: %v", err)
	}

	// Second upsert — hash must survive.
	cfg2, err := repo.Upsert(ctx, "nsconfig_test_preserve", &UpsertRequest{Lambda: 0.99})
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	if cfg2.APIKeyHash != "fakehash" {
		t.Errorf("APIKeyHash: got %q, want %q", cfg2.APIKeyHash, "fakehash")
	}
	if cfg2.Lambda != 0.99 {
		t.Errorf("Lambda: got %v, want 0.99", cfg2.Lambda)
	}
}

func TestRepositoryGet_ReturnsConfig(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "nsconfig_test_get")
	ctx := context.Background()

	repo := NewRepository(db)
	req := &UpsertRequest{Lambda: 0.07, MaxResults: 50}
	if _, err := repo.Upsert(ctx, "nsconfig_test_get", req); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	cfg, err := repo.Get(ctx, "nsconfig_test_get")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.Lambda != req.Lambda {
		t.Errorf("Lambda: got %v, want %v", cfg.Lambda, req.Lambda)
	}
	if cfg.MaxResults != req.MaxResults {
		t.Errorf("MaxResults: got %d, want %d", cfg.MaxResults, req.MaxResults)
	}
}

func TestRepositoryGet_UnknownNamespace_ReturnsNil(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)

	cfg, err := repo.Get(context.Background(), "does_not_exist_xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil for unknown namespace, got %+v", cfg)
	}
}

func TestRepositorySetAPIKeyHash_IsNoOpIfAlreadySet(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "nsconfig_test_hash")
	ctx := context.Background()

	repo := NewRepository(db)
	if _, err := repo.Upsert(ctx, "nsconfig_test_hash", &UpsertRequest{}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	if err := repo.SetAPIKeyHash(ctx, "nsconfig_test_hash", "first-hash"); err != nil {
		t.Fatalf("SetAPIKeyHash first: %v", err)
	}
	// Second call must not overwrite — WHERE api_key_hash IS NULL guard.
	if err := repo.SetAPIKeyHash(ctx, "nsconfig_test_hash", "second-hash"); err != nil {
		t.Fatalf("SetAPIKeyHash second: %v", err)
	}

	cfg, err := repo.Get(ctx, "nsconfig_test_hash")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cfg.APIKeyHash != "first-hash" {
		t.Errorf("APIKeyHash: got %q, want %q", cfg.APIKeyHash, "first-hash")
	}
}
