package nslifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists lifecycle gates in PostgreSQL.
type Repository struct{ db *pgxpool.Pool }

// NewRepository constructs a lifecycle repository.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func scanNamespace(row pgx.Row) (*NamespaceLifecycle, error) {
	var lifecycle NamespaceLifecycle
	var lastError *string
	err := row.Scan(&lifecycle.Namespace, &lifecycle.Generation, &lifecycle.State,
		&lifecycle.ActivatedAt, &lifecycle.LegacyMessagesAllowed, &lastError, &lifecycle.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNamespaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan namespace lifecycle: %w", err)
	}
	if lastError != nil {
		lifecycle.LastError = *lastError
	}
	return &lifecycle, nil
}

// Activate creates generation 1 or recreates a fully deleted namespace with
// generation+1. A deleting namespace remains fenced.
func (r *Repository) Activate(ctx context.Context, namespace string) (*NamespaceLifecycle, error) {
	lifecycle, err := scanNamespace(r.db.QueryRow(ctx, `
		INSERT INTO namespace_lifecycles
			(namespace, generation, state, activated_at, legacy_messages_allowed, updated_at)
		VALUES ($1, 1, 'active', NOW(),
			-- Generation 1 has exactly one incarnation, so an envelope that names
			-- no generation is unambiguous — and the documented Redis Streams
			-- transport publishes exactly that. Seeding FALSE would make every
			-- stream event for a newly created namespace be acked and dropped
			-- with nothing returned to the producer. The gate closes globally,
			-- once, against adoption evidence (see DisableLegacyEnvelopes); a
			-- namespace created after that must not reopen it.
			(SELECT legacy_envelopes_disabled_at IS NULL FROM system_lifecycle WHERE singleton = TRUE),
			NOW())
		ON CONFLICT (namespace) DO UPDATE SET
			generation = CASE WHEN namespace_lifecycles.state = 'deleted'
				THEN namespace_lifecycles.generation + 1 ELSE namespace_lifecycles.generation END,
			state = 'active',
			activated_at = CASE WHEN namespace_lifecycles.state = 'deleted'
				THEN NOW() ELSE namespace_lifecycles.activated_at END,
			legacy_messages_allowed = CASE WHEN namespace_lifecycles.state = 'deleted'
				THEN FALSE ELSE namespace_lifecycles.legacy_messages_allowed END,
			last_error = CASE WHEN namespace_lifecycles.state = 'deleted'
				THEN NULL ELSE namespace_lifecycles.last_error END,
			updated_at = CASE WHEN namespace_lifecycles.state = 'deleted'
				THEN NOW() ELSE namespace_lifecycles.updated_at END
		WHERE namespace_lifecycles.state IN ('active', 'deleted')
		RETURNING namespace, generation, state, activated_at,
			legacy_messages_allowed, last_error, updated_at`, namespace))
	if errors.Is(err, ErrNamespaceNotFound) {
		return nil, ErrNamespaceNotActive
	}
	if err != nil {
		return nil, fmt.Errorf("activate namespace lifecycle: %w", err)
	}
	return lifecycle, nil
}

// GetNamespace returns the current lifecycle or ErrNamespaceNotFound.
func (r *Repository) GetNamespace(ctx context.Context, namespace string) (*NamespaceLifecycle, error) {
	lifecycle, err := scanNamespace(r.db.QueryRow(ctx, `
		SELECT namespace, generation, state, activated_at,
			legacy_messages_allowed, last_error, updated_at
		FROM namespace_lifecycles WHERE namespace = $1`, namespace))
	if err != nil {
		return nil, fmt.Errorf("get namespace lifecycle: %w", err)
	}
	return lifecycle, nil
}

// StartDelete durably closes an active lifecycle and is idempotent while deleting.
func (r *Repository) StartDelete(ctx context.Context, namespace string) (*NamespaceLifecycle, error) {
	lifecycle, err := scanNamespace(r.db.QueryRow(ctx, `
		UPDATE namespace_lifecycles
		SET state = 'deleting', last_error = NULL, updated_at = NOW()
		WHERE namespace = $1 AND state IN ('active', 'deleting')
		RETURNING namespace, generation, state, activated_at,
			legacy_messages_allowed, last_error, updated_at`, namespace))
	if err != nil {
		return nil, fmt.Errorf("start namespace deletion: %w", err)
	}
	return lifecycle, nil
}

