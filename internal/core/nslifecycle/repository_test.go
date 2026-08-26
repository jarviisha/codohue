package nslifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeStore struct {
	lifecycle *NamespaceLifecycle
	system    *SystemLifecycle
	errorText string
}

func (f *fakeStore) Activate(_ context.Context, namespace string) (*NamespaceLifecycle, error) {
	if f.lifecycle == nil {
		f.lifecycle = &NamespaceLifecycle{Namespace: namespace, Generation: 1, State: StateActive}
		return cloneLifecycle(f.lifecycle), nil
	}
	if f.lifecycle.State == StateDeleting {
		return nil, ErrNamespaceNotActive
	}
	if f.lifecycle.State == StateDeleted {
		f.lifecycle.Generation++
		f.lifecycle.State = StateActive
		f.lifecycle.LegacyMessagesAllowed = false
	}
	return cloneLifecycle(f.lifecycle), nil
}

func (f *fakeStore) GetNamespace(_ context.Context, _ string) (*NamespaceLifecycle, error) {
	return cloneLifecycle(f.lifecycle), nil
}
func (f *fakeStore) StartDelete(_ context.Context, _ string) (*NamespaceLifecycle, error) {
	f.lifecycle.State = StateDeleting
	return cloneLifecycle(f.lifecycle), nil
}
func (f *fakeStore) CompleteDelete(_ context.Context, _ string, _ int64) error {
	f.lifecycle.State = StateDeleted
	return nil
}
func (f *fakeStore) RecordNamespaceError(_ context.Context, _ string, _ int64, message string) error {
	f.errorText = message
	f.lifecycle.LastError = message
	return nil
}
func (f *fakeStore) GetSystem(context.Context) (*SystemLifecycle, error) {
	if f.system == nil {
		f.system = &SystemLifecycle{State: SystemActive}
	}
	snapshot := *f.system
	return &snapshot, nil
}
func (f *fakeStore) StartReset(context.Context) error    { f.system.State = SystemResetting; return nil }
func (f *fakeStore) CompleteReset(context.Context) error { f.system.State = SystemActive; return nil }
func (f *fakeStore) RecordResetError(_ context.Context, message string) error {
	f.system.LastError = message
	return nil
}
func (f *fakeStore) DisableLegacy(_ context.Context, _ string, at time.Time) (bool, error) {
	if f.system.LegacyEnvelopesDisabledAt != nil {
		return false, nil
	}
	f.system.LegacyEnvelopesDisabledAt = &at
	if f.lifecycle != nil {
		f.lifecycle.LegacyMessagesAllowed = false
	}
	return true, nil
}

