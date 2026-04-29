package compute

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type rowsIterator interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// Repository reads events and namespace data from PostgreSQL for the compute job.
type Repository struct {
	db      *pgxpool.Pool
	queryFn func(ctx context.Context, sql string, args ...any) (rowsIterator, error)
}

// NewRepository creates a new Repository with the given PostgreSQL connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
		queryFn: func(ctx context.Context, sql string, args ...any) (rowsIterator, error) {
			return db.Query(ctx, sql, args...)
		},
	}
}

// GetActiveSubjects returns subjects that have events within the last 90 days.
func (r *Repository) GetActiveSubjects(ctx context.Context, namespace string) ([]string, error) {
	rows, err := r.queryFn(ctx, `
		SELECT DISTINCT subject_id FROM events
		WHERE namespace = $1
		  AND occurred_at > NOW() - INTERVAL '90 days'`,
		namespace,
	)
	if err != nil {
		return nil, fmt.Errorf("get active subjects: %w", err)
	}
	defer rows.Close()

	var subjects []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan subject: %w", err)
		}
		subjects = append(subjects, id)
	}
	if err := rows.Err(); err != nil {
		return subjects, fmt.Errorf("iterate active subjects: %w", err)
	}
	return subjects, nil
}

// GetSubjectEvents returns all events for a subject within the last 90 days.
func (r *Repository) GetSubjectEvents(ctx context.Context, namespace, subjectID string) ([]*RawEvent, error) {
	rows, err := r.queryFn(ctx, `
		SELECT subject_id, object_id, action, weight,
		       EXTRACT(EPOCH FROM occurred_at)::BIGINT,
		       EXTRACT(EPOCH FROM object_created_at)::BIGINT
		FROM events
		WHERE subject_id = $1
		  AND namespace  = $2
		  AND occurred_at > NOW() - INTERVAL '90 days'`,
		subjectID, namespace,
	)
	if err != nil {
		return nil, fmt.Errorf("get subject events: %w", err)
	}
	defer rows.Close()

	var events []*RawEvent
	for rows.Next() {
		e := &RawEvent{}
		if err := rows.Scan(&e.SubjectID, &e.ObjectID, &e.Action, &e.Weight, &e.OccurredAt, &e.ObjectCreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return events, fmt.Errorf("iterate subject events: %w", err)
	}
	return events, nil
}

// GetAllNamespaceEvents returns all events for a namespace within the last 90 days,
// ordered by subject_id then occurred_at. Used by dense vector computation to build
// interaction sequences and interaction matrices without an extra DB round-trip.
func (r *Repository) GetAllNamespaceEvents(ctx context.Context, namespace string) ([]*RawEvent, error) {
	rows, err := r.queryFn(ctx, `
		SELECT subject_id, object_id, action, weight,
		       EXTRACT(EPOCH FROM occurred_at)::BIGINT,
		       EXTRACT(EPOCH FROM object_created_at)::BIGINT
		FROM events
		WHERE namespace  = $1
		  AND occurred_at > NOW() - INTERVAL '90 days'
		ORDER BY subject_id, occurred_at`,
		namespace,
	)
	if err != nil {
		return nil, fmt.Errorf("get all namespace events: %w", err)
	}
	defer rows.Close()

	var events []*RawEvent
	for rows.Next() {
		e := &RawEvent{}
		if err := rows.Scan(&e.SubjectID, &e.ObjectID, &e.Action, &e.Weight, &e.OccurredAt, &e.ObjectCreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return events, fmt.Errorf("iterate namespace events: %w", err)
	}
	return events, nil
}

// GetNamespaceEventsInWindow returns all events for a namespace within the last windowHours hours,
// ordered by occurred_at ascending. Used by trending score computation.
func (r *Repository) GetNamespaceEventsInWindow(ctx context.Context, namespace string, windowHours int) ([]*RawEvent, error) {
	rows, err := r.queryFn(ctx, `
		SELECT subject_id, object_id, action, weight,
		       EXTRACT(EPOCH FROM occurred_at)::BIGINT,
		       EXTRACT(EPOCH FROM object_created_at)::BIGINT
		FROM events
		WHERE namespace  = $1
		  AND occurred_at > NOW() - make_interval(hours => $2)
		ORDER BY occurred_at`,
		namespace, windowHours,
	)
	if err != nil {
		return nil, fmt.Errorf("get namespace events in window: %w", err)
	}
	defer rows.Close()

	var events []*RawEvent
	for rows.Next() {
		e := &RawEvent{}
		if err := rows.Scan(&e.SubjectID, &e.ObjectID, &e.Action, &e.Weight, &e.OccurredAt, &e.ObjectCreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return events, fmt.Errorf("iterate window events: %w", err)
	}
	return events, nil
}

// GetActiveNamespaces returns namespaces that have events within the last 90 days.
func (r *Repository) GetActiveNamespaces(ctx context.Context) ([]string, error) {
	rows, err := r.queryFn(ctx, `
		SELECT DISTINCT namespace FROM events
		WHERE occurred_at > NOW() - INTERVAL '90 days'`,
	)
	if err != nil {
		return nil, fmt.Errorf("get active namespaces: %w", err)
	}
	defer rows.Close()

	var ns []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scan namespace: %w", err)
		}
		ns = append(ns, n)
	}
	if err := rows.Err(); err != nil {
		return ns, fmt.Errorf("iterate active namespaces: %w", err)
	}
	return ns, nil
}

// InsertBatchRunLog inserts a new in-progress batch run log row and returns its ID.
func (r *Repository) InsertBatchRunLog(ctx context.Context, namespace string, startedAt time.Time) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO batch_run_logs (namespace, started_at, subjects_processed, success)
		VALUES ($1, $2, 0, FALSE)
		RETURNING id
	`, namespace, startedAt).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert batch_run_log: %w", err)
	}
	return id, nil
}

// UpdateBatchRunLog updates a batch run log row on completion or failure.
func (r *Repository) UpdateBatchRunLog(ctx context.Context, id int64, completedAt time.Time, durationMs int, subjectsProcessed int, success bool, errMsg string) error {
	var errMsgPtr *string
	if errMsg != "" {
		errMsgPtr = &errMsg
	}
	_, err := r.db.Exec(ctx, `
		UPDATE batch_run_logs
		SET completed_at = $2, duration_ms = $3, subjects_processed = $4, success = $5, error_message = $6
		WHERE id = $1
	`, id, completedAt, durationMs, subjectsProcessed, success, errMsgPtr)
	if err != nil {
		return fmt.Errorf("update batch_run_log %d: %w", id, err)
	}
	return nil
}
