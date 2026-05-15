package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// Sentinel errors surfaced by the catalog operator endpoints.
var (
	// ErrReembedAlreadyRunning indicates an unfinished re-embed batch_run_logs
	// row exists for the namespace. Handler maps to 409 Conflict.
	ErrReembedAlreadyRunning = errors.New("admin: re-embed already in progress for namespace")

	// ErrCatalogNotEnabled indicates the namespace's catalog_enabled flag is
	// false. Handler maps to 404 (same body as namespace-not-found per FR-008
	// to avoid leaking namespace existence to unauthenticated probes).
	ErrCatalogNotEnabled = errors.New("admin: namespace catalog auto-embedding not enabled")

	// ErrCatalogStrategyPickerUnavailable indicates the wiring layer did not
	// install a catalogStrategyPicker — the catalog feature is not enabled in
	// this deployment. Handler maps to 503.
	ErrCatalogStrategyPickerUnavailable = errors.New("admin: catalog strategy picker not wired")
)

// qdrantClientPointDeleter wraps *qdrant.Client to satisfy qdrantPointDeleter.
type qdrantClientPointDeleter struct {
	c *qdrant.Client
}

func newQdrantClientPointDeleter(c *qdrant.Client) *qdrantClientPointDeleter {
	return &qdrantClientPointDeleter{c: c}
}

// DeletePoint removes a single point from the named Qdrant collection.
// A "collection not found" error is treated as a successful no-op so the
// operator-side delete is idempotent across both data planes.
func (q *qdrantClientPointDeleter) DeletePoint(ctx context.Context, collection string, numericID uint64) error {
	if q == nil || q.c == nil {
		return nil
	}
	_, err := q.c.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collection,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{
					Ids: []*qdrant.PointId{qdrant.NewIDNum(numericID)},
				},
			},
		},
	})
	if err != nil {
		if grpcstatus.Code(err) == codes.NotFound {
			return nil
		}
		return fmt.Errorf("delete from %q: %w", collection, err)
	}
	return nil
}

// streamName returns the per-namespace embed stream name. Mirrors the
// definition in internal/catalog so we keep the on-the-wire format aligned
// across packages without a forbidden cross-domain import.
func catalogStreamName(ns string) string { return "catalog:embed:" + ns }

// publishCatalogEnqueue writes one XADD to catalog:embed:{ns}. The payload
// matches what internal/catalog publishes on first ingest so the embedder
// worker decodes both with the same code path.
func (s *Service) publishCatalogEnqueue(ctx context.Context, ns string, target CatalogReembedTarget, strategyID, strategyVersion string) error {
	if s.streamPublisher == nil {
		return errors.New("admin: stream publisher not wired")
	}
	args := &goredis.XAddArgs{
		Stream: catalogStreamName(ns),
		Values: map[string]any{
			"catalog_item_id":  target.ID,
			"namespace":        ns,
			"object_id":        target.ObjectID,
			"strategy_id":      strategyID,
			"strategy_version": strategyVersion,
			"enqueued_at":      s.nowFn().UTC().Format(time.RFC3339Nano),
		},
	}
	if err := s.streamPublisher.XAdd(ctx, args).Err(); err != nil {
		return fmt.Errorf("xadd %s: %w", args.Stream, err)
	}
	return nil
}

