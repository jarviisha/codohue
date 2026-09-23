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
	var lookedUp []string
	repo := &Repository{
		queryFn: func(_ context.Context, _ string, args ...any) (rowsIterator, error) {
			lookedUp = args[2].([]string)
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
	if len(lookedUp) != 2 {
		t.Fatalf("input must be deduped to 2, got %v", lookedUp)
	}
}

// Every id already mapped must cost one SELECT and no write at all: the
// INSERT ... ON CONFLICT DO UPDATE this replaced burned a sequence value and
// left a dead tuple per already-mapped id, 80x the real row count in practice.
func TestRepositoryGetOrCreateBatch_ExistingIDsIssueNoWrite(t *testing.T) {
	var statements []string
	repo := &Repository{
		queryFn: func(_ context.Context, sql string, _ ...any) (rowsIterator, error) {
			statements = append(statements, sql)
			return &fakeRows{rows: [][]any{{"a", int64(1)}}}, nil
		},
	}
	if _, err := repo.GetOrCreateBatch(context.Background(), []string{"a"}, "ns", "object"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statements) != 1 {
		t.Fatalf("hit must issue exactly one statement, got %d: %v", len(statements), statements)
	}
	if strings.Contains(statements[0], "INSERT") {
		t.Fatalf("hit must not write: %s", statements[0])
	}
}

func TestRepositoryGetOrCreateBatch_InsertsOnlyMissingIDs(t *testing.T) {
	var insertArgs []string
	repo := &Repository{
		queryFn: func(_ context.Context, sql string, args ...any) (rowsIterator, error) {
			if strings.Contains(sql, "INSERT") {
				insertArgs = args[0].([]string)
				return &fakeRows{rows: [][]any{{"b", int64(2)}}}, nil
			}
			return &fakeRows{rows: [][]any{{"a", int64(1)}}}, nil
		},
	}
	out, err := repo.GetOrCreateBatch(context.Background(), []string{"a", "b"}, "ns", "object")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["a"] != 1 || out["b"] != 2 {
		t.Fatalf("result map wrong: %v", out)
	}
	if len(insertArgs) != 1 || insertArgs[0] != "b" {
		t.Fatalf("insert must carry only the missing id, got %v", insertArgs)
	}
}

// A conflicting insert returns no row under DO NOTHING; the id must still
// resolve via the follow-up read rather than vanish from the result map.
func TestRepositoryGetOrCreateBatch_ResolvesRacedInsert(t *testing.T) {
	lookups := 0
	repo := &Repository{
		queryFn: func(_ context.Context, sql string, _ ...any) (rowsIterator, error) {
			if strings.Contains(sql, "INSERT") {
				return &fakeRows{rows: [][]any{}}, nil
			}
			lookups++
			if lookups == 1 {
				return &fakeRows{rows: [][]any{}}, nil
			}
			return &fakeRows{rows: [][]any{{"a", int64(9)}}}, nil
		},
	}
	out, err := repo.GetOrCreateBatch(context.Background(), []string{"a"}, "ns", "object")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["a"] != 9 {
		t.Fatalf("raced id must resolve to the winner's mapping, got %v", out)
	}
}

func TestRepositoryGetOrCreate_ExistingIDIssuesNoWrite(t *testing.T) {
	var statements []string
	repo := &Repository{
		queryRowFn: func(_ context.Context, sql string, _ ...any) rowScanner {
			statements = append(statements, sql)
			return fakeRow{scanFn: func(dest ...any) error { *dest[0].(*int64) = 42; return nil }}
		},
	}
	id, err := repo.GetOrCreate(context.Background(), "obj-1", "ns", "object")
	if err != nil || id != 42 {
		t.Fatalf("GetOrCreate id=%d err=%v", id, err)
	}
	if len(statements) != 1 {
		t.Fatalf("hit must issue exactly one statement, got %d: %v", len(statements), statements)
	}
	if strings.Contains(statements[0], "INSERT") {
		t.Fatalf("hit must not write: %s", statements[0])
	}
}

func TestRepositoryGetOrCreate_ResolvesRacedInsert(t *testing.T) {
	calls := 0
	repo := &Repository{
		queryRowFn: func(_ context.Context, sql string, _ ...any) rowScanner {
			calls++
			switch {
			case strings.Contains(sql, "INSERT"):
				return fakeRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
			case calls == 1:
				return fakeRow{scanFn: func(_ ...any) error { return pgx.ErrNoRows }}
			default:
				return fakeRow{scanFn: func(dest ...any) error { *dest[0].(*int64) = 9; return nil }}
			}
		},
	}
	id, err := repo.GetOrCreate(context.Background(), "obj-1", "ns", "object")
	if err != nil || id != 9 {
		t.Fatalf("raced GetOrCreate id=%d err=%v", id, err)
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
