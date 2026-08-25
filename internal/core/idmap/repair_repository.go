package idmap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RepairRepository persists the reconciliation manifest.
//
// It is a separate type from Repository because the two have opposite
// contracts: Repository serves the hot path and refuses to mint a mapping
// without a namespace lease, while this one runs under the global exclusive
// lease with every writer frozen.
type RepairRepository struct {
	db *pgxpool.Pool
}

// NewRepairRepository creates a repository over the pool.
func NewRepairRepository(db *pgxpool.Pool) *RepairRepository {
	return &RepairRepository{db: db}
}

// ManifestHash is the immutable fingerprint of an audited item set.
//
// It covers the decision inputs and the decision itself — identity, every
// observed numeric id, and the selected target — so any change to what the
// operator reviewed changes the hash. Items are sorted first because map or
// query order must not affect it.
func ManifestHash(items []RepairItem) string {
	keys := make([]string, 0, len(items))
	for _, item := range items {
		target := "none"
		if item.TargetNumericID != nil {
			target = fmt.Sprint(*item.TargetNumericID)
		}
		olds := append([]int64(nil), item.OldNumericIDs...)
		sort.Slice(olds, func(i, j int) bool { return olds[i] < olds[j] })
		oldParts := make([]string, len(olds))
		for i, old := range olds {
			oldParts[i] = fmt.Sprint(old)
		}
		keys = append(keys, strings.Join([]string{
			item.Namespace, item.EntityType, item.StringID,
			strings.Join(oldParts, ","), target, string(item.State),
		}, "\x1f"))
	}
	sort.Strings(keys)

	sum := sha256.Sum256([]byte(strings.Join(keys, "\x1e")))
	return hex.EncodeToString(sum[:])
}