// TriggerReEmbed kicks off a namespace-wide re-embed under the namespace's
// currently active (strategy_id, strategy_version). Behavior:
//
//   - 404 path  : namespace does not exist OR catalog_enabled=false. Returns
//     (nil, nil) — handler maps to 404.
//   - 409 path  : a running re-embed already exists for this namespace.
//     Returns ErrReembedAlreadyRunning.
//   - 503 path  : catalog strategy picker not wired. Returns
//     ErrCatalogStrategyPickerUnavailable.
//   - 202 path  : new batch_run_logs row inserted; stale items reset to
//     'pending'; one XADD per stale id; response carries the
//     batch run id and stale count.
//
// Best-effort guarantees:
//   - The DB reset is atomic via UPDATE ... RETURNING.
//   - XADD failures for individual items are logged but do not roll back the
//     reset. The embedder's recovery sweep eventually picks up any 'pending'
//     row that has no PEL entry.
func (s *Service) TriggerReEmbed(ctx context.Context, namespace string) (*CatalogReEmbedResponse, error) {
	if s.catalogPicker == nil {
		return nil, ErrCatalogStrategyPickerUnavailable
	}

	strategyID, strategyVersion, enabled, err := s.catalogPicker.GetCatalogStrategy(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get catalog strategy: %w", err)
	}
	if !enabled {
		// Both "namespace missing" and "catalog disabled" surface as 404.
		return nil, nil
	}
	if strategyVersion == "" {
		// Defensive: enabled=true with no version is a misconfiguration —
		// surface as 400 via the picker error path.
		return nil, fmt.Errorf("namespace %q has catalog_enabled but no strategy_version", namespace)
	}

	if _, busy := s.runningReembed.LoadOrStore(namespace, true); busy {
		return nil, ErrReembedAlreadyRunning
	}
	// Hand-off ownership: we keep the lock held until the function returns
	// (success or error). The actual embedding work is done async by the
	// embedder workers; the watcher closes the batch_run_logs row.
	defer s.runningReembed.Delete(namespace)

	// Belt-and-suspenders: a row may already be open in DB even if our
	// in-memory map is clear (process restart, multi-replica admin).
	if existing, err := s.repo.FindRunningReembedRun(ctx, namespace); err != nil {
		return nil, fmt.Errorf("check running reembed: %w", err)
	} else if existing != nil {
		return nil, fmt.Errorf("%w (batch_run_id=%d)", ErrReembedAlreadyRunning, existing.ID)
	}

	startedAt := s.nowFn().UTC()
	batchID, err := s.repo.InsertReembedRun(ctx, namespace, strategyID, strategyVersion, startedAt)
	if err != nil {
		return nil, fmt.Errorf("insert reembed run: %w", err)
	}

	targets, err := s.repo.SelectAndResetStaleCatalogItems(ctx, namespace, strategyVersion)
	if err != nil {
		return nil, fmt.Errorf("reset stale catalog items: %w", err)
	}

	for _, t := range targets {
		if err := s.publishCatalogEnqueue(ctx, namespace, t, strategyID, strategyVersion); err != nil {
			slog.WarnContext(ctx, "catalog reembed publish failed; recovery sweep will retry",
				slog.String("namespace", namespace),
				slog.Int64("catalog_item_id", t.ID),
				slog.String("error", err.Error()),
			)
		}
	}

	return &CatalogReEmbedResponse{
		BatchRunID:      batchID,
		Namespace:       namespace,
		StrategyID:      strategyID,
		StrategyVersion: strategyVersion,
		StaleItems:      len(targets),
		StartedAt:       startedAt,
	}, nil
}

