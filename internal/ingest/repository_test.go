package ingest

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewRepository(t *testing.T) {
	repo := NewRepository(nil)
	if repo == nil {
		t.Fatal("expected repository")
	}
}

func TestRepositoryInsert_ExecError(t *testing.T) {
	repo := &Repository{
		execFn: func(_ context.Context, _ string, _ ...any) error {
			return errors.New("exec failed")
		},
	}

	err := repo.Insert(context.Background(), &Event{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRepositoryInsert(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()
	defer db.Exec(ctx, `DELETE FROM events WHERE namespace = $1`, "ingest_test") //nolint:errcheck // test cleanup, failure is not critical

	repo := NewRepository(db)

	now := time.Now().UTC().Truncate(time.Second)
	event := &Event{
		Namespace:  "ingest_test",
		SubjectID:  "user-1",
		ObjectID:   "item-1",
		Action:     ActionLike,
		Weight:     5.0,
		OccurredAt: now,
	}

	if err := repo.Insert(ctx, event); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	var weight float64
	err = db.QueryRow(ctx,
		`SELECT weight FROM events WHERE namespace=$1 AND subject_id=$2 AND object_id=$3`,
		event.Namespace, event.SubjectID, event.ObjectID,
	).Scan(&weight)
	if err != nil {
		t.Fatalf("query inserted row: %v", err)
	}
	if weight != event.Weight {
		t.Errorf("weight: got %.1f, want %.1f", weight, event.Weight)
	}
}

func TestRepositoryInsert_WithObjectCreatedAt(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()
	defer db.Exec(ctx, `DELETE FROM events WHERE namespace = $1`, "ingest_test") //nolint:errcheck // test cleanup, failure is not critical

	repo := NewRepository(db)

	now := time.Now().UTC().Truncate(time.Second)
	createdAt := now.Add(-48 * time.Hour)
	event := &Event{
		Namespace:       "ingest_test",
		SubjectID:       "user-2",
		ObjectID:        "item-2",
		Action:          ActionView,
		Weight:          1.0,
		OccurredAt:      now,
		ObjectCreatedAt: &createdAt,
	}

	if err := repo.Insert(ctx, event); err != nil {
		t.Fatalf("Insert with ObjectCreatedAt: %v", err)
	}

	var gotCreatedAt *time.Time
	err = db.QueryRow(ctx,
		`SELECT object_created_at FROM events WHERE namespace=$1 AND subject_id=$2 AND object_id=$3`,
		event.Namespace, event.SubjectID, event.ObjectID,
	).Scan(&gotCreatedAt)
	if err != nil {
		t.Fatalf("query inserted row: %v", err)
	}
	if gotCreatedAt == nil || !gotCreatedAt.Truncate(time.Second).Equal(createdAt.Truncate(time.Second)) {
		t.Errorf("ObjectCreatedAt: got %v, want %v", gotCreatedAt, createdAt)
	}
}
