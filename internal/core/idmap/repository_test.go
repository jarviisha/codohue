package idmap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
)

// Repository tests. The shared fakes live in idmap_test.go.

func TestNewRepository(t *testing.T) {
	repo := NewRepository(nil)
	if repo == nil {
		t.Fatal("expected repository")
	}
}

func TestRepositoryGetOrCreate_Success(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				ptr, ok := dest[0].(*int64)
				if !ok {
					return errors.New("expected *int64")
				}
				*ptr = 42
				return nil
			}}
		},
	}

	id, err := repo.GetOrCreate(context.Background(), "obj-1", "ns", "object")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Fatalf("id: got %d want 42", id)
	}
}

func TestRepositoryGetOrCreate_QueryError(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return errors.New("query failed") }}
		},
	}

	_, err := repo.GetOrCreate(context.Background(), "obj-1", "ns", "object")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetOrCreateRequiresLifecycleLeaseButLookupDoesNot(t *testing.T) {
	repo := &Repository{requireLease: true, queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
		return fakeRow{scanFn: func(dest ...any) error { *dest[0].(*int64) = 42; return nil }}
	}}
	if _, err := repo.GetOrCreate(context.Background(), "obj-1", "ns", "object"); !errors.Is(err, nslifecycle.ErrLeaseRequired) {
		t.Fatalf("mutation error = %v", err)
	}
	ctx := nslifecycle.ContextWithLease(context.Background(), "ns", 2, nslifecycle.LockShared)
	if id, err := repo.GetOrCreate(ctx, "obj-1", "ns", "object"); err != nil || id != 42 {
		t.Fatalf("leased mutation id=%d err=%v", id, err)
	}
	if _, _, err := repo.Lookup(context.Background(), "obj-1", "ns", "object"); err != nil {
		t.Fatalf("read-only lookup required lease: %v", err)
	}
}

func TestRepositoryLookup_Found(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				*dest[0].(*int64) = 42
				return nil
			}}
		},
	}
	id, found, err := repo.Lookup(context.Background(), "obj-1", "ns", "object")
	if err != nil || !found || id != 42 {
		t.Fatalf("Lookup: id=%d found=%v err=%v, want 42/true/nil", id, found, err)
	}
}

func TestRepositoryLookup_NotFound(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
		},
	}
	_, found, err := repo.Lookup(context.Background(), "missing", "ns", "object")
	if err != nil || found {
		t.Fatalf("not-found must be (false, nil), got found=%v err=%v", found, err)
	}
}

func TestRepositoryLookup_QueryError(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return errors.New("db down") }}
		},
	}
	if _, _, err := repo.Lookup(context.Background(), "obj-1", "ns", "object"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRepositoryLookupBatchIsReadOnlyAndOmitsMissing(t *testing.T) {
	rows := &fakeRows{rows: [][]any{{"known", int64(7)}}}
	repo := &Repository{requireLease: true, queryFn: func(_ context.Context, sql string, _ ...any) (rowsIterator, error) {
		if !strings.Contains(sql, "SELECT string_id, numeric_id") || strings.Contains(sql, "INSERT") {
			t.Fatalf("lookup query is not read-only: %s", sql)
		}
		return rows, nil
	}}
	got, err := repo.LookupBatch(context.Background(), []string{"known", "missing"}, "ns", "object")
	if err != nil || got["known"] != 7 {
		t.Fatalf("LookupBatch=%v err=%v", got, err)
	}
	if _, ok := got["missing"]; ok {
		t.Fatal("missing mapping must be omitted")
	}
}

func TestRepositoryGetOrCreateBatch_Empty(t *testing.T) {
	repo := &Repository{}
	out, err := repo.GetOrCreateBatch(context.Background(), nil, "ns", "object")
	if err != nil || len(out) != 0 {
		t.Fatalf("empty input: got %v err %v", out, err)
	}
}

func TestRepositoryGetOrCreateBatch_DedupsAndMaps(t *testing.T) {
	var gotArgs []any
	repo := &Repository{
		queryFn: func(_ context.Context, _ string, args ...any) (rowsIterator, error) {
			gotArgs = args
			return &fakeRows{rows: [][]any{{"a", int64(1)}, {"b", int64(2)}}}, nil
		},
	}
	out, err := repo.GetOrCreateBatch(context.Background(), []string{"a", "b", "a"}, "ns", "object")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["a"] != 1 || out["b"] != 2 {
		t.Fatalf("result map wrong: %v", out)
	}
	// The duplicate "a" must be collapsed before unnest — ON CONFLICT DO UPDATE
	// errors on a repeated key in one statement.
	distinct := gotArgs[0].([]string)
	if len(distinct) != 2 {
		t.Fatalf("input must be deduped to 2, got %v", distinct)
	}
}

func TestRepositoryGetOrCreateBatch_QueryError(t *testing.T) {
	repo := &Repository{
		queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) {
			return nil, errors.New("db down")
		},
	}
	if _, err := repo.GetOrCreateBatch(context.Background(), []string{"a"}, "ns", "object"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRepositoryGetOrCreateBatch_ScanError(t *testing.T) {
	repo := &Repository{
		queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) {
			return &fakeRows{rows: [][]any{{"a", int64(1)}}, scanErr: errors.New("scan fail")}, nil
		},
	}
	if _, err := repo.GetOrCreateBatch(context.Background(), []string{"a"}, "ns", "object"); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestRepositoryGetOrCreateBatch_RowsError(t *testing.T) {
	repo := &Repository{
		queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) {
			return &fakeRows{rows: [][]any{}, rowsErr: errors.New("rows fail")}, nil
		},
	}
	if _, err := repo.GetOrCreateBatch(context.Background(), []string{"a"}, "ns", "object"); err == nil {
		t.Fatal("expected rows error")
	}
}
