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
		Lambda:        ptr(0.05),
		Gamma:         ptr(0.02),
		MaxResults:    ptr(20),
		DenseSource:   ptr("disabled"),
	}

	cfg, err := repo.Upsert(context.Background(), "nsconfig_test", req)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if cfg.Namespace != "nsconfig_test" {
		t.Errorf("Namespace: got %q, want %q", cfg.Namespace, "nsconfig_test")
	}
	if cfg.Lambda != *req.Lambda {
		t.Errorf("Lambda: got %v, want %v", cfg.Lambda, *req.Lambda)
	}
	if cfg.ActionWeights["LIKE"] != 5.0 {
		t.Errorf("ActionWeights[LIKE]: got %.1f, want 5.0", cfg.ActionWeights["LIKE"])
	}
}

// TestRepositoryUpsert_EmptyDenseSource_NormalizedToDisabled locks the
// normalization rule: an omitted dense_source ("") must be persisted as
// "disabled" (same mapping as the migration-016 backfill), never tripping
// the dense_source_chk constraint.
func TestRepositoryUpsert_EmptyDenseSource_NormalizedToDisabled(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "nsconfig_test_empty_source")

	repo := NewRepository(db)
	cfg, err := repo.Upsert(context.Background(), "nsconfig_test_empty_source", &UpsertRequest{})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if cfg.DenseSource != "disabled" {
		t.Errorf("DenseSource: got %q, want %q", cfg.DenseSource, "disabled")
	}
}

// TestRepositoryUpsertCatalogConfig_DisableSemantics covers the disable path:
// a catalog-mode namespace lands on "disabled", while disabling against a
// namespace that never entered catalog mode leaves dense_source untouched.
func TestRepositoryUpsertCatalogConfig_DisableSemantics(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "nsconfig_test_disable")
	ctx := context.Background()

	repo := NewRepository(db)
	if _, err := repo.Upsert(ctx, "nsconfig_test_disable", &UpsertRequest{DenseSource: ptr("item2vec")}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	cfg, err := repo.UpsertCatalogConfig(ctx, "nsconfig_test_disable", &UpdateCatalogRequest{Enabled: false})
	if err != nil {
		t.Fatalf("UpsertCatalogConfig (non-catalog disable): %v", err)
	}
	if cfg.DenseSource != "item2vec" {
		t.Errorf("DenseSource after no-op disable: got %q, want %q", cfg.DenseSource, "item2vec")
	}

	if _, err := repo.UpsertCatalogConfig(ctx, "nsconfig_test_disable",
		&UpdateCatalogRequest{Enabled: true, StrategyID: "hash", StrategyVersion: "v1"}); err != nil {
		t.Fatalf("UpsertCatalogConfig (enable): %v", err)
	}
	cfg, err = repo.UpsertCatalogConfig(ctx, "nsconfig_test_disable", &UpdateCatalogRequest{Enabled: false})
	if err != nil {
		t.Fatalf("UpsertCatalogConfig (disable): %v", err)
	}
	if cfg.DenseSource != "disabled" {
		t.Errorf("DenseSource after disable: got %q, want %q", cfg.DenseSource, "disabled")
	}
}

