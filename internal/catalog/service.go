package catalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// catalogRepository abstracts Repository for tests.
type catalogRepository interface {
	Upsert(ctx context.Context, namespace, objectID, content string, contentHash []byte, metadata map[string]any) (*UpsertResult, error)
	UpsertWithAttribution(ctx context.Context, namespace, objectID, content string, contentHash []byte, metadata map[string]any, writeAuthor AttributionWriter) (*UpsertResult, error)
	ListObjects(ctx context.Context, namespace string, changedSince *time.Time, limit, offset int, cursor *objectCursor) ([]ObjectRow, int, error)
}

// nsConfigGetter abstracts nsconfig.Service.Get for tests.
type nsConfigGetter interface {
	Get(ctx context.Context, namespace string) (*namespace.Config, error)
}

type lifecycleWriter interface {
	WithWriter(context.Context, string, func(context.Context, *nslifecycle.NamespaceLifecycle) error) error
}

// objectAuthorWriter records an object's author. Satisfied by
// objects.Service in cmd/api — declared here as an interface because the
// import rule forbids catalog from importing a peer domain directly.
type objectAuthorWriter interface {
	// SetAuthorTx runs inside the catalog row's transaction so content and
	// attribution commit together.
	SetAuthorTx(ctx context.Context, tx pgx.Tx, namespace, objectID, authorSubjectID string) error
}

// xAdder abstracts the Redis client's XAdd method so the service can be
// unit-tested without a real Redis. The signature matches *redis.Client.XAdd.
type xAdder interface {
	XAdd(ctx context.Context, args *redis.XAddArgs) *redis.StringCmd
}

// Service validates incoming catalog ingest requests, persists them to the
// catalog_items table, and publishes pending items to the per-namespace
// Redis Stream catalog:embed:{ns} for the embedder worker to consume.
type Service struct {
	repo         catalogRepository
	nsConfigSvc  nsConfigGetter
	publisher    xAdder
	authorWriter objectAuthorWriter // optional; attribution is skipped when nil
	clock        func() time.Time
	lifecycle    lifecycleWriter

	// defaultMaxContentBytes is the global content cap applied when the
	// namespace carries no override (NULL column). Wired from
	// CODOHUE_CATALOG_MAX_CONTENT_BYTES by cmd/api; 0 disables the fallback.
	defaultMaxContentBytes int
}

// SetDefaultMaxContentBytes wires the global content cap. Call once at
// startup, before serving.
func (s *Service) SetDefaultMaxContentBytes(n int) { s.defaultMaxContentBytes = n }

// SetAuthorWriter wires the objects domain in. The wiring layer calls this
// once at startup; when unset, catalog ingest simply drops author_subject_id.
func (s *Service) SetAuthorWriter(w objectAuthorWriter) { s.authorWriter = w }

// SetLifecycleWriter fences catalog persistence and its embed-stream publish.
func (s *Service) SetLifecycleWriter(writer lifecycleWriter) { s.lifecycle = writer }

// NewService creates a Service with the given dependencies. The publisher
// is typically the process-wide *redis.Client; pass any implementation of
// xAdder in tests. clock is provided so tests can pin timestamps; production
// callers can pass time.Now or NewServiceWithDefaults.
func NewService(repo *Repository, nsConfigSvc nsConfigGetter, publisher xAdder) *Service {
	return &Service{
		repo:        repo,
		nsConfigSvc: nsConfigSvc,
		publisher:   publisher,
		clock:       time.Now,
	}
}

// Ingest validates, persists, and conditionally publishes the catalog item
// described by req. It returns the resulting Item regardless of
// whether a publish was needed.
//
// The namespace argument is taken from the URL path (single source of truth
// per the 003 RESTful redesign convention); any namespace value in req is
// ignored at the handler layer before reaching this service.
func (s *Service) Ingest(ctx context.Context, ns string, req *IngestRequest) (*Item, error) {
	if ns == "" {
		return nil, fmt.Errorf("%w: namespace is required", ErrInvalidRequest)
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request body is required", ErrInvalidRequest)
	}
	if req.ObjectID == "" {
		return nil, fmt.Errorf("%w: object_id is required", ErrInvalidRequest)
	}

	trimmed := strings.TrimSpace(req.Content)
	if trimmed == "" {
		return nil, ErrEmptyContent
	}

	if s.lifecycle != nil && nslifecycle.RequireNamespaceLease(ctx, ns) != nil {
		var item *Item
		err := s.lifecycle.WithWriter(ctx, ns, func(leased context.Context, _ *nslifecycle.NamespaceLifecycle) error {
			var ingestErr error
			item, ingestErr = s.ingestActive(leased, ns, req)
			return ingestErr
		})
		return item, err
	}
	return s.ingestActive(ctx, ns, req)
}

