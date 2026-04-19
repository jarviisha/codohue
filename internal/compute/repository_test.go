package compute

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
	return db
}

func seedEvent(t *testing.T, db *pgxpool.Pool, ns, subjectID, objectID string, occurredAt time.Time) {
	t.Helper()
	_, err := db.Exec(context.Background(), `
		INSERT INTO events (namespace, subject_id, object_id, action, weight, occurred_at)
		VALUES ($1, $2, $3, 'VIEW', 1.0, $4)`,
		ns, subjectID, objectID, occurredAt,
	)
	if err != nil {
		t.Fatalf("seedEvent: %v", err)
	}
}

func cleanupNS(t *testing.T, db *pgxpool.Pool, ns string) {
	t.Helper()
	t.Cleanup(func() {
		db.Exec(context.Background(), //nolint:errcheck // test cleanup, failure is not critical
			`DELETE FROM events WHERE namespace = $1`, ns)
	})
}

func TestRepositoryGetActiveSubjects(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "compute_test")

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()

	seedEvent(t, db, "compute_test", "user-1", "item-1", now)
	seedEvent(t, db, "compute_test", "user-2", "item-2", now)
	// Duplicate — should be deduplicated.
	seedEvent(t, db, "compute_test", "user-1", "item-3", now)

	subjects, err := repo.GetActiveSubjects(ctx, "compute_test")
	if err != nil {
		t.Fatalf("GetActiveSubjects: %v", err)
	}

	got := make(map[string]bool)
	for _, s := range subjects {
		got[s] = true
	}
	for _, want := range []string{"user-1", "user-2"} {
		if !got[want] {
			t.Errorf("expected subject %q in results", want)
		}
	}
	if len(subjects) != 2 {
		t.Errorf("expected 2 distinct subjects, got %d", len(subjects))
	}
}

func TestRepositoryGetActiveSubjects_ExcludesOldEvents(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "compute_test_old")

	repo := NewRepository(db)
	ctx := context.Background()

	// Event older than 90 days — should not appear.
	old := time.Now().Add(-91 * 24 * time.Hour)
	seedEvent(t, db, "compute_test_old", "old-user", "item-1", old)

	subjects, err := repo.GetActiveSubjects(ctx, "compute_test_old")
	if err != nil {
		t.Fatalf("GetActiveSubjects: %v", err)
	}
	if len(subjects) != 0 {
		t.Errorf("expected no subjects for old events, got %v", subjects)
	}
}

func TestRepositoryGetSubjectEvents(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "compute_test_events")

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()

	seedEvent(t, db, "compute_test_events", "user-1", "item-1", now)
	seedEvent(t, db, "compute_test_events", "user-1", "item-2", now)
	seedEvent(t, db, "compute_test_events", "user-2", "item-3", now) // different subject

	events, err := repo.GetSubjectEvents(ctx, "compute_test_events", "user-1")
	if err != nil {
		t.Fatalf("GetSubjectEvents: %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 events for user-1, got %d", len(events))
	}
	for _, e := range events {
		if e.SubjectID != "user-1" {
			t.Errorf("unexpected SubjectID: %q", e.SubjectID)
		}
	}
}

func TestRepositoryGetActiveNamespaces(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "compute_ns_a")
	cleanupNS(t, db, "compute_ns_b")

	repo := NewRepository(db)
	ctx := context.Background()
	now := time.Now()

	seedEvent(t, db, "compute_ns_a", "user-1", "item-1", now)
	seedEvent(t, db, "compute_ns_b", "user-1", "item-1", now)

	namespaces, err := repo.GetActiveNamespaces(ctx)
	if err != nil {
		t.Fatalf("GetActiveNamespaces: %v", err)
	}

	got := make(map[string]bool)
	for _, ns := range namespaces {
		got[ns] = true
	}
	for _, want := range []string{"compute_ns_a", "compute_ns_b"} {
		if !got[want] {
			t.Errorf("expected namespace %q in results", want)
		}
	}
}

func TestRepositoryGetNamespaceEventsInWindow(t *testing.T) {
	db := openTestDB(t)
	cleanupNS(t, db, "compute_test_window")

	repo := NewRepository(db)
	ctx := context.Background()

	recent := time.Now().Add(-1 * time.Hour)
	old := time.Now().Add(-48 * time.Hour)

	seedEvent(t, db, "compute_test_window", "user-1", "item-recent", recent)
	seedEvent(t, db, "compute_test_window", "user-1", "item-old", old)

	events, err := repo.GetNamespaceEventsInWindow(ctx, "compute_test_window", 24)
	if err != nil {
		t.Fatalf("GetNamespaceEventsInWindow: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event in 24h window, got %d", len(events))
	}
	if events[0].ObjectID != "item-recent" {
		t.Errorf("expected item-recent, got %q", events[0].ObjectID)
	}
}
