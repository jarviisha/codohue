package recommend

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type rowScanner interface {
	Scan(dest ...any) error
}

type rowsIterator interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// Repository queries events and popular items from PostgreSQL for the recommendation service.
type Repository struct {
	db         *pgxpool.Pool
	queryFn    func(ctx context.Context, sql string, args ...any) (rowsIterator, error)
	queryRowFn func(ctx context.Context, sql string, args ...any) rowScanner
}

// NewRepository creates a new Repository with the given PostgreSQL connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
		queryFn: func(ctx context.Context, sql string, args ...any) (rowsIterator, error) {
			return db.Query(ctx, sql, args...)
		},
		queryRowFn: func(ctx context.Context, sql string, args ...any) rowScanner {
			return db.QueryRow(ctx, sql, args...)
		},
	}
}

// GetSeenItems returns the distinct object IDs the subject interacted with in the last seenItemsDays days.
func (r *Repository) GetSeenItems(ctx context.Context, namespace, subjectID string, seenItemsDays int) ([]string, error) {
	rows, err := r.queryFn(ctx, `
		SELECT DISTINCT object_id FROM events
		WHERE subject_id  = $1
		  AND namespace   = $2
		  AND occurred_at > NOW() - ($3 * INTERVAL '1 day')`,
		subjectID, namespace, seenItemsDays,
	)
	if err != nil {
		return nil, fmt.Errorf("query seen items: %w", err)
	}
	defer rows.Close()

	var items []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan seen item: %w", err)
		}
		items = append(items, id)
	}
	if err := rows.Err(); err != nil {
		return items, fmt.Errorf("iterate seen items: %w", err)
	}
	return items, nil
}

// GetPopularItems returns the top items by interaction weight in the last 7 days.
func (r *Repository) GetPopularItems(ctx context.Context, namespace string, limit int) ([]string, error) {
	rows, err := r.queryFn(ctx, `
		SELECT object_id FROM (
			SELECT object_id, SUM(weight) AS popularity_score
			FROM events
			WHERE namespace  = $1
			  AND occurred_at > NOW() - INTERVAL '7 days'
			GROUP BY object_id
			ORDER BY popularity_score DESC
			LIMIT $2
		) sub`,
		namespace, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query popular items: %w", err)
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan popular item: %w", err)
		}
		items = append(items, id)
	}
	if err := rows.Err(); err != nil {
		return items, fmt.Errorf("iterate popular items: %w", err)
	}
	return items, nil
}

// CountInteractions returns the total number of interactions for a subject in the namespace.
func (r *Repository) CountInteractions(ctx context.Context, namespace, subjectID string) (int, error) {
	var count int
	err := r.queryRowFn(ctx, `
		SELECT COUNT(*) FROM events
		WHERE subject_id = $1 AND namespace = $2`,
		subjectID, namespace,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count interactions: %w", err)
	}
	return count, nil
}