func (s *Service) ingestActive(ctx context.Context, ns string, req *IngestRequest) (*Item, error) {
	cfg, err := s.nsConfigSvc.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("load namespace config: %w", err)
	}
	if cfg == nil {
		return nil, ErrNamespaceNotFound
	}
	if cfg.DenseSource != codohuetypes.DenseSourceCatalog {
		return nil, ErrNamespaceNotEnabled
	}

	maxContentBytes := cfg.CatalogMaxContentBytes
	if maxContentBytes <= 0 {
		// Namespace has no override (NULL column) — fall back to the global
		// default injected from CODOHUE_CATALOG_MAX_CONTENT_BYTES.
		maxContentBytes = s.defaultMaxContentBytes
	}
	if maxContentBytes > 0 && len(req.Content) > maxContentBytes {
		return nil, fmt.Errorf("%w: limit=%d got=%d", ErrContentTooLarge,
			maxContentBytes, len(req.Content))
	}

	hash := ContentHash(req.Content)

	// Attribution lives in the objects table, not here, so that it works for
	// every dense_source. It is written through an injected interface because
	// the import rule forbids reaching into a peer domain, and inside the same
	// transaction as the content so a request cannot report success having
	// stored only half of what it was given.
	//
	// A re-ingest of identical content still updates attribution: the content
	// write is a no-op upsert (NeedsPublish stays false, so no duplicate embed
	// work) while the author row is rewritten.
	var writeAuthor AttributionWriter
	if s.authorWriter != nil && req.AuthorSubjectID != "" {
		writeAuthor = func(ctx context.Context, tx pgx.Tx) error {
			return s.authorWriter.SetAuthorTx(ctx, tx, ns, req.ObjectID, req.AuthorSubjectID)
		}
	}

	res, err := s.repo.UpsertWithAttribution(ctx, ns, req.ObjectID, req.Content, hash, req.Metadata, writeAuthor)
	if err != nil {
		return nil, fmt.Errorf("persist catalog item: %w", err)
	}

	if res.NeedsPublish {
		if err := s.publish(ctx, ns, res.Item, cfg); err != nil {
			// Persistence already succeeded; the recovery sweep in the
			// embedder will eventually re-publish the row. Surface the error
			// to caller for observability but do NOT roll back the row.
			slog.WarnContext(ctx, "catalog publish to redis failed; row will be picked up by recovery sweep",
				slog.String("namespace", ns),
				slog.String("object_id", req.ObjectID),
				slog.Int64("catalog_item_id", res.Item.ID),
				slog.String("error", err.Error()),
			)
			return res.Item, fmt.Errorf("publish to embed stream: %w", err)
		}
		slog.DebugContext(ctx, "catalog item accepted and queued",
			slog.String("namespace", ns),
			slog.String("object_id", req.ObjectID),
			slog.Int64("catalog_item_id", res.Item.ID),
		)
	} else {
		slog.DebugContext(ctx, "catalog item idempotent re-ingest (no publish)",
			slog.String("namespace", ns),
			slog.String("object_id", req.ObjectID),
			slog.Int64("catalog_item_id", res.Item.ID),
		)
	}

	return res.Item, nil
}

// IngestBatch runs the single-item ingest for every entry of a batch and
// reports per-item outcomes, so one invalid item does not fail the rest.
// Namespace-level failures (namespace missing / catalog mode off) abort the
// whole batch instead — they are identical for every item, and a partial
// response would just repeat the same error N times.
func (s *Service) IngestBatch(ctx context.Context, ns string, req *BatchIngestRequest) (*codohuetypes.CatalogBatchIngestResponse, error) {
	if req == nil || len(req.Items) == 0 {
		return nil, fmt.Errorf("%w: items is required", ErrInvalidRequest)
	}
	if len(req.Items) > codohuetypes.CatalogBatchMaxItems {
		return nil, fmt.Errorf("%w: at most %d items per batch, got %d",
			ErrInvalidRequest, codohuetypes.CatalogBatchMaxItems, len(req.Items))
	}

	resp := &codohuetypes.CatalogBatchIngestResponse{
		Namespace: ns,
		Results:   make([]codohuetypes.CatalogBatchItemResult, 0, len(req.Items)),
	}
	for i := range req.Items {
		item := req.Items[i]
		_, err := s.Ingest(ctx, ns, &item)
		switch {
		case err == nil:
			resp.Accepted++
			resp.Results = append(resp.Results, codohuetypes.CatalogBatchItemResult{ObjectID: item.ObjectID, Accepted: true})
		case errors.Is(err, ErrNamespaceNotFound), errors.Is(err, ErrNamespaceNotEnabled):
			return nil, err
		default:
			resp.Rejected++
			resp.Results = append(resp.Results, codohuetypes.CatalogBatchItemResult{
				ObjectID: item.ObjectID, Accepted: false, Error: itemErrorCode(err),
			})
		}
	}
	return resp, nil
}

