package idmap

import (
	"context"
	"errors"
	"testing"
)

type fakeRow struct {
	scanFn func(dest ...any) error
}

func (f fakeRow) Scan(dest ...any) error {
	return f.scanFn(dest...)
}

type fakeRepo struct {
	id         uint64
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