func TestRepositoryUpsert_Update_PreservesAPIKeyHash(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "nsconfig_test_preserve")
	ctx := context.Background()

	repo := NewRepository(db)

	// First upsert — no hash yet.
	cfg, err := repo.Upsert(ctx, "nsconfig_test_preserve", &UpsertRequest{Lambda: ptr(0.01)})
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
	cfg2, err := repo.Upsert(ctx, "nsconfig_test_preserve", &UpsertRequest{Lambda: ptr(0.99)})
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
	req := &UpsertRequest{Lambda: ptr(0.07), MaxResults: ptr(50)}
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
	if cfg.Lambda != *req.Lambda {
		t.Errorf("Lambda: got %v, want %v", cfg.Lambda, *req.Lambda)
	}
	if cfg.MaxResults != *req.MaxResults {
		t.Errorf("MaxResults: got %d, want %d", cfg.MaxResults, *req.MaxResults)
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

// TestRepositoryListCatalogNamespaces covers the embedder-facing discovery
// query: only namespaces with dense_source='catalog' must appear, ordered by
// namespace ASC, and the catalog_strategy_params JSONB must round-trip.
func TestRepositoryListCatalogNamespaces(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	const (
		nsEnabledA = "nsconfig_list_catalog_a"
		nsEnabledB = "nsconfig_list_catalog_b"
		nsDisabled = "nsconfig_list_catalog_disabled"
	)
	for _, n := range []string{nsEnabledA, nsEnabledB, nsDisabled} {
		cleanupNS(t, db, n)
	}

	repo := NewRepository(db)
	for _, n := range []string{nsEnabledA, nsEnabledB, nsDisabled} {
		if _, err := repo.Upsert(ctx, n, &UpsertRequest{EmbeddingDim: ptr(128)}); err != nil {
			t.Fatalf("Upsert %q: %v", n, err)
		}
	}

	// Enable two namespaces with distinct strategy params.
	for _, p := range []struct {
		ns  string
		req *UpdateCatalogRequest
	}{
		{nsEnabledA, &UpdateCatalogRequest{
			Enabled: true, StrategyID: "internal-hashing-ngrams",
			StrategyVersion: "v1", Params: map[string]any{"dim": float64(128)},
			MaxAttempts: 5, MaxContentBytes: 32768,
		}},
		{nsEnabledB, &UpdateCatalogRequest{
			Enabled: true, StrategyID: "internal-hashing-ngrams",
			StrategyVersion: "v1", Params: map[string]any{"dim": float64(128)},
			MaxAttempts: 7, MaxContentBytes: 65536,
		}},
	} {
		if _, err := repo.UpsertCatalogConfig(ctx, p.ns, p.req); err != nil {
			t.Fatalf("UpsertCatalogConfig %q: %v", p.ns, err)
		}
	}

	got, err := repo.ListCatalogNamespaces(ctx)
	if err != nil {
		t.Fatalf("ListCatalogNamespaces: %v", err)
	}

	// We must see both enabled namespaces and not the disabled one. Filter the
	// result down to the namespaces this test owns so unrelated rows do not
	// flake the assertion.
	owned := make(map[string]bool, 2)
	for _, c := range got {
		switch c.Namespace {
		case nsEnabledA, nsEnabledB:
			owned[c.Namespace] = true
			if c.DenseSource != "catalog" {
				t.Errorf("ns %q: dense_source=%q want catalog", c.Namespace, c.DenseSource)
			}
			if c.CatalogStrategyID != "internal-hashing-ngrams" {
				t.Errorf("ns %q: strategy_id=%q", c.Namespace, c.CatalogStrategyID)
			}
			if v := c.CatalogStrategyParams["dim"]; v != float64(128) {
				t.Errorf("ns %q: params[dim]=%v want 128", c.Namespace, v)
			}
		case nsDisabled:
			t.Errorf("disabled namespace %q must NOT appear in ListCatalogNamespaces", c.Namespace)
		}
	}
	if !owned[nsEnabledA] || !owned[nsEnabledB] {
		t.Errorf("expected both enabled namespaces, owned=%v", owned)
	}

	// Order check: the two we own must be in ASC order relative to each
	// other (a before b).
	var posA, posB = -1, -1
	for i, c := range got {
		if c.Namespace == nsEnabledA {
			posA = i
		}
		if c.Namespace == nsEnabledB {
			posB = i
		}
	}
	if posA == -1 || posB == -1 || posA >= posB {
		t.Errorf("expected nsEnabledA (pos=%d) before nsEnabledB (pos=%d)", posA, posB)
	}
}

func TestRepositoryListCatalogNamespaces_NilDB(t *testing.T) {
	repo := &Repository{}
	if _, err := repo.ListCatalogNamespaces(context.Background()); err == nil {
		t.Fatal("expected error when db is nil")
	}
}

// Regression: a partial upsert used to reset every unmentioned column to its
// Go zero value, so editing one field in the admin UI wiped the rest.
func TestRepositoryUpsert_PartialLeavesOtherFieldsAlone(t *testing.T) {
	db := openTestDB(t)
	const ns = "nsconfig_test_partial"
	cleanupNS(t, db, ns)
	ctx := context.Background()

	repo := NewRepository(db)
	full := &UpsertRequest{
		ActionWeights:   map[string]float64{"VIEW": 1, "LIKE": 5},
		Lambda:          ptr(0.9),
		Gamma:           ptr(0.15),
		MaxResults:      ptr(42),
		SeenItemsDays:   ptr(30),
		ExcludeAuthored: ptr(true),
		Alpha:           ptr(0.65),
		DenseSource:     ptr("byoe"),
		EmbeddingDim:    ptr(128),
		DenseDistance:   ptr("cosine"),
		TrendingWindow:  ptr(72),
		TrendingTTL:     ptr(3600),
		LambdaTrending:  ptr(0.18),
	}
	if _, err := repo.Upsert(ctx, ns, full); err != nil {
		t.Fatalf("initial Upsert: %v", err)
	}

	// Exactly what the admin UI sends when only lambda was edited.
	got, err := repo.Upsert(ctx, ns, &UpsertRequest{Lambda: ptr(0.5)})
	if err != nil {
		t.Fatalf("partial Upsert: %v", err)
	}

	if got.Lambda != 0.5 {
		t.Errorf("Lambda: got %v, want the edited 0.5", got.Lambda)
	}
	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"Gamma", got.Gamma, 0.15},
		{"MaxResults", got.MaxResults, 42},
		{"SeenItemsDays", got.SeenItemsDays, 30},
		{"ExcludeAuthored", got.ExcludeAuthored, true},
		{"Alpha", got.Alpha, 0.65},
		{"DenseSource", got.DenseSource, "byoe"},
		{"EmbeddingDim", got.EmbeddingDim, 128},
		{"DenseDistance", got.DenseDistance, "cosine"},
		{"TrendingWindow", got.TrendingWindow, 72},
		{"TrendingTTL", got.TrendingTTL, 3600},
		{"LambdaTrending", got.LambdaTrending, 0.18},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s was clobbered by a partial upsert: got %v, want %v", c.field, c.got, c.want)
		}
	}
	if got.ActionWeights["LIKE"] != 5 {
		t.Errorf("ActionWeights clobbered: got %v", got.ActionWeights)
	}
}

