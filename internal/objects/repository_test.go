package objects

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const testNS = "objects_repo_test"

func openTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	u := os.Getenv("DATABASE_URL")
	if u == "" {
		t.Skip("DATABASE_URL not set")
	}
	db, err := pgxpool.New(context.Background(), u)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	t.Cleanup(func() {
		db.Exec(context.Background(), //nolint:errcheck // test cleanup, failure is not critical
			`DELETE FROM objects WHERE namespace = $1`, testNS)
	})
	return db
}

func TestRepositoryUpsert_CreatesThenUpdates(t *testing.T) {
	repo := NewRepository(openTestDB(t))
	ctx := context.Background()

	created, err := repo.Upsert(ctx, testNS, "o1", "u1")
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}
	if created.AuthorSubjectID != "u1" {
		t.Fatalf("author: got %q, want u1", created.AuthorSubjectID)
	}

	updated, err := repo.Upsert(ctx, testNS, "o1", "u2")
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	if updated.AuthorSubjectID != "u2" {
		t.Errorf("author: got %q, want u2", updated.AuthorSubjectID)
	}
	// The row is keyed by (namespace, object_id), so re-upserting must not
	// create a second row or move created_at.
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("created_at moved on update: %v -> %v", created.CreatedAt, updated.CreatedAt)
	}
}

// An empty author is stored as NULL, and reads back as the empty string via
// COALESCE — never as a literal "" in the column, which would make the
// partial index and the IS NOT NULL coverage count disagree.
func TestRepositoryUpsert_EmptyAuthorStoresNull(t *testing.T) {
	db := openTestDB(t)
	repo := NewRepository(db)
	ctx := context.Background()

	if _, err := repo.Upsert(ctx, testNS, "o2", "u1"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := repo.Upsert(ctx, testNS, "o2", "")
	if err != nil {
		t.Fatalf("clearing Upsert: %v", err)
	}
	if got.AuthorSubjectID != "" {
		t.Errorf("author: got %q, want empty", got.AuthorSubjectID)
	}

	var nulls int
	if err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM objects WHERE namespace = $1 AND object_id = 'o2' AND author_subject_id IS NULL`,
		testNS,
	).Scan(&nulls); err != nil {
		t.Fatalf("count nulls: %v", err)
	}
	if nulls != 1 {
		t.Errorf("expected the cleared author to be SQL NULL, got %d null rows", nulls)
	}
}

func TestRepositoryGet_MissingReturnsNil(t *testing.T) {
	got, err := NewRepository(openTestDB(t)).Get(context.Background(), testNS, "never-written")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for a missing object, got %+v", got)
	}
}

func TestRepositoryGet_ReturnsRow(t *testing.T) {
	repo := NewRepository(openTestDB(t))
	ctx := context.Background()
	if _, err := repo.Upsert(ctx, testNS, "o3", "u9"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.Get(ctx, testNS, "o3")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.AuthorSubjectID != "u9" || got.ObjectID != "o3" {
		t.Errorf("unexpected row: %+v", got)
	}
}

// Delete is idempotent — removing an object that was never attributed is a
// no-op, which is what the object-deletion path relies on.
func TestRepositoryDelete_Idempotent(t *testing.T) {
	repo := NewRepository(openTestDB(t))
	ctx := context.Background()
	if _, err := repo.Upsert(ctx, testNS, "o4", "u1"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	for i := range 2 {
		if err := repo.Delete(ctx, testNS, "o4"); err != nil {
			t.Fatalf("Delete #%d: %v", i+1, err)
		}
	}
	got, err := repo.Get(ctx, testNS, "o4")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Errorf("expected the row gone, got %+v", got)
	}
}
