package recommend

import (
	"context"
	"errors"
	"testing"
)

type fakeRows struct {
	items   [][]any
	idx     int
	scanErr error
	rowsErr error
	closed  bool
}

func (f *fakeRows) Next() bool {
	return f.idx < len(f.items)
}

func (f *fakeRows) Scan(dest ...any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	row := f.items[f.idx]
	f.idx++
	for i := range dest {
		d, ok := dest[i].(*string)
		if !ok {
			return errors.New("expected *string")
		}
		v, ok := row[i].(string)
		if !ok {
			return errors.New("expected string")
		}
		*d = v
	}
	return nil
}

func (f *fakeRows) Err() error { return f.rowsErr }
func (f *fakeRows) Close()     { f.closed = true }

type fakeRow struct {
	scanFn func(dest ...any) error
}

func (f fakeRow) Scan(dest ...any) error { return f.scanFn(dest...) }

func TestNewRepository(t *testing.T) {
	repo := NewRepository(nil)
	if repo == nil {
		t.Fatal("expected repository")
	}
}

func TestRepositoryGetSeenItems_QueryError(t *testing.T) {
	repo := &Repository{
		queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) {
			return nil, errors.New("query failed")
		},
	}
	if _, err := repo.GetSeenItems(context.Background(), "ns", "u1", 30); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetSeenItems_ScanError(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"obj-1"}}, scanErr: errors.New("scan failed")}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	if _, err := repo.GetSeenItems(context.Background(), "ns", "u1", 30); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetSeenItems_RowsError(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"obj-1"}}, rowsErr: errors.New("rows failed")}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	if _, err := repo.GetSeenItems(context.Background(), "ns", "u1", 30); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetSeenItems_Success(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"obj-1"}, {"obj-2"}}}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	items, err := repo.GetSeenItems(context.Background(), "ns", "u1", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 || items[0] != "obj-1" || items[1] != "obj-2" {
		t.Fatalf("unexpected items: %v", items)
	}
	if !rows.closed {
		t.Fatal("expected rows to be closed")
	}
}

func TestRepositoryGetPopularItems_QueryError(t *testing.T) {
	repo := &Repository{
		queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) {
			return nil, errors.New("query failed")
		},
	}
	if _, err := repo.GetPopularItems(context.Background(), "ns", 10); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetPopularItems_ScanError(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"obj-1"}}, scanErr: errors.New("scan failed")}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	if _, err := repo.GetPopularItems(context.Background(), "ns", 10); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetPopularItems_RowsError(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"obj-1"}}, rowsErr: errors.New("rows failed")}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	if _, err := repo.GetPopularItems(context.Background(), "ns", 10); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetPopularItems_Success(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"obj-1"}, {"obj-2"}}}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	items, err := repo.GetPopularItems(context.Background(), "ns", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("unexpected items: %v", items)
	}
}

func TestRepositoryCountInteractions_QueryError(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(_ ...any) error { return errors.New("scan failed") }}
		},
	}
	if _, err := repo.CountInteractions(context.Background(), "ns", "u1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryCountInteractions_Success(t *testing.T) {
	repo := &Repository{
		queryRowFn: func(_ context.Context, _ string, _ ...any) rowScanner {
			return fakeRow{scanFn: func(dest ...any) error {
				ptr, ok := dest[0].(*int)
				if !ok {
					return errors.New("expected *int")
				}
				*ptr = 7
				return nil
			}}
		},
	}
	count, err := repo.CountInteractions(context.Background(), "ns", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 7 {
		t.Fatalf("count: got %d want 7", count)
	}
}