// A brand-new namespace created from an empty request must land on the schema
// defaults, not on Go zero values.
func TestRepositoryUpsert_CreateAppliesSchemaDefaults(t *testing.T) {
	db := openTestDB(t)
	const ns = "nsconfig_test_defaults"
	cleanupNS(t, db, ns)

	cfg, err := NewRepository(db).Upsert(context.Background(), ns, &UpsertRequest{})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	checks := []struct {
		field string
		got   any
		want  any
	}{
		{"Lambda", cfg.Lambda, 0.95},
		{"Gamma", cfg.Gamma, 0.02},
		{"MaxResults", cfg.MaxResults, 50},
		{"SeenItemsDays", cfg.SeenItemsDays, 30},
		{"ExcludeAuthored", cfg.ExcludeAuthored, false},
		{"Alpha", cfg.Alpha, 0.7},
		{"EmbeddingDim", cfg.EmbeddingDim, 64},
		{"DenseDistance", cfg.DenseDistance, "cosine"},
		{"TrendingWindow", cfg.TrendingWindow, 24},
		{"TrendingTTL", cfg.TrendingTTL, 600},
		{"LambdaTrending", cfg.LambdaTrending, 0.1},
		// App-level default, deliberately not the schema's "item2vec".
		{"DenseSource", cfg.DenseSource, "disabled"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s default: got %v, want %v", c.field, c.got, c.want)
		}
	}
}
