package nsconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

type fakeRow struct {
	scanFn func(dest ...any) error
}

func (f fakeRow) Scan(dest ...any) error {
	return f.scanFn(dest...)
}

func setString(dest any, value string) error {
	ptr, ok := dest.(*string)
	if !ok {
		return errors.New("expected *string")
	}
	*ptr = value
	return nil
}

func setBytes(dest any, value []byte) error {
	ptr, ok := dest.(*[]byte)
	if !ok {
		return errors.New("expected *[]byte")
	}
	*ptr = value
	return nil
}

func setFloat64(dest any, value float64) error {
	ptr, ok := dest.(*float64)
	if !ok {
		return errors.New("expected *float64")
	}
	*ptr = value
	return nil
}

func setInt(dest any, value int) error {
	ptr, ok := dest.(*int)
	if !ok {
		return errors.New("expected *int")
	}
	*ptr = value
	return nil
}

func setBool(dest any, value bool) error {
	ptr, ok := dest.(*bool)
	if !ok {
		return errors.New("expected *bool")
	}
	*ptr = value
	return nil
}

func setTime(dest any, value time.Time) error {
	ptr, ok := dest.(*time.Time)
	if !ok {
		return errors.New("expected *time.Time")
	}
	*ptr = value
	return nil
}

// fillScanRow populates the 22-field scan row used by Repository.Upsert,
// Repository.Get, and Repository.UpsertCatalogConfig. weightsRaw is the
// JSON-encoded action_weights bytes; paramsRaw is the JSON-encoded
// catalog_strategy_params bytes. Field positions match the scan order in
// repository.go; tests that need to inject a malformed value at a specific
// position can call this helper and then overwrite the field they care about.
func fillScanRow(dest []any, weightsRaw, paramsRaw []byte, now time.Time) error {
	if len(dest) < 22 {
		return errors.New("scan dest too short")
	}
	if err := setString(dest[0], "ns"); err != nil {
		return err
	}
	if err := setBytes(dest[1], weightsRaw); err != nil {
		return err
	}
	if err := setFloat64(dest[2], 0.05); err != nil {
		return err
	}
	if err := setFloat64(dest[3], 0.02); err != nil {
		return err
	}
	if err := setInt(dest[4], 20); err != nil {
		return err
	}
	if err := setInt(dest[5], 7); err != nil {
		return err
	}
	if err := setString(dest[6], ""); err != nil {
		return err
	}
	if err := setFloat64(dest[7], 0.7); err != nil {
		return err
	}
	if err := setString(dest[8], "disabled"); err != nil {
		return err
	}
	if err := setInt(dest[9], 64); err != nil {
		return err
	}
	if err := setString(dest[10], "cosine"); err != nil {
		return err
	}
	if err := setInt(dest[11], 24); err != nil {
		return err
	}
	if err := setInt(dest[12], 600); err != nil {
		return err
	}
	if err := setFloat64(dest[13], 0.1); err != nil {
		return err
	}
	if err := setBool(dest[14], false); err != nil {
		return err
	}
	if err := setString(dest[15], ""); err != nil {
		return err
	}
	if err := setString(dest[16], ""); err != nil {
		return err
	}
	if err := setBytes(dest[17], paramsRaw); err != nil {
		return err
	}
	if err := setInt(dest[18], 5); err != nil {
		return err
	}
	if err := setInt(dest[19], 32768); err != nil {
		return err
	}
	if err := setTime(dest[20], now); err != nil {
		return err
	}
	return setTime(dest[21], now)
}

func TestNewRepository(t *testing.T) {
	repo := NewRepository(nil)
	if repo == nil {
		t.Fatal("expected repository")
	}
}

func TestRepositoryUpsert_QueryError(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return errors.New("query failed") }}
		},
	}

	_, err := repo.Upsert(context.Background(), "ns", &UpsertRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryUpsert_UnmarshalActionWeightsError(t *testing.T) {
	now := time.Now()
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, []byte("not-json"), []byte("{}"), now)
			}}
		},
	}

	_, err := repo.Upsert(context.Background(), "ns", &UpsertRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryUpsert_UnmarshalCatalogParamsError(t *testing.T) {
	now := time.Now()
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, []byte("{}"), []byte("not-json"), now)
			}}
		},
	}

	_, err := repo.Upsert(context.Background(), "ns", &UpsertRequest{})
	if err == nil {
		t.Fatal("expected error on malformed catalog params, got nil")
	}
}

func TestRepositoryGet_NoRowsReturnsNil(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}

	cfg, err := repo.Get(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config, got %+v", cfg)
	}
}