// CreateRun opens a reconciliation and stores its audited manifest in one
// transaction: a run whose items landed without the run row (or vice versa)
// would be unresumable.
func (r *RepairRepository) CreateRun(ctx context.Context, items []RepairItem, startedAt time.Time) (*RepairRun, error) {
	var run RepairRun
	err := beginFunc(ctx, r.db, func(tx pgx.Tx) error {
		hash := ManifestHash(items)
		err := tx.QueryRow(ctx, `
			INSERT INTO id_mapping_repair_runs (state, manifest_hash, started_at)
			VALUES ('audited', $1, $2)
			RETURNING id, state, COALESCE(pg_snapshot_ref, ''), qdrant_snapshot_refs,
			          manifest_hash, rebuilt_namespaces, started_at, completed_at, COALESCE(error, '')`,
			hash, startedAt,
		).Scan(&run.ID, &run.State, &run.PGSnapshotRef, &run.QdrantSnapshotRefs,
			&run.ManifestHash, &run.RebuiltNamespaces, &run.StartedAt, &run.CompletedAt, &run.Error)
		if err != nil {
			if isUniqueViolation(err) {
				return ErrRepairRunActive
			}
			return fmt.Errorf("insert repair run: %w", err)
		}
		for i := range items {
			items[i].RunID = run.ID
			if err := insertItem(ctx, tx, items[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &run, nil
}

func insertItem(ctx context.Context, tx pgx.Tx, item RepairItem) error {
	sources, err := json.Marshal(item.Sources)
	if err != nil {
		return fmt.Errorf("marshal repair item sources: %w", err)
	}
	if item.Sources == nil {
		sources = []byte(`{}`)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO id_mapping_repair_items (
			run_id, namespace, entity_type, string_id, old_numeric_ids,
			target_numeric_id, sources, payload_hash, vector_hash, state, error, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), NULLIF($9, ''), $10, NULLIF($11, ''), NOW())`,
		item.RunID, item.Namespace, item.EntityType, item.StringID, item.OldNumericIDs,
		item.TargetNumericID, sources, item.PayloadHash, item.VectorHash, string(item.State), item.Error,
	); err != nil {
		return fmt.Errorf("insert repair item %s/%s/%s: %w", item.Namespace, item.EntityType, item.StringID, err)
	}
	return nil
}

// GetRun loads one run.
func (r *RepairRepository) GetRun(ctx context.Context, id int64) (*RepairRun, error) {
	var run RepairRun
	err := r.db.QueryRow(ctx, `
		SELECT id, state, COALESCE(pg_snapshot_ref, ''), qdrant_snapshot_refs,
		       manifest_hash, rebuilt_namespaces, started_at, completed_at, COALESCE(error, '')
		FROM id_mapping_repair_runs WHERE id = $1`, id,
	).Scan(&run.ID, &run.State, &run.PGSnapshotRef, &run.QdrantSnapshotRefs,
		&run.ManifestHash, &run.RebuiltNamespaces, &run.StartedAt, &run.CompletedAt, &run.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRepairRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get repair run %d: %w", id, err)
	}
	return &run, nil
}

// ActiveRun returns the in-flight run, or (nil, nil) when there is none.
func (r *RepairRepository) ActiveRun(ctx context.Context) (*RepairRun, error) {
	var run RepairRun
	err := r.db.QueryRow(ctx, `
		SELECT id, state, COALESCE(pg_snapshot_ref, ''), qdrant_snapshot_refs,
		       manifest_hash, rebuilt_namespaces, started_at, completed_at, COALESCE(error, '')
		FROM id_mapping_repair_runs
		WHERE state NOT IN ('complete', 'failed')`,
	).Scan(&run.ID, &run.State, &run.PGSnapshotRef, &run.QdrantSnapshotRefs,
		&run.ManifestHash, &run.RebuiltNamespaces, &run.StartedAt, &run.CompletedAt, &run.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active repair run: %w", err)
	}
	return &run, nil
}

// RecordRebuiltNamespace appends a namespace to the run's rebuild record.
//
// Appended one at a time, immediately after each rebuild succeeds, so an
// interrupted apply leaves an accurate partial record instead of an all-or-
// nothing claim.
func (r *RepairRepository) RecordRebuiltNamespace(ctx context.Context, id int64, namespace string) error {
	if _, err := r.db.Exec(ctx, `
		UPDATE id_mapping_repair_runs
		SET rebuilt_namespaces = (
			SELECT jsonb_agg(DISTINCT value)
			FROM jsonb_array_elements_text(rebuilt_namespaces || to_jsonb($2::text)) AS value
		)
		WHERE id = $1`, id, namespace); err != nil {
		return fmt.Errorf("record rebuilt namespace %q: %w", namespace, err)
	}
	return nil
}

// SetRunState advances the run. errMessage is empty on success paths.
func (r *RepairRepository) SetRunState(ctx context.Context, id int64, state RepairRunState, errMessage string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE id_mapping_repair_runs
		SET state = $2,
		    error = NULLIF($3, ''),
		    completed_at = CASE WHEN $2 IN ('complete', 'failed') THEN NOW() ELSE completed_at END
		WHERE id = $1`, id, string(state), errMessage)
	if err != nil {
		return fmt.Errorf("set repair run %d state: %w", id, err)
	}
	return nil
}

// RecordSnapshots stores the coordinated backup references taken before apply.
// Both stores are recorded together because a recovery that restores only one
// of them reintroduces exactly the cross-store divergence being repaired.
func (r *RepairRepository) RecordSnapshots(ctx context.Context, id int64, pgRef string, qdrantRefs map[string]string) error {
	refs, err := json.Marshal(qdrantRefs)
	if err != nil {
		return fmt.Errorf("marshal qdrant snapshot refs: %w", err)
	}
	if _, err := r.db.Exec(ctx, `
		UPDATE id_mapping_repair_runs
		SET pg_snapshot_ref = NULLIF($2, ''), qdrant_snapshot_refs = $3
		WHERE id = $1`, id, pgRef, refs); err != nil {
		return fmt.Errorf("record repair snapshots: %w", err)
	}
	return nil
}

// ListItems returns the run's manifest ordered deterministically, so a resumed
// run walks the same sequence the audit reviewed.
func (r *RepairRepository) ListItems(ctx context.Context, runID int64) ([]RepairItem, error) {
	return r.queryItems(ctx, `
		SELECT run_id, namespace, entity_type, string_id, old_numeric_ids,
		       target_numeric_id, sources, COALESCE(payload_hash, ''), COALESCE(vector_hash, ''),
		       state, COALESCE(error, '')
		FROM id_mapping_repair_items
		WHERE run_id = $1
		ORDER BY namespace, entity_type, string_id`, runID)
}

// ListItemsInState powers both resume ("what is still pending") and the
// quarantine report ("what stopped this run").
func (r *RepairRepository) ListItemsInState(ctx context.Context, runID int64, states ...RepairItemState) ([]RepairItem, error) {
	raw := make([]string, len(states))
	for i, state := range states {
		raw[i] = string(state)
	}
	return r.queryItems(ctx, `
		SELECT run_id, namespace, entity_type, string_id, old_numeric_ids,
		       target_numeric_id, sources, COALESCE(payload_hash, ''), COALESCE(vector_hash, ''),
		       state, COALESCE(error, '')
		FROM id_mapping_repair_items
		WHERE run_id = $1 AND state = ANY($2)
		ORDER BY namespace, entity_type, string_id`, runID, raw)
}

func (r *RepairRepository) queryItems(ctx context.Context, sql string, args ...any) ([]RepairItem, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query repair items: %w", err)
	}
	defer rows.Close()

	var out []RepairItem
	for rows.Next() {
		var item RepairItem
		var sources []byte
		if err := rows.Scan(&item.RunID, &item.Namespace, &item.EntityType, &item.StringID,
			&item.OldNumericIDs, &item.TargetNumericID, &sources,
			&item.PayloadHash, &item.VectorHash, &item.State, &item.Error); err != nil {
			return nil, fmt.Errorf("scan repair item: %w", err)
		}
		if len(sources) > 0 {
			if err := json.Unmarshal(sources, &item.Sources); err != nil {
				return nil, fmt.Errorf("unmarshal repair item sources: %w", err)
			}
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate repair items: %w", err)
	}
	return out, nil
}

// SetItemState records per-item progress so a crash resumes mid-manifest
// rather than restarting a partially-applied run.
func (r *RepairRepository) SetItemState(ctx context.Context, item RepairItem, state RepairItemState, errMessage string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE id_mapping_repair_items
		SET state = $5, error = NULLIF($6, ''), updated_at = NOW()
		WHERE run_id = $1 AND namespace = $2 AND entity_type = $3 AND string_id = $4`,
		item.RunID, item.Namespace, item.EntityType, item.StringID, string(state), errMessage)
	if err != nil {
		return fmt.Errorf("set repair item state: %w", err)
	}
	return nil
}

// RetargetMapping points the logical identity at its authoritative numeric id.
//
// Deliberately not GetOrCreate: this runs under the global exclusive lease
// with every writer frozen, and it must move an EXISTING mapping rather than
// mint a parallel one.
func (r *RepairRepository) RetargetMapping(ctx context.Context, namespace, entityType, stringID string, numericID int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE id_mappings SET numeric_id = $4
		WHERE namespace = $1 AND entity_type = $2 AND string_id = $3`,
		namespace, entityType, stringID, numericID)
	if err != nil {
		return fmt.Errorf("retarget mapping %s/%s/%s: %w", namespace, entityType, stringID, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("retarget mapping %s/%s/%s: no such mapping", namespace, entityType, stringID)
	}
	return nil
}

// LoadMappings returns every mapping for a namespace, which is the PostgreSQL
// half of the audit's evidence.
func (r *RepairRepository) LoadMappings(ctx context.Context, namespace string) (map[string]map[string]int64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT entity_type, string_id, numeric_id
		FROM id_mappings WHERE namespace = $1
		ORDER BY entity_type, string_id`, namespace)
	if err != nil {
		return nil, fmt.Errorf("load mappings for %q: %w", namespace, err)
	}
	defer rows.Close()

	out := map[string]map[string]int64{}
	for rows.Next() {
		var entityType, stringID string
		var numericID int64
		if err := rows.Scan(&entityType, &stringID, &numericID); err != nil {
			return nil, fmt.Errorf("scan mapping: %w", err)
		}
		if out[entityType] == nil {
			out[entityType] = map[string]int64{}
		}
		out[entityType][stringID] = numericID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mappings: %w", err)
	}
	return out, nil
}

// NextNumericID reserves a fresh numeric id from the same sequence the hot
// path uses, so a repaired identity cannot collide with one minted later.
func (r *RepairRepository) NextNumericID(ctx context.Context) (int64, error) {
	var id int64
	if err := r.db.QueryRow(ctx, `SELECT nextval(pg_get_serial_sequence('id_mappings', 'numeric_id'))`).Scan(&id); err != nil {
		return 0, fmt.Errorf("reserve numeric id: %w", err)
	}
	return id, nil
}

// beginFunc wraps pgx.BeginFunc so the transaction error carries this
// package's context rather than bare pgx wording.
func beginFunc(ctx context.Context, db *pgxpool.Pool, fn func(pgx.Tx) error) error {
	if err := pgx.BeginFunc(ctx, db, fn); err != nil {
		return fmt.Errorf("repair manifest transaction: %w", err)
	}
	return nil
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}