// CompleteDelete records verified cleanup while retaining the tombstone.
func (r *Repository) CompleteDelete(ctx context.Context, namespace string, generation int64) error {
	tag, err := r.db.Exec(ctx, `UPDATE namespace_lifecycles
		SET state = 'deleted', last_error = NULL, updated_at = NOW()
		WHERE namespace = $1 AND generation = $2 AND state = 'deleting'`, namespace, generation)
	if err != nil {
		return fmt.Errorf("complete namespace deletion: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNamespaceNotActive
	}
	return nil
}

// RecordNamespaceError persists a resumable cleanup failure without opening the gate.
func (r *Repository) RecordNamespaceError(ctx context.Context, namespace string, generation int64, message string) error {
	_, err := r.db.Exec(ctx, `UPDATE namespace_lifecycles
		SET last_error = $3, updated_at = NOW()
		WHERE namespace = $1 AND generation = $2 AND state = 'deleting'`, namespace, generation, message)
	if err != nil {
		return fmt.Errorf("record namespace lifecycle error: %w", err)
	}
	return nil
}

// GetSystem reads the singleton system lifecycle.
func (r *Repository) GetSystem(ctx context.Context) (*SystemLifecycle, error) {
	var system SystemLifecycle
	var lastError, evidence *string
	err := r.db.QueryRow(ctx, `SELECT state, legacy_envelopes_disabled_at,
		legacy_adoption_evidence, last_error, updated_at
		FROM system_lifecycle WHERE singleton = TRUE`).Scan(
		&system.State, &system.LegacyEnvelopesDisabledAt, &evidence, &lastError, &system.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get system lifecycle: %w", err)
	}
	if lastError != nil {
		system.LastError = *lastError
	}
	if evidence != nil {
		system.LegacyAdoptionEvidence = *evidence
	}
	return &system, nil
}

// StartReset marks the application-wide reset as in progress. It is durable so
// a crash mid-reset does not reopen the system to writers on restart.
func (r *Repository) StartReset(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `UPDATE system_lifecycle
		SET state = 'resetting', last_error = NULL, updated_at = NOW()
		WHERE singleton = TRUE`)
	if err != nil {
		return fmt.Errorf("start system reset: %w", err)
	}
	return nil
}

// CompleteReset reopens the system, and only from the resetting state — a
// stray call must not clear a failure nobody has looked at.
func (r *Repository) CompleteReset(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `UPDATE system_lifecycle
		SET state = 'active', last_error = NULL, updated_at = NOW()
		WHERE singleton = TRUE AND state = 'resetting'`)
	if err != nil {
		return fmt.Errorf("complete system reset: %w", err)
	}
	return nil
}

// RecordResetError leaves the system durably closed with a diagnostic, so the
// operator sees why a reset stopped rather than a silently reopened system.
func (r *Repository) RecordResetError(ctx context.Context, message string) error {
	_, err := r.db.Exec(ctx, `UPDATE system_lifecycle SET last_error = $1,
		updated_at = NOW() WHERE singleton = TRUE AND state = 'resetting'`, message)
	if err != nil {
		return fmt.Errorf("record system reset error: %w", err)
	}
	return nil
}

// DisableLegacy atomically closes every legacy envelope allowance once.
func (r *Repository) DisableLegacy(ctx context.Context, adoptionEvidence string, at time.Time) (bool, error) {
	if adoptionEvidence == "" {
		return false, ErrAdoptionEvidence
	}
	var changed bool
	err := pgx.BeginFunc(ctx, r.db, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE system_lifecycle
			SET legacy_envelopes_disabled_at = $1, legacy_adoption_evidence = $2, updated_at = NOW()
			WHERE singleton = TRUE AND legacy_envelopes_disabled_at IS NULL`, at, adoptionEvidence)
		if err != nil {
			return fmt.Errorf("close global legacy gate: %w", err)
		}
		changed = tag.RowsAffected() == 1
		if !changed {
			return nil
		}
		if _, err = tx.Exec(ctx, `UPDATE namespace_lifecycles
			SET legacy_messages_allowed = FALSE, updated_at = NOW()
			WHERE legacy_messages_allowed = TRUE`); err != nil {
			return fmt.Errorf("close per-namespace legacy gates: %w", err)
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("disable legacy envelopes: %w", err)
	}
	return changed, nil
}

// ListCleanupCandidates returns old or deleted physical generations in stable,
// bounded order. Cleanup is idempotent, so retries are safe.
func (r *Repository) ListCleanupCandidates(ctx context.Context, limit int) ([]CleanupCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT lifecycle.namespace, generations.generation
		FROM namespace_lifecycles lifecycle
		CROSS JOIN LATERAL generate_series(
			1,
			CASE WHEN lifecycle.state = 'deleted' THEN lifecycle.generation
				ELSE lifecycle.generation - 1 END
		) AS generations(generation)
		ORDER BY lifecycle.namespace, generations.generation
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list lifecycle cleanup candidates: %w", err)
	}
	defer rows.Close()
	var out []CleanupCandidate
	for rows.Next() {
		var candidate CleanupCandidate
		if err := rows.Scan(&candidate.Namespace, &candidate.Generation); err != nil {
			return nil, fmt.Errorf("scan cleanup candidate: %w", err)
		}
		out = append(out, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cleanup candidates: %w", err)
	}
	return out, nil
}

// ListNonDeleted returns every lifecycle that reset must resume, including
// rows already in deleting after a prior partial attempt.
func (r *Repository) ListNonDeleted(ctx context.Context) ([]*NamespaceLifecycle, error) {
	rows, err := r.db.Query(ctx, `SELECT namespace, generation, state, activated_at,
		legacy_messages_allowed, last_error, updated_at
		FROM namespace_lifecycles WHERE state <> 'deleted' ORDER BY namespace`)
	if err != nil {
		return nil, fmt.Errorf("list non-deleted lifecycles: %w", err)
	}
	defer rows.Close()
	var out []*NamespaceLifecycle
	for rows.Next() {
		lifecycle, err := scanNamespace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, lifecycle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate non-deleted lifecycles: %w", err)
	}
	return out, nil
}
