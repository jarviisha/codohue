package idmap

import (
	"context"
	"errors"
	"testing"
)

// Service tests. The shared fakes live in idmap_test.go alongside the
// repository tests, so both halves drive the same doubles.

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
