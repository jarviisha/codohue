package compute

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

func (f *fakeRows) Next() bool { return f.idx < len(f.items) }

func (f *fakeRows) Scan(dest ...any) error {
	if f.scanErr != nil {
		return f.scanErr
	}
	row := f.items[f.idx]
	f.idx++
	for i := range dest {
		switch d := dest[i].(type) {
		case *string:
			v, ok := row[i].(string)
			if !ok {
				return errors.New("expected string")
			}
			*d = v
		case *float64:
			v, ok := row[i].(float64)
			if !ok {
				return errors.New("expected float64")
			}
			*d = v
		case *int64:
			val := row[i]
			switch v := val.(type) {
			case int64:
				*d = v
			case *int64:
				*d = *v
			case nil:
				*d = 0
			default:
				return errors.New("expected int64")
			}
		case **int64:
			val := row[i]
			if val == nil {
				*d = nil
			} else {
				v, ok := val.(int64)
				if !ok {
					return errors.New("expected int64 pointer payload")
				}
				*d = &v
			}
		}
	}
	return nil
}

func (f *fakeRows) Err() error { return f.rowsErr }
func (f *fakeRows) Close()     { f.closed = true }

func TestNewRepository(t *testing.T) {
	repo := NewRepository(nil)
	if repo == nil {
		t.Fatal("expected repository")
	}
}

func TestRepositoryGetActiveSubjects_QueryError(t *testing.T) {
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) {
		return nil, errors.New("query failed")
	}}
	if _, err := repo.GetActiveSubjects(context.Background(), "ns"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetActiveSubjects_ScanError(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"u1"}}, scanErr: errors.New("scan failed")}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	if _, err := repo.GetActiveSubjects(context.Background(), "ns"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetActiveSubjects_RowsError(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"u1"}}, rowsErr: errors.New("rows failed")}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	if _, err := repo.GetActiveSubjects(context.Background(), "ns"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetActiveSubjects_Success(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"u1"}, {"u2"}}}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	got, err := repo.GetActiveSubjects(context.Background(), "ns")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "u1" || got[1] != "u2" {
		t.Fatalf("unexpected subjects: %v", got)
	}
}

func TestRepositoryGetSubjectEvents_QueryError(t *testing.T) {
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) {
		return nil, errors.New("query failed")
	}}
	if _, err := repo.GetSubjectEvents(context.Background(), "ns", "u1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetSubjectEvents_ScanError(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"u1", "o1", "VIEW", 1.0, int64(10), nil}}, scanErr: errors.New("scan failed")}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	if _, err := repo.GetSubjectEvents(context.Background(), "ns", "u1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryGetSubjectEvents_Success(t *testing.T) {
	created := int64(5)
	rows := &fakeRows{items: [][]any{{"u1", "o1", "VIEW", 1.0, int64(10), created}}}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	events, err := repo.GetSubjectEvents(context.Background(), "ns", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 || events[0].SubjectID != "u1" || events[0].ObjectID != "o1" {
		t.Fatalf("unexpected events: %+v", events)
	}
	if events[0].ObjectCreatedAt == nil || *events[0].ObjectCreatedAt != created {
		t.Fatalf("unexpected created_at: %+v", events[0].ObjectCreatedAt)
	}
}

func TestRepositoryGetAllNamespaceEvents_Success(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"u1", "o1", "VIEW", 1.0, int64(10), nil}}}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	events, err := repo.GetAllNamespaceEvents(context.Background(), "ns")
	if err != nil || len(events) != 1 {
		t.Fatalf("unexpected result events=%+v err=%v", events, err)
	}
}

func TestRepositoryGetNamespaceEventsInWindow_Success(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"u1", "o1", "VIEW", 1.0, int64(10), nil}}}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	events, err := repo.GetNamespaceEventsInWindow(context.Background(), "ns", 24)
	if err != nil || len(events) != 1 {
		t.Fatalf("unexpected result events=%+v err=%v", events, err)
	}
}

func TestRepositoryGetActiveNamespaces_Success(t *testing.T) {
	rows := &fakeRows{items: [][]any{{"ns1"}, {"ns2"}}}
	repo := &Repository{queryFn: func(_ context.Context, _ string, _ ...any) (rowsIterator, error) { return rows, nil }}
	namespaces, err := repo.GetActiveNamespaces(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(namespaces) != 2 || namespaces[0] != "ns1" || namespaces[1] != "ns2" {
		t.Fatalf("unexpected namespaces: %v", namespaces)
	}
}