// ListCatalogItems returns a paginated browse of catalog items for the admin UI.
func (s *Service) ListCatalogItems(ctx context.Context, namespace, state string, limit, offset int, objectIDFilter string) (*CatalogItemsListResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	items, total, err := s.repo.ListCatalogItems(ctx, namespace, state, limit, offset, objectIDFilter)
	if err != nil {
		return nil, fmt.Errorf("list catalog items: %w", err)
	}
	if items == nil {
		items = []CatalogItemSummary{}
	}
	return &CatalogItemsListResponse{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// GetCatalogItem returns the full record for one catalog item. nil result
// when the row is not found so the handler can map to 404.
func (s *Service) GetCatalogItem(ctx context.Context, namespace string, id int64) (*CatalogItemDetail, error) {
	item, err := s.repo.GetCatalogItem(ctx, namespace, id)
	if err != nil {
		return nil, fmt.Errorf("get catalog item: %w", err)
	}
	if item != nil {
		s.attachCatalogVector(ctx, item)
	}
	return item, nil
}

func (s *Service) attachCatalogVector(ctx context.Context, item *CatalogItemDetail) {
	if s.qdrantClient == nil {
		return
	}
	numericID, ok, err := s.repo.LookupNumericObjectID(ctx, item.Namespace, item.ObjectID)
	if err != nil || !ok {
		if err != nil {
			slog.WarnContext(ctx, "catalog item vector lookup failed",
				slog.String("namespace", item.Namespace),
				slog.String("object_id", item.ObjectID),
				slog.String("error", err.Error()),
			)
		}
		return
	}

	collection := item.Namespace + "_objects_dense"
	results, err := s.qdrantClient.Get(ctx, &qdrant.GetPoints{
		CollectionName: collection,
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(numericID)},
		WithVectors:    qdrant.NewWithVectorsInclude("dense_interactions"),
	})
	if err != nil || len(results) == 0 {
		if err != nil {
			slog.WarnContext(ctx, "catalog item vector fetch failed",
				slog.String("namespace", item.Namespace),
				slog.String("object_id", item.ObjectID),
				slog.String("collection", collection),
				slog.String("error", err.Error()),
			)
		}
		return
	}

	vec := results[0].GetVectors().GetVectors().GetVectors()["dense_interactions"]
	if vec == nil || vec.GetDense() == nil {
		return
	}
	values := vec.GetDense().GetData()
	item.Vector = &CatalogVector{
		Collection: collection,
		NumericID:  numericID,
		Dim:        len(values),
		Values:     values,
	}
}

// RedriveCatalogItem resets a single failed/dead-letter row to 'pending' and
// publishes one XADD entry. Returns:
//   - nil result + nil error: row not found OR not in failed/dead_letter state
//     (handler maps both to 404).
//   - response + nil error: success, returns 202 to caller.
//   - 503 sentinel: catalog feature not wired.
func (s *Service) RedriveCatalogItem(ctx context.Context, namespace string, id int64) (*CatalogRedriveResponse, error) {
	if s.catalogPicker == nil {
		return nil, ErrCatalogStrategyPickerUnavailable
	}

	strategyID, strategyVersion, enabled, err := s.catalogPicker.GetCatalogStrategy(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get catalog strategy: %w", err)
	}
	if !enabled {
		return nil, nil
	}

	item, err := s.repo.RedriveCatalogItem(ctx, namespace, id)
	if err != nil {
		return nil, fmt.Errorf("redrive catalog item: %w", err)
	}
	if item == nil {
		return nil, nil
	}

	if err := s.publishCatalogEnqueue(ctx, namespace,
		CatalogReembedTarget{ID: item.ID, ObjectID: item.ObjectID},
		strategyID, strategyVersion); err != nil {
		slog.WarnContext(ctx, "catalog redrive publish failed; recovery sweep will retry",
			slog.String("namespace", namespace),
			slog.Int64("catalog_item_id", item.ID),
			slog.String("error", err.Error()),
		)
	}
	return &CatalogRedriveResponse{
		ID:       item.ID,
		ObjectID: item.ObjectID,
		State:    item.State,
	}, nil
}

// BulkRedriveDeadletter resets every dead_letter row in the namespace to
// 'pending' and publishes one XADD per id. Used by SC-008 ("operator can
// retry every dead-letter item with one click").
func (s *Service) BulkRedriveDeadletter(ctx context.Context, namespace string) (*CatalogBulkRedriveResponse, error) {
	if s.catalogPicker == nil {
		return nil, ErrCatalogStrategyPickerUnavailable
	}

	strategyID, strategyVersion, enabled, err := s.catalogPicker.GetCatalogStrategy(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get catalog strategy: %w", err)
	}
	if !enabled {
		return nil, nil
	}

	targets, err := s.repo.BulkRedriveDeadletter(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("bulk redrive dead-letter: %w", err)
	}

	for _, t := range targets {
		if err := s.publishCatalogEnqueue(ctx, namespace, t, strategyID, strategyVersion); err != nil {
			slog.WarnContext(ctx, "catalog bulk redrive publish failed; recovery sweep will retry",
				slog.String("namespace", namespace),
				slog.Int64("catalog_item_id", t.ID),
				slog.String("error", err.Error()),
			)
		}
	}
	return &CatalogBulkRedriveResponse{
		Namespace: namespace,
		Redriven:  len(targets),
	}, nil
}

// DeleteCatalogItem removes a catalog item from Postgres and best-effort
// removes the matching object dense vector from Qdrant. The path is
// idempotent — deleting a non-existent item still returns 204 so retries
// are safe.
func (s *Service) DeleteCatalogItem(ctx context.Context, namespace string, id int64) error {
	objectID, found, err := s.repo.DeleteCatalogItem(ctx, namespace, id)
	if err != nil {
		return fmt.Errorf("delete catalog item: %w", err)
	}
	if !found {
		// Idempotent — also try to delete by id directly so callers cannot
		// break the API by referencing the row's id even after Postgres
		// already lost it. This branch only matters for retries.
		return nil
	}

	if s.qdrantDeleter == nil {
		return nil
	}
	numID, ok, err := s.repo.LookupNumericObjectID(ctx, namespace, objectID)
	if err != nil {
		slog.WarnContext(ctx, "catalog delete: numeric id lookup failed; postgres row already deleted",
			slog.String("namespace", namespace),
			slog.String("object_id", objectID),
			slog.String("error", err.Error()),
		)
		return nil
	}
	if !ok {
		return nil
	}
	if err := s.qdrantDeleter.DeletePoint(ctx, namespace+"_objects_dense", numID); err != nil {
		slog.WarnContext(ctx, "catalog delete: qdrant point removal failed; postgres row already deleted",
			slog.String("namespace", namespace),
			slog.String("object_id", objectID),
			slog.String("collection", namespace+"_objects_dense"),
			slog.String("error", err.Error()),
		)
	}
	return nil
}
