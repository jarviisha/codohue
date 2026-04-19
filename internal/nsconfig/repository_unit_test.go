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

func setTime(dest any, value time.Time) error {
	ptr, ok := dest.(*time.Time)
	if !ok {
		return errors.New("expected *time.Time")
	}
	*ptr = value
	return nil
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

func TestRepositoryUpsert_UnmarshalError(t *testing.T) {
	now := time.Now()
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				if err := setString(dest[0], "ns"); err != nil {
					return err
				}
				if err := setBytes(dest[1], []byte("not-json")); err != nil {
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
				if err := setTime(dest[14], now); err != nil {
					return err
				}
				return setTime(dest[15], now)
			}}
		},
	}

	_, err := repo.Upsert(context.Background(), "ns", &UpsertRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
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

func TestRepositoryGet_UnmarshalError(t *testing.T) {
	now := time.Now()
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				if err := setString(dest[0], "ns"); err != nil {
					return err
				}
				if err := setBytes(dest[1], []byte("not-json")); err != nil {
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
				if err := setTime(dest[14], now); err != nil {
					return err
				}
				return setTime(dest[15], now)
			}}
		},
	}

	_, err := repo.Get(context.Background(), "ns")
	if err == nil {
		t.Fatal("expected error, got nil")
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
