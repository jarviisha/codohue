package idmap

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type fakeRow struct {
	scanFn func(dest ...any) error
}

func (f fakeRow) Scan(dest ...any) error {
	return f.scanFn(dest...)
}

type fakeRepo struct {
	id         uint64
	found      bool
	err        error
	lastString string
	lastNS     string
	lastType   string
}

func (f *fakeRepo) GetOrCreate(_ context.Context, stringID, namespace, entityType string) (uint64, error) {
	f.lastString = stringID
	f.lastNS = namespace
	f.lastType = entityType
	return f.id, f.err
}

func (f *fakeRepo) Lookup(_ context.Context, stringID, namespace, entityType string) (numericID uint64, found bool, err error) {
	f.lastString = stringID
	f.lastNS = namespace
	f.lastType = entityType
	return f.id, f.found, f.err
}

func (f *fakeRepo) GetOrCreateBatch(_ context.Context, stringIDs []string, namespace, entityType string) (map[string]uint64, error) {
	f.lastNS = namespace
	f.lastType = entityType
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]uint64, len(stringIDs))
	for i, id := range stringIDs {
		out[id] = f.id + uint64(i)
	}
	return out, nil
}

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

func TestNewService(t *testing.T) {
	repo := &Repository{}
	svc := NewService(repo)
	if svc == nil || svc.repo != repo {
		t.Fatal("expected service to wire repository")
	}
}

func TestServiceGetOrCreateSubjectID(t *testing.T) {
	repo := &fakeRepo{id: 11}
	svc := &Service{repo: repo}

	id, err := svc.GetOrCreateSubjectID(context.Background(), "user-1", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 11 {
		t.Fatalf("id: got %d want 11", id)
	}
	if repo.lastType != "subject" {
		t.Fatalf("entityType: got %q want %q", repo.lastType, "subject")
	}
}

func TestServiceGetOrCreateObjectID(t *testing.T) {
	repo := &fakeRepo{id: 22}
	svc := &Service{repo: repo}

	id, err := svc.GetOrCreateObjectID(context.Background(), "obj-1", "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 22 {
		t.Fatalf("id: got %d want 22", id)
	}
	if repo.lastType != "object" {
		t.Fatalf("entityType: got %q want %q", repo.lastType, "object")
	}
}

func TestServiceGetOrCreateSubjectID_Error(t *testing.T) {
	svc := &Service{repo: &fakeRepo{err: errors.New("repo failed")}}
	if _, err := svc.GetOrCreateSubjectID(context.Background(), "user-1", "ns"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestServiceGetOrCreateObjectID_Error(t *testing.T) {
	svc := &Service{repo: &fakeRepo{err: errors.New("repo failed")}}
	if _, err := svc.GetOrCreateObjectID(context.Background(), "obj-1", "ns"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// fakeRows drives the queryFn seam for GetOrCreateBatch tests.
type fakeRows struct {
	rows    [][]any // each row: [string_id(string), numeric_id(int64)]
	idx     int
	scanErr error
	rowsErr error
	closed  bool
}

func (f *fakeRows) Next() bool { return f.idx < len(f.rows) }
func (f *fakeRows) Scan(dest ...any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	row := f.rows[f.idx]
	f.idx++
	*dest[0].(*string) = row[0].(string)
	*dest[1].(*int64) = row[1].(int64)
	return nil
}
func (f *fakeRows) Err() error { return f.rowsErr }
func (f *fakeRows) Close()     { f.closed = true }

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

func TestServiceLookupObjectID(t *testing.T) {
	repo := &fakeRepo{id: 22, found: true}
	svc := &Service{repo: repo}
	id, found, err := svc.LookupObjectID(context.Background(), "obj-1", "ns")
	if err != nil || !found || id != 22 {
		t.Fatalf("LookupObjectID: id=%d found=%v err=%v", id, found, err)
	}
	if repo.lastType != "object" {
		t.Errorf("entityType: got %q, want object", repo.lastType)
	}
}

func TestServiceLookupObjectID_Error(t *testing.T) {
	svc := &Service{repo: &fakeRepo{err: errors.New("boom")}}
	if _, _, err := svc.LookupObjectID(context.Background(), "obj-1", "ns"); err == nil {
		t.Fatal("expected error")
	}
}

func TestServiceGetOrCreateObjectIDs(t *testing.T) {
	repo := &fakeRepo{id: 10}
	svc := &Service{repo: repo}
	out, err := svc.GetOrCreateObjectIDs(context.Background(), []string{"a", "b"}, "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 ids, got %v", out)
	}
	if repo.lastType != "object" {
		t.Errorf("entityType: got %q, want object", repo.lastType)
	}
}

func TestServiceGetOrCreateObjectIDs_Error(t *testing.T) {
	svc := &Service{repo: &fakeRepo{err: errors.New("boom")}}
	if _, err := svc.GetOrCreateObjectIDs(context.Background(), []string{"a"}, "ns"); err == nil {
		t.Fatal("expected error")
	}
}
