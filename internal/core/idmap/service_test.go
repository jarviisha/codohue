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

func TestServiceLookupSubjectID(t *testing.T) {
	repo := &fakeRepo{id: 42, found: true}
	svc := NewService(repo)

	id, found, err := svc.LookupSubjectID(context.Background(), "s1", "ns")
	if err != nil {
		t.Fatalf("LookupSubjectID: %v", err)
	}
	if id != 42 || !found {
		t.Fatalf("got (%d, %v), want (42, true)", id, found)
	}
	// The entity type is what keeps a subject and an object with the same
	// string id from resolving to each other's mapping.
	if repo.lastType != "subject" {
		t.Errorf("entity type = %q, want subject", repo.lastType)
	}
	if repo.lastNS != "ns" {
		t.Errorf("namespace = %q, want ns", repo.lastNS)
	}
}

// An absent mapping is not an error: the caller distinguishes "no vector yet"
// from "the store is broken", and a lookup must never mint a row.
func TestServiceLookupSubjectID_MissingIsNotAnError(t *testing.T) {
	svc := NewService(&fakeRepo{found: false})

	id, found, err := svc.LookupSubjectID(context.Background(), "s1", "ns")
	if err != nil {
		t.Fatalf("LookupSubjectID: %v", err)
	}
	if found || id != 0 {
		t.Fatalf("got (%d, %v), want (0, false)", id, found)
	}
}

func TestServiceLookupSubjectID_Error(t *testing.T) {
	svc := NewService(&fakeRepo{err: errors.New("db down")})

	if _, _, err := svc.LookupSubjectID(context.Background(), "s1", "ns"); err == nil {
		t.Fatal("expected error")
	}
}

func TestServiceLookupObjectIDs(t *testing.T) {
	repo := &fakeRepo{id: 100}
	svc := NewService(repo)

	ids, err := svc.LookupObjectIDs(context.Background(), []string{"o1", "o2"}, "ns")
	if err != nil {
		t.Fatalf("LookupObjectIDs: %v", err)
	}
	if len(ids) != 2 || ids["o1"] != 100 || ids["o2"] != 101 {
		t.Fatalf("ids = %v, want o1=100 o2=101", ids)
	}
	if repo.lastType != "object" {
		t.Errorf("entity type = %q, want object", repo.lastType)
	}
}

func TestServiceLookupObjectIDs_Error(t *testing.T) {
	svc := NewService(&fakeRepo{err: errors.New("db down")})

	if _, err := svc.LookupObjectIDs(context.Background(), []string{"o1"}, "ns"); err == nil {
		t.Fatal("expected error")
	}
}

// lookupOnlyRepo satisfies idmapRepo without LookupBatch, which is the
// fallback LookupObjectIDs takes for a repository that cannot batch. Absent
// ids are omitted rather than mapped to zero — zero is a valid point id.
type lookupOnlyRepo struct {
	ids map[string]uint64
	err error
}

func (r *lookupOnlyRepo) GetOrCreate(context.Context, string, string, string) (uint64, error) {
	return 0, nil
}

func (r *lookupOnlyRepo) Lookup(_ context.Context, stringID, _, _ string) (numericID uint64, found bool, err error) {
	if r.err != nil {
		return 0, false, r.err
	}
	id, ok := r.ids[stringID]
	return id, ok, nil
}

func (r *lookupOnlyRepo) GetOrCreateBatch(context.Context, []string, string, string) (map[string]uint64, error) {
	return nil, nil
}

func TestServiceLookupObjectIDs_FallsBackToPerIDLookup(t *testing.T) {
	svc := NewService(&lookupOnlyRepo{ids: map[string]uint64{"o1": 7}})

	ids, err := svc.LookupObjectIDs(context.Background(), []string{"o1", "missing"}, "ns")
	if err != nil {
		t.Fatalf("LookupObjectIDs: %v", err)
	}
	if len(ids) != 1 || ids["o1"] != 7 {
		t.Fatalf("ids = %v, want only o1=7", ids)
	}
	if _, present := ids["missing"]; present {
		t.Error("an unmapped id was returned; the caller cannot tell it from a real point id")
	}
}

func TestServiceLookupObjectIDs_FallbackError(t *testing.T) {
	svc := NewService(&lookupOnlyRepo{err: errors.New("db down")})

	if _, err := svc.LookupObjectIDs(context.Background(), []string{"o1"}, "ns"); err == nil {
		t.Fatal("expected error")
	}
}