func cloneLifecycle(in *NamespaceLifecycle) *NamespaceLifecycle {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func TestLifecyclePersistenceTransitionsAndGeneration(t *testing.T) {
	store := &fakeStore{system: &SystemLifecycle{State: SystemActive}}
	first, err := store.Activate(context.Background(), "tenant")
	if err != nil || first.Generation != 1 || first.State != StateActive {
		t.Fatalf("first activation = %+v, %v", first, err)
	}
	if _, err := store.StartDelete(context.Background(), "tenant"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Activate(context.Background(), "tenant"); !errors.Is(err, ErrNamespaceNotActive) {
		t.Fatalf("activate while deleting error = %v", err)
	}
	if err := store.RecordNamespaceError(context.Background(), "tenant", 1, "qdrant down"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteDelete(context.Background(), "tenant", 1); err != nil {
		t.Fatal(err)
	}
	second, err := store.Activate(context.Background(), "tenant")
	if err != nil || second.Generation != 2 || second.State != StateActive || second.LegacyMessagesAllowed {
		t.Fatalf("recreation = %+v, %v", second, err)
	}
}

func TestSystemResetAndLegacyClosureAreDurableAndIdempotent(t *testing.T) {
	store := &fakeStore{system: &SystemLifecycle{State: SystemActive}, lifecycle: &NamespaceLifecycle{Namespace: "tenant", Generation: 1, State: StateActive, LegacyMessagesAllowed: true}}
	if err := store.StartReset(context.Background()); err != nil {
		t.Fatal(err)
	}
	state, _ := store.GetSystem(context.Background())
	if state.State != SystemResetting {
		t.Fatalf("state = %q", state.State)
	}
	if err := store.RecordResetError(context.Background(), "redis down"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteReset(context.Background()); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	changed, err := store.DisableLegacy(context.Background(), "deploy-42", at)
	if err != nil || !changed {
		t.Fatalf("first closure changed=%v err=%v", changed, err)
	}
	changed, err = store.DisableLegacy(context.Background(), "deploy-42", at.Add(time.Hour))
	if err != nil || changed {
		t.Fatalf("second closure changed=%v err=%v", changed, err)
	}
	if store.lifecycle.LegacyMessagesAllowed {
		t.Fatal("legacy allowance reopened")
	}
}

// The janitor walks ListCleanupCandidates, and until now only a fake
// implemented it — the real generate_series bound and keyset predicate were
// verified by hand. These exercise the SQL.
//
// The query scans the whole ledger, so the fixtures use a high-sorting unique
// prefix and every assertion is anchored to a cursor just below it. That keeps
// the results deterministic on a shared database without the test having to
// own the table.
const cleanupCandidatePrefix = "zzz_cleanup_candidates_test_"

func seedCleanupLifecycle(t *testing.T, db *pgxpool.Pool, suffix, state string, generation int64) string {
	t.Helper()
	namespace := cleanupCandidatePrefix + suffix
	ctx := context.Background()
	if _, err := db.Exec(ctx, `DELETE FROM namespace_lifecycles WHERE namespace = $1`, namespace); err != nil {
		t.Fatalf("clear lifecycle %q: %v", namespace, err)
	}
	if _, err := db.Exec(ctx, `
		INSERT INTO namespace_lifecycles (namespace, generation, state, activated_at)
		VALUES ($1, $2, $3, NOW())`, namespace, generation, state); err != nil {
		t.Fatalf("seed lifecycle %q: %v", namespace, err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM namespace_lifecycles WHERE namespace = $1`, namespace)
	})
	return namespace
}

// beforeCleanupFixtures is a cursor sorting immediately below every fixture, so
// a page starting here begins at the first one.
var beforeCleanupFixtures = CleanupCandidate{Namespace: cleanupCandidatePrefix, Generation: 0}

func TestListCleanupCandidatesBoundsGenerationsByLifecycleState(t *testing.T) {
	db := testPool(t)
	repo := NewRepository(db)

	// A deleted lifecycle's current generation is itself superseded; an active
	// one's is not, so only the generations below it are candidates.
	deleted := seedCleanupLifecycle(t, db, "a_deleted_gen3", "deleted", 3)
	active := seedCleanupLifecycle(t, db, "b_active_gen2", "active", 2)
	seedCleanupLifecycle(t, db, "c_active_gen1", "active", 1)

	// Deliberately over-fetch and filter to the fixtures. Asking for exactly the
	// expected count would hide the dangerous direction of a wrong bound: an
	// active lifecycle's *live* generation becoming a deletion candidate simply
	// falls past the limit and is never seen.
	page, err := repo.ListCleanupCandidates(context.Background(), beforeCleanupFixtures, 20)
	if err != nil {
		t.Fatalf("ListCleanupCandidates: %v", err)
	}
	var got []CleanupCandidate
	for _, candidate := range page {
		if strings.HasPrefix(candidate.Namespace, cleanupCandidatePrefix) {
			got = append(got, candidate)
		}
	}

	// A deleted lifecycle's own generation is superseded, so it is included. An
	// active one's is not: emitting it would delete the collections a live
	// namespace is serving from. The generation-1 active lifecycle therefore
	// contributes nothing at all — generate_series(1, 0) is empty.
	want := []CleanupCandidate{
		{Namespace: deleted, Generation: 1},
		{Namespace: deleted, Generation: 2},
		{Namespace: deleted, Generation: 3},
		{Namespace: active, Generation: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d fixture candidates (%v), want exactly %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestListCleanupCandidatesResumesStrictlyAfterTheCursor(t *testing.T) {
	db := testPool(t)
	repo := NewRepository(db)

	deleted := seedCleanupLifecycle(t, db, "a_deleted_gen3", "deleted", 3)
	active := seedCleanupLifecycle(t, db, "b_active_gen2", "active", 2)

	ctx := context.Background()
	first, err := repo.ListCleanupCandidates(ctx, beforeCleanupFixtures, 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first) != 2 || first[0].Generation != 1 || first[1].Generation != 2 {
		t.Fatalf("first page = %v, want generations 1 and 2 of %q", first, deleted)
	}

	// Resuming from the last row of the first page must not repeat it — that
	// repetition is what made a plain LIMIT re-clean the head forever.
	second, err := repo.ListCleanupCandidates(ctx, first[1], 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	want := []CleanupCandidate{{Namespace: deleted, Generation: 3}, {Namespace: active, Generation: 1}}
	if len(second) != 2 || second[0] != want[0] || second[1] != want[1] {
		t.Fatalf("second page = %v, want %v", second, want)
	}

	// The cursor orders on (namespace, generation), not namespace alone: a
	// cursor mid-way through one namespace's generations must resume inside it.
	mid, err := repo.ListCleanupCandidates(ctx, CleanupCandidate{Namespace: deleted, Generation: 1}, 1)
	if err != nil {
		t.Fatalf("mid-namespace page: %v", err)
	}
	if len(mid) != 1 || mid[0] != (CleanupCandidate{Namespace: deleted, Generation: 2}) {
		t.Fatalf("mid-namespace page = %v, want generation 2 of %q", mid, deleted)
	}
}

func TestListCleanupCandidatesExhaustsTheFixtures(t *testing.T) {
	db := testPool(t)
	repo := NewRepository(db)

	deleted := seedCleanupLifecycle(t, db, "a_deleted_gen2", "deleted", 2)

	ctx := context.Background()
	page, err := repo.ListCleanupCandidates(ctx, CleanupCandidate{Namespace: deleted, Generation: 2}, 10)
	if err != nil {
		t.Fatalf("ListCleanupCandidates: %v", err)
	}
	// Past the last fixture the walk is done; anything returned belongs to
	// another namespace, never to this one.
	for _, candidate := range page {
		if strings.HasPrefix(candidate.Namespace, cleanupCandidatePrefix) {
			t.Errorf("cursor past the last fixture still returned %v", candidate)
		}
	}
}

func TestListCleanupCandidatesRejectsANonPositiveLimit(t *testing.T) {
	db := testPool(t)
	repo := NewRepository(db)

	for _, limit := range []int{0, -1} {
		got, err := repo.ListCleanupCandidates(context.Background(), beforeCleanupFixtures, limit)
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		if got != nil {
			t.Errorf("limit %d returned %v, want no candidates", limit, got)
		}
	}
}
