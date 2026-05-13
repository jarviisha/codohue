package embedder

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

func (f fakeRow) Scan(dest ...any) error { return f.scanFn(dest...) }

func setInt64(dest any, v int64) error {
	ptr, ok := dest.(*int64)
	if !ok {
		return errors.New("expected *int64")
	}
	*ptr = v
	return nil
}

func setInt(dest any, v int) error {
	ptr, ok := dest.(*int)
	if !ok {
		return errors.New("expected *int")
	}
	*ptr = v
	return nil
}

func setString(dest any, v string) error {
	ptr, ok := dest.(*string)
	if !ok {
		return errors.New("expected *string")
	}
	*ptr = v
	return nil
}

func setBytes(dest any, v []byte) error {
	ptr, ok := dest.(*[]byte)
	if !ok {
		return errors.New("expected *[]byte")
	}
	*ptr = v
	return nil
}

func TestNewRepository(t *testing.T) {
	if NewRepository(nil) == nil {
		t.Fatal("expected repository")
	}
}

func TestRepositoryLoadByID_Success(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				if err := setInt64(dest[0], 7); err != nil {
					return err
				}
				if err := setString(dest[1], "ns"); err != nil {
					return err
				}
				if err := setString(dest[2], "obj1"); err != nil {
					return err
				}
				if err := setString(dest[3], "hello"); err != nil {
					return err
				}
				if err := setBytes(dest[4], []byte("hash")); err != nil {
					return err
				}
				if err := setString(dest[5], "internal-hashing-ngrams"); err != nil {
					return err
				}
				if err := setString(dest[6], "v1"); err != nil {
					return err
				}
				return setInt(dest[7], 2)
			}}
		},
	}
	item, err := repo.LoadByID(context.Background(), 7)
	if err != nil {
		t.Fatalf("LoadByID: %v", err)
	}
	if item.ID != 7 || item.Namespace != "ns" || item.ObjectID != "obj1" {
		t.Errorf("unexpected item: %+v", item)
	}
	if item.StrategyID != "internal-hashing-ngrams" || item.StrategyVersion != "v1" {
		t.Errorf("strategy fields: %s@%s", item.StrategyID, item.StrategyVersion)
	}
	if item.AttemptCount != 2 {
		t.Errorf("AttemptCount: got %d, want 2", item.AttemptCount)
	}
}

func TestRepositoryLoadByID_NotFound(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	_, err := repo.LoadByID(context.Background(), 7)
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}

func TestRepositoryLoadByID_QueryError(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return errors.New("db down") }}
		},
	}
	_, err := repo.LoadByID(context.Background(), 7)
	if err == nil || errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected wrapped DB error, got %v", err)
	}
}

func TestRepositoryMarkInFlight_Success(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				return setInt(dest[0], 3)
			}}
		},
	}
	got, err := repo.MarkInFlight(context.Background(), 7)
	if err != nil {
		t.Fatalf("MarkInFlight: %v", err)
	}
	if got != 3 {
		t.Errorf("attempt_count: got %d, want 3", got)
	}
}

func TestRepositoryMarkInFlight_NotFound(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	_, err := repo.MarkInFlight(context.Background(), 7)
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}

func TestRepositoryMarkEmbedded_Success(t *testing.T) {
	called := false
	repo := &Repository{
		execFn: func(_ context.Context, _ string, args ...any) (int64, error) {
			called = true
			if args[0].(int64) != 7 {
				return 0, errors.New("wrong id")
			}
			if args[1].(string) != "internal-hashing-ngrams" {
				return 0, errors.New("wrong strategy_id")
			}
			if args[2].(string) != "v1" {
				return 0, errors.New("wrong strategy_version")
			}
			return 1, nil
		},
	}
	err := repo.MarkEmbedded(context.Background(), 7, "internal-hashing-ngrams", "v1", time.Now())
	if err != nil {
		t.Fatalf("MarkEmbedded: %v", err)
	}
	if !called {
		t.Fatal("execFn not called")
	}
}

func TestRepositoryMarkEmbedded_NotFoundReturnsSentinel(t *testing.T) {
	repo := &Repository{
		execFn: func(_ context.Context, _ string, _ ...any) (int64, error) {
			return 0, nil
		},
	}
	err := repo.MarkEmbedded(context.Background(), 7, "x", "v1", time.Now())
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}

func TestRepositoryMarkFailed_PropagatesDBError(t *testing.T) {
	repo := &Repository{
		execFn: func(_ context.Context, _ string, _ ...any) (int64, error) {
			return 0, errors.New("db down")
		},
	}
	err := repo.MarkFailed(context.Background(), 7, "boom")
	if err == nil || errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected wrapped DB error, got %v", err)
	}
}

func TestRepositoryMarkDeadLetter_Success(t *testing.T) {
	repo := &Repository{
		execFn: func(_ context.Context, _ string, args ...any) (int64, error) {
			if args[1].(string) != "max attempts exhausted" {
				return 0, errors.New("wrong last_error")
			}
			return 1, nil
		},
	}
	err := repo.MarkDeadLetter(context.Background(), 7, "max attempts exhausted")
	if err != nil {
		t.Fatalf("MarkDeadLetter: %v", err)
	}
}

func TestRepositoryMarkDeadLetter_NotFoundReturnsSentinel(t *testing.T) {
	repo := &Repository{
		execFn: func(_ context.Context, _ string, _ ...any) (int64, error) {
			return 0, nil
		},
	}
	err := repo.MarkDeadLetter(context.Background(), 7, "boom")
	if !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("expected ErrItemNotFound, got %v", err)
	}
}
