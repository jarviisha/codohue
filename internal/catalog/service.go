package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

// catalogRepository abstracts Repository for tests.
type catalogRepository interface {
	Upsert(ctx context.Context, namespace, objectID, content string, contentHash []byte, metadata map[string]any) (*UpsertResult, error)
}

// nsConfigGetter abstracts nsconfig.Service.Get for tests.
type nsConfigGetter interface {
	Get(ctx context.Context, namespace string) (*namespace.Config, error)
}

// objectAuthorWriter records an object's author. Satisfied by
// objects.Service in cmd/api — declared here as an interface because the
// import rule forbids catalog from importing a peer domain directly.
type objectAuthorWriter interface {
	SetAuthor(ctx context.Context, namespace, objectID, authorSubjectID string) error
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
}

// SetAuthorWriter wires the objects domain in. The wiring layer calls this
// once at startup; when unset, catalog ingest simply drops author_subject_id.
func (s *Service) SetAuthorWriter(w objectAuthorWriter) { s.authorWriter = w }

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

	cfg, err := s.nsConfigSvc.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("load namespace config: %w", err)
	}
	if cfg == nil {
		return nil, ErrNamespaceNotFound
	}
	if cfg.DenseSource != "catalog" {
		return nil, ErrNamespaceNotEnabled
	}

	if cfg.CatalogMaxContentBytes > 0 && len(req.Content) > cfg.CatalogMaxContentBytes {
		return nil, fmt.Errorf("%w: limit=%d got=%d", ErrContentTooLarge,
			cfg.CatalogMaxContentBytes, len(req.Content))
	}

	hash := ContentHash(req.Content)

	res, err := s.repo.Upsert(ctx, ns, req.ObjectID, req.Content, hash, req.Metadata)
	if err != nil {
		return nil, fmt.Errorf("persist catalog item: %w", err)
	}

	// Attribution lives in the objects table, not here, so that it works for
	// every dense_source. Written through an injected interface because the
	// import rule forbids reaching into a peer domain; cmd/api supplies the
	// real implementation. A failure is logged, not fatal: the catalog row is
	// already durable and attribution is not embedding input.
	if s.authorWriter != nil && req.AuthorSubjectID != "" {
		if err := s.authorWriter.SetAuthor(ctx, ns, req.ObjectID, req.AuthorSubjectID); err != nil {
			slog.WarnContext(ctx, "catalog ingest: could not record object author",
				slog.String("namespace", ns),
				slog.String("object_id", req.ObjectID),
				slog.String("error", err.Error()),
			)
		}
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

// streamName returns the per-namespace embed stream name. Per data-model.md
// §5: catalog:embed:{namespace}.
func streamName(ns string) string { return "catalog:embed:" + ns }

// catalogStreamMaxLen caps the embed stream via approximate XADD MAXLEN.
// XACK never deletes entries, so without a cap the stream grows one entry
// per ingest forever. Keep in sync with the same constant in internal/admin
// and internal/embedder (peer-domain imports are forbidden).
const catalogStreamMaxLen = 100_000

func (s *Service) publish(ctx context.Context, ns string, item *Item, cfg *namespace.Config) error {
	args := &redis.XAddArgs{
		Stream: streamName(ns),
		MaxLen: catalogStreamMaxLen,
		Approx: true,
		Values: map[string]any{
			"catalog_item_id":  item.ID,
			"namespace":        ns,
			"object_id":        item.ObjectID,
			"strategy_id":      cfg.CatalogStrategyID,
			"strategy_version": cfg.CatalogStrategyVersion,
			"enqueued_at":      s.clock().UTC().Format(time.RFC3339Nano),
		},
	}
	if err := s.publisher.XAdd(ctx, args).Err(); err != nil {
		return fmt.Errorf("xadd %s: %w", args.Stream, err)
	}
	return nil
}