// itemErrorCode maps a per-item ingest error to the machine-readable code the
// single-item endpoint uses, so batch consumers reuse one error vocabulary.
func itemErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrEmptyContent):
		return "empty_content"
	case errors.Is(err, ErrContentTooLarge):
		return "content_too_large"
	case errors.Is(err, ErrInvalidRequest):
		return "invalid_request"
	default:
		return "internal_error"
	}
}

// ListObjects is the data-plane reconciliation read: which object ids does
// this namespace already hold (optionally: changed since a timestamp). A
// repair pass diffs this against its own corpus and re-sends only the gap.
func (s *Service) ListObjects(ctx context.Context, ns string, changedSince *time.Time, limit, offset int) (*codohuetypes.CatalogObjectsResponse, error) {
	return s.ListObjectsPage(ctx, ns, changedSince, limit, offset, "")
}

// ListObjectsPage supports the versioned keyset cursor while retaining the
// legacy offset argument for one compatibility window.
func (s *Service) ListObjectsPage(ctx context.Context, ns string, changedSince *time.Time, limit, offset int, rawCursor string) (*codohuetypes.CatalogObjectsResponse, error) {
	if ns == "" {
		return nil, fmt.Errorf("%w: namespace is required", ErrInvalidRequest)
	}
	cfg, err := s.nsConfigSvc.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("load namespace config: %w", err)
	}
	if cfg == nil {
		return nil, ErrNamespaceNotFound
	}

	sinceKey := ""
	if changedSince != nil {
		sinceKey = changedSince.UTC().Format(time.RFC3339Nano)
	}
	cursor, err := decodeObjectCursor(rawCursor, ns, sinceKey)
	if err != nil {
		return nil, err
	}
	if cursor != nil && offset != 0 {
		return nil, fmt.Errorf("%w: cursor and offset cannot be combined", ErrInvalidRequest)
	}
	rows, total, err := s.repo.ListObjects(ctx, ns, changedSince, limit, offset, cursor)
	if err != nil {
		return nil, fmt.Errorf("list catalog objects: %w", err)
	}
	resp := &codohuetypes.CatalogObjectsResponse{
		Namespace: ns,
		Items:     make([]codohuetypes.CatalogObjectSummary, len(rows)),
		Total:     total,
		Limit:     limit,
		Offset:    offset,
	}
	for i, row := range rows {
		resp.Items[i] = codohuetypes.CatalogObjectSummary{
			ObjectID:  row.ObjectID,
			UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339),
		}
	}
	if len(rows) == limit && len(rows) > 0 {
		last := rows[len(rows)-1]
		resp.NextCursor, err = encodeObjectCursor(objectCursor{Version: 1, Namespace: ns, ChangedSince: sinceKey, UpdatedAt: last.UpdatedAt.UTC(), ID: last.ID})
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}

// streamName returns the per-namespace embed stream name. Per data-model.md
// §5: catalog:embed:{namespace}.
func streamName(ns string) string { return "catalog:embed:" + ns }

func generationStreamName(ns string, generation int64) string {
	if generation < 1 {
		generation = 1
	}
	return nslifecycle.MustPhysicalName(nslifecycle.KindEmbedStream, ns, generation)
}

func (s *Service) publish(ctx context.Context, ns string, item *Item, cfg *namespace.Config) error {
	args := &redis.XAddArgs{
		Stream: generationStreamName(ns, cfg.Generation),
		Values: map[string]any{
			"catalog_item_id":      item.ID,
			"namespace":            ns,
			"namespace_generation": cfg.Generation,
			"object_id":            item.ObjectID,
			"strategy_id":          cfg.CatalogStrategyID,
			"strategy_version":     cfg.CatalogStrategyVersion,
			"enqueued_at":          s.clock().UTC().Format(time.RFC3339Nano),
		},
	}
	if err := s.publisher.XAdd(ctx, args).Err(); err != nil {
		return fmt.Errorf("xadd %s: %w", args.Stream, err)
	}
	return nil
}