func TestRepositoryGet_QueryError(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return errors.New("query failed") }}
		},
	}

	_, err := repo.Get(context.Background(), "ns")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGet_UnmarshalActionWeightsError(t *testing.T) {
	now := time.Now()
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, []byte("not-json"), []byte("{}"), now)
			}}
		},
	}

	_, err := repo.Get(context.Background(), "ns")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGet_UnmarshalCatalogParamsError(t *testing.T) {
	now := time.Now()
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, []byte("{}"), []byte("not-json"), now)
			}}
		},
	}

	_, err := repo.Get(context.Background(), "ns")
	if err == nil {
		t.Fatal("expected error on malformed catalog params, got nil")
	}
}

func TestRepositoryGet_PopulatesCatalogFields(t *testing.T) {
	now := time.Now()
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				if err := fillScanRow(dest, []byte("{}"), []byte(`{"dim":128}`), now); err != nil {
					return err
				}
				// Override the catalog defaults set by fillScanRow with an
				// enabled/v1 strategy so we can assert population.
				if err := setBool(dest[14], true); err != nil {
					return err
				}
				if err := setString(dest[15], "internal-hashing-ngrams"); err != nil {
					return err
				}
				return setString(dest[16], "v1")
			}}
		},
	}

	cfg, err := repo.Get(context.Background(), "ns")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !cfg.CatalogEnabled {
		t.Error("expected CatalogEnabled = true")
	}
	if cfg.CatalogStrategyID != "internal-hashing-ngrams" {
		t.Errorf("CatalogStrategyID: got %q, want %q", cfg.CatalogStrategyID, "internal-hashing-ngrams")
	}
	if cfg.CatalogStrategyVersion != "v1" {
		t.Errorf("CatalogStrategyVersion: got %q, want %q", cfg.CatalogStrategyVersion, "v1")
	}
	if cfg.CatalogStrategyParams["dim"] != float64(128) { // JSON numbers decode to float64
		t.Errorf("CatalogStrategyParams[dim]: got %v, want 128", cfg.CatalogStrategyParams["dim"])
	}
	if cfg.CatalogMaxAttempts != 5 {
		t.Errorf("CatalogMaxAttempts: got %d, want 5", cfg.CatalogMaxAttempts)
	}
	if cfg.CatalogMaxContentBytes != 32768 {
		t.Errorf("CatalogMaxContentBytes: got %d, want 32768", cfg.CatalogMaxContentBytes)
	}
}

func TestRepositoryUpsertCatalogConfig_QueryError(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return errors.New("query failed") }}
		},
	}
	_, err := repo.UpsertCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{Enabled: true, StrategyID: "x", StrategyVersion: "v1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryUpsertCatalogConfig_NoRowsReturnsNil(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	cfg, err := repo.UpsertCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{Enabled: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatalf("expected nil config when row missing, got %+v", cfg)
	}
}

func TestRepositoryUpsertCatalogConfig_AppliesDefaults(t *testing.T) {
	now := time.Now()
	var capturedArgs []any
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, args ...any) rowScanner {
			capturedArgs = args
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, []byte("{}"), []byte("{}"), now)
			}}
		},
	}
	_, err := repo.UpsertCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{Enabled: true, StrategyID: "x", StrategyVersion: "v1"})
	if err != nil {
		t.Fatalf("UpsertCatalogConfig: %v", err)
	}
	// Args order: ns, enabled, strategy_id, strategy_version, params, max_attempts, max_content_bytes.
	if capturedArgs[5].(int) != 5 {
		t.Errorf("expected default max_attempts=5, got %v", capturedArgs[5])
	}
	if capturedArgs[6].(int) != 32768 {
		t.Errorf("expected default max_content_bytes=32768, got %v", capturedArgs[6])
	}
}

func TestRepositoryUpsertCatalogConfig_DisableNullsStrategy(t *testing.T) {
	now := time.Now()
	var capturedArgs []any
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, args ...any) rowScanner {
			capturedArgs = args
			return fakeRow{scanFn: func(dest ...any) error {
				return fillScanRow(dest, []byte("{}"), []byte("{}"), now)
			}}
		},
	}
	_, err := repo.UpsertCatalogConfig(context.Background(), "ns", &UpdateCatalogRequest{Enabled: false, StrategyID: "x", StrategyVersion: "v1"})
	if err != nil {
		t.Fatalf("UpsertCatalogConfig: %v", err)
	}
	if capturedArgs[2] != nil {
		t.Errorf("expected strategy_id to be nil when disabled, got %v", capturedArgs[2])
	}
	if capturedArgs[3] != nil {
		t.Errorf("expected strategy_version to be nil when disabled, got %v", capturedArgs[3])
	}
}

func TestRepositorySetAPIKeyHash_ExecError(t *testing.T) {
	repo := &Repository{
		execFn: func(_ context.Context, _ string, _ ...any) error {
			return errors.New("exec failed")
		},
	}

	err := repo.SetAPIKeyHash(context.Background(), "ns", "hash")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
