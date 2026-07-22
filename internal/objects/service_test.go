package objects

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	obj        *Object
	err        error
	upsertArgs []string // "ns/object/author" per call
	deleted    []string
}

func (f *fakeRepo) Upsert(_ context.Context, ns, objectID, author string) (*Object, error) {
	f.upsertArgs = append(f.upsertArgs, ns+"/"+objectID+"/"+author)
	if f.err != nil {
		return nil, f.err
	}
	if f.obj != nil {
		return f.obj, nil
	}
	return &Object{Namespace: ns, ObjectID: objectID, AuthorSubjectID: author, UpdatedAt: time.Now()}, nil
}

func (f *fakeRepo) Get(_ context.Context, _, _ string) (*Object, error) { return f.obj, f.err }

func (f *fakeRepo) Delete(_ context.Context, ns, objectID string) error {
	f.deleted = append(f.deleted, ns+"/"+objectID)
	return f.err
}

func TestUpsert_TrimsAuthor(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	if _, err := svc.Upsert(context.Background(), "ns", "o1",
		&UpsertRequest{AuthorSubjectID: "  u1  "}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if len(repo.upsertArgs) != 1 || repo.upsertArgs[0] != "ns/o1/u1" {
		t.Errorf("repo args = %v, want ns/o1/u1", repo.upsertArgs)
	}
}

// An empty author through the objects endpoint is an explicit clear, unlike
// the catalog write-through where absence means "unspecified".
func TestUpsert_EmptyAuthorClears(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	if _, err := svc.Upsert(context.Background(), "ns", "o1", &UpsertRequest{}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if len(repo.upsertArgs) != 1 || repo.upsertArgs[0] != "ns/o1/" {
		t.Errorf("repo args = %v, want an empty author to reach the repo", repo.upsertArgs)
	}
}

func TestUpsert_RejectsMissingPathParams(t *testing.T) {
	svc := NewService(&fakeRepo{})
	for _, tc := range []struct{ ns, id string }{{"", "o1"}, {"ns", ""}, {"", ""}} {
		_, err := svc.Upsert(context.Background(), tc.ns, tc.id, &UpsertRequest{})
		if !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("ns=%q id=%q: expected ErrInvalidRequest, got %v", tc.ns, tc.id, err)
		}
	}
}

func TestUpsert_RejectsNilBody(t *testing.T) {
	svc := NewService(&fakeRepo{})
	if _, err := svc.Upsert(context.Background(), "ns", "o1", nil); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("expected ErrInvalidRequest, got %v", err)
	}
}

// SetAuthor is the catalog write-through: an empty author must be a no-op so
// a catalog re-ingest without attribution does not wipe an author that was
// set through the objects endpoint.
func TestSetAuthor_EmptyIsNoOp(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	for _, author := range []string{"", "   ", "\t"} {
		if err := svc.SetAuthor(context.Background(), "ns", "o1", author); err != nil {
			t.Fatalf("SetAuthor(%q): %v", author, err)
		}
	}
	if len(repo.upsertArgs) != 0 {
		t.Errorf("expected no repo writes, got %v", repo.upsertArgs)
	}
}

func TestSetAuthor_WritesTrimmed(t *testing.T) {
	repo := &fakeRepo{}
	if err := NewService(repo).SetAuthor(context.Background(), "ns", "o1", " u2 "); err != nil {
		t.Fatalf("SetAuthor: %v", err)
	}
	if len(repo.upsertArgs) != 1 || repo.upsertArgs[0] != "ns/o1/u2" {
		t.Errorf("repo args = %v", repo.upsertArgs)
	}
}

func TestSetAuthor_PropagatesRepoError(t *testing.T) {
	svc := NewService(&fakeRepo{err: errors.New("db down")})
	if err := svc.SetAuthor(context.Background(), "ns", "o1", "u1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDelete(t *testing.T) {
	repo := &fakeRepo{}
	if err := NewService(repo).Delete(context.Background(), "ns", "o1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != "ns/o1" {
		t.Errorf("deleted = %v", repo.deleted)
	}
}
