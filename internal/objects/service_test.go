package objects

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
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

// --- lifecycle fencing ----------------------------------------------------

type fakeLifecycleWriter struct {
	generation int64
	err        error
	calls      int
	leased     bool
}

func (f *fakeLifecycleWriter) WithWriter(ctx context.Context, namespace string, fn func(context.Context, *nslifecycle.NamespaceLifecycle) error) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	leasedCtx := nslifecycle.ContextWithLease(ctx, namespace, f.generation, nslifecycle.LockShared)
	return fn(leasedCtx, &nslifecycle.NamespaceLifecycle{
		Namespace:  namespace,
		Generation: f.generation,
		State:      nslifecycle.StateActive,
	})
}

// Object metadata is namespace-owned state, so every mutation runs under a
// lifecycle lease. Without it, a write racing a delete would resurrect a row
// in a namespace the operator was told no longer exists.
func TestObjectMutations_RunUnderLifecycleLease(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Service) error
	}{
		{"Upsert", func(s *Service) error {
			_, err := s.Upsert(context.Background(), "ns", "o1", &UpsertRequest{AuthorSubjectID: "u1"})
			return err
		}},
		{"SetAuthor", func(s *Service) error {
			return s.SetAuthor(context.Background(), "ns", "o1", "u1")
		}},
		{"Delete", func(s *Service) error {
			return s.Delete(context.Background(), "ns", "o1")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := NewService(repo)
			lifecycle := &fakeLifecycleWriter{generation: 3}
			svc.SetLifecycleWriter(lifecycle)

			if err := tc.mutate(svc); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if lifecycle.calls != 1 {
				t.Errorf("expected exactly 1 lease acquisition, got %d", lifecycle.calls)
			}
		})
	}
}

// Reads are deliberately unfenced: a lease is a writer contract, and taking one
// per Get would serialize read traffic behind delete/recreate.
func TestObjectGet_TakesNoLease(t *testing.T) {
	svc := NewService(&fakeRepo{obj: &Object{Namespace: "ns", ObjectID: "o1"}})
	lifecycle := &fakeLifecycleWriter{generation: 3}
	svc.SetLifecycleWriter(lifecycle)

	if _, err := svc.Get(context.Background(), "ns", "o1"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if lifecycle.calls != 0 {
		t.Errorf("reads must not take a lease, got %d acquisitions", lifecycle.calls)
	}
}

func TestObjectMutations_InactiveNamespaceWritesNothing(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	svc.SetLifecycleWriter(&fakeLifecycleWriter{err: nslifecycle.ErrNamespaceNotActive})

	if _, err := svc.Upsert(context.Background(), "ns", "o1", &UpsertRequest{AuthorSubjectID: "u1"}); !errors.Is(err, nslifecycle.ErrNamespaceNotActive) {
		t.Fatalf("Upsert: expected ErrNamespaceNotActive, got %v", err)
	}
	if err := svc.Delete(context.Background(), "ns", "o1"); !errors.Is(err, nslifecycle.ErrNamespaceNotActive) {
		t.Fatalf("Delete: expected ErrNamespaceNotActive, got %v", err)
	}
	if len(repo.upsertArgs) != 0 || len(repo.deleted) != 0 {
		t.Errorf("inactive namespace reached the repo: upserts=%v deletes=%v", repo.upsertArgs, repo.deleted)
	}
}

// A caller that already holds the lease — catalog ingest writing attribution
// through under its own lease — must not take a second one.
func TestObjectMutations_ReuseHeldLease(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	lifecycle := &fakeLifecycleWriter{generation: 3}
	svc.SetLifecycleWriter(lifecycle)

	ctx := nslifecycle.ContextWithLease(context.Background(), "ns", 3, nslifecycle.LockShared)
	if err := svc.SetAuthor(ctx, "ns", "o1", "u1"); err != nil {
		t.Fatalf("SetAuthor: %v", err)
	}
	if lifecycle.calls != 0 {
		t.Errorf("held lease must be reused, got %d acquisitions", lifecycle.calls)
	}
	if len(repo.upsertArgs) != 1 {
		t.Errorf("write did not reach the repo: %v", repo.upsertArgs)
	}
}
