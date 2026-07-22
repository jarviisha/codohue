package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/batchrun"
)

// adminRepo is the repository interface used by Service.
type adminRepo interface {
	ListNamespaces(ctx context.Context) ([]NamespaceConfig, error)
	GetNamespace(ctx context.Context, namespace string) (*NamespaceConfig, error)
	GetBatchRunLogs(ctx context.Context, namespace, status, kind string, limit, offset int) ([]BatchRunLog, int, BatchRunStats, error)
	GetBatchRunByID(ctx context.Context, id int64) (*BatchRunLog, error)
	RequestCancel(ctx context.Context, id int64) (RequestCancelResult, error)
	GetBatchRunStats(ctx context.Context, windowSeconds, bucketSeconds int) ([]BatchRunStatsBucket, error)
	GetCatalogBacklogHistory(ctx context.Context, namespace string, windowSeconds int) ([]CatalogBacklogSample, error)
	GetCatalogFailuresSummary(ctx context.Context, namespace string, windowSeconds, limit int) ([]CatalogFailureReason, error)
	GetLastBatchRunPerNamespace(ctx context.Context) (map[string]BatchRunLog, error)
	GetRecentEventCounts(ctx context.Context, windowHours int) (map[string]int, error)
	ListSubjects(ctx context.Context, ns, prefix, sort string, limit, offset int) ([]SubjectListItem, int, error)
	GetAuthorCoverage(ctx context.Context, namespace string) (attributed, total int, err error)
	GetSubjectStats(ctx context.Context, namespace, subjectID string, seenItemsDays int) (*SubjectStats, error)
	GetRecentEvents(ctx context.Context, ns string, limit, offset int, subjectID string) ([]EventSummary, int, error)
	GetEventsSummary(ctx context.Context, ns string, windowSecs, bucketSecs int) (total int, byAction map[string]int, series []EventsSummaryBucket, err error)
	SeedDemoEvents(ctx context.Context, namespace string, events []demoEvent, now time.Time) (int, error)
	SeedDemoCatalogItems(ctx context.Context, namespace string, items []demoCatalogItem, now time.Time) (int, error)
	ClearNamespaceData(ctx context.Context, namespace string) (int, error)
	TruncateAllNamespaceData(ctx context.Context) (eventsDeleted, namespacesDeleted int, err error)

	// Catalog re-embed orchestration (US3).
	FindRunningReembedRun(ctx context.Context, namespace string) (*BatchRunLog, error)
	FindLatestReembedRun(ctx context.Context, namespace string) (*BatchRunLog, error)
	InsertReembedRun(ctx context.Context, namespace, strategyID, strategyVersion string, startedAt time.Time) (int64, error)
	StartReembedRun(ctx context.Context, namespace, strategyID, strategyVersion, onlyState string, startedAt time.Time) (batchRunID int64, targets []CatalogReembedTarget, err error)
	SelectAndResetStaleCatalogItems(ctx context.Context, namespace, targetStrategyVersion, onlyState string) ([]CatalogReembedTarget, error)

	// Catalog liveness signal for the admin Status tab.
	GetLastCatalogEmbeddedAt(ctx context.Context, namespace string) (*time.Time, error)

	// Catalog item operator endpoints (US3).
	ListCatalogItems(ctx context.Context, namespace, state string, limit, offset int, objectIDFilter, authorFilter string) ([]CatalogItemSummary, int, error)
	GetCatalogItem(ctx context.Context, namespace string, id int64) (*CatalogItemDetail, error)
	RedriveCatalogItem(ctx context.Context, namespace string, id int64) (*CatalogItemDetail, error)
	BulkRedriveDeadletter(ctx context.Context, namespace string) ([]CatalogReembedTarget, error)
	DeleteCatalogItem(ctx context.Context, namespace string, id int64) (string, bool, error)
	LookupNumericObjectID(ctx context.Context, namespace, objectID string) (uint64, bool, error)
}

// nsConfigUpserter is the abstract collaborator that creates or updates a namespace
// configuration. The wiring layer provides an adapter around the concrete
// namespace config service so the admin domain does not import it directly.
type nsConfigUpserter interface {
	Upsert(ctx context.Context, namespace string, req *NamespaceUpsertRequest) (*NamespaceUpsertResponse, error)
	// RotateAPIKey mints a replacement namespace key, invalidating the old
	// one immediately. Returns (nil, nil) when the namespace does not exist.
	RotateAPIKey(ctx context.Context, namespace string) (*NamespaceKeyRotateResponse, error)
}

// nsCatalogConfigurator owns the catalog-specific config update path. Wired
// by an adapter in cmd/admin so this domain need not import nsconfig or
// embedstrategy directly.
type nsCatalogConfigurator interface {
	// GetNamespace returns the current admin view of a namespace, including
	// its catalog config; nil when the namespace does not exist.
	GetCatalog(ctx context.Context, namespace string) (*NamespaceCatalogConfig, error)

	// UpdateCatalog applies the request and returns the resulting state.
	// The implementation is expected to translate dim-mismatch into
	// *CatalogDimensionMismatch and unknown-namespace into a nil result.
	UpdateCatalog(ctx context.Context, namespace string, req *NamespaceCatalogUpdateRequest) (*NamespaceCatalogConfig, error)

	// AvailableStrategies returns every registered strategy variant whose
	// Dim matches the namespace's embedding_dim. Empty slice when no
	// matching variant is registered.
	AvailableStrategies(namespaceEmbeddingDim int) []CatalogStrategyDescriptor
}

// catalogBacklogReader reports the operational catalog counts for a
// namespace by querying both Postgres (catalog_items state buckets) and
// Redis (XLEN of catalog:embed:{ns}). Wired by an adapter in cmd/admin.
type catalogBacklogReader interface {
	Read(ctx context.Context, namespace string) (CatalogBacklog, error)
}

// eventRateReader exposes the admin-plane per-namespace ingest rate, fed by
// the events-tail bridge. Wired by cmd/admin; *eventsrate.Tracker satisfies
// it. When nil, ingest rates surface as zero.
type eventRateReader interface {
	RatePerSec(namespace string, window time.Duration) float64
	RatesPerSec(window time.Duration) map[string]float64
}

type batchRunner interface {
	// StartNamespaceRun inserts the running row and executes the run in a
	// goroutine detached from ctx, returning the run id. Returns
	// batchrun.ErrRunInProgress when the namespace's cross-process compute
	// lock is held.
	StartNamespaceRun(ctx context.Context, namespace string, triggerSource batchrun.TriggerSource, timeout time.Duration) (int64, error)
	// LockNamespace blocks until the namespace's compute lock is free —
	// taken by DeleteNamespace so a wipe never races a run in flight.
	LockNamespace(ctx context.Context, namespace string) (release func(), err error)
}

// streamPublisher abstracts the Redis Streams write used by the catalog
// re-embed and redrive paths. The concrete *goredis.Client implements this
// interface; tests inject a fake. Defined here (not in catalog_ops_service.go)
// so the Service struct can declare it as a field.
type streamPublisher interface {
	XAdd(ctx context.Context, args *goredis.XAddArgs) *goredis.StringCmd
}

// catalogStrategyPicker exposes the active (strategy_id, strategy_version) for
// a namespace's catalog config. The cmd/admin wiring layer satisfies this with
// an adapter over nsconfig.Service so we avoid a forbidden cross-domain import.
type catalogStrategyPicker interface {
	GetCatalogStrategy(ctx context.Context, namespace string) (strategyID, strategyVersion string, enabled bool, err error)
}

// qdrantPointDeleter abstracts Qdrant point deletion so the delete catalog
// item path is testable. The concrete *qdrant.Client implements this via a
// thin adapter (see qdrantClientPointDeleter); tests inject a fake.
type qdrantPointDeleter interface {
	DeletePoint(ctx context.Context, collection string, numericID uint64) error
}

// Service implements admin business logic.
type Service struct {
	repo            adminRepo
	apiURL          string
	apiKey          string
	redisClient     *goredis.Client
	qdrantClient    *qdrant.Client
	httpClient      *http.Client
	job             batchRunner
	nsConfigSvc     nsConfigUpserter
	catalogConfig   nsCatalogConfigurator
	catalogBacklog  catalogBacklogReader
	catalogPicker   catalogStrategyPicker
	streamPublisher streamPublisher
	qdrantDeleter   qdrantPointDeleter
	eventRate       eventRateReader
	nowFn           func() time.Time
	runningReembed  sync.Map // keyed by namespace name; serializes re-embed triggers
}

// NewService creates a new Service.
func NewService(repo adminRepo, apiURL, apiKey string, redisClient *goredis.Client, qdrantClient *qdrant.Client, job batchRunner, nsConfigSvc nsConfigUpserter) *Service {
	s := &Service{
		repo:         repo,
		apiURL:       apiURL,
		apiKey:       apiKey,
		redisClient:  redisClient,
		qdrantClient: qdrantClient,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		job:          job,
		nsConfigSvc:  nsConfigSvc,
		nowFn:        time.Now,
	}
	if redisClient != nil {
		s.streamPublisher = redisClient
	}
	if qdrantClient != nil {
		s.qdrantDeleter = newQdrantClientPointDeleter(qdrantClient)
	}
	return s
}

// SetCatalogConfigurator wires the catalog config adapter. Optional —
// when nil, the catalog admin endpoints return 503 Service Unavailable
// to signal that the catalog feature is not enabled in this deployment.
// (Plain method rather than constructor parameter so existing callers
// of NewService stay source-compatible.)
func (s *Service) SetCatalogConfigurator(c nsCatalogConfigurator) {
	s.catalogConfig = c
}

// SetCatalogBacklogReader wires the optional Postgres+Redis backlog reader
// for the catalog admin endpoint. When nil, GetCatalog returns zero
// backlog counts (admin UI degrades gracefully).
func (s *Service) SetCatalogBacklogReader(r catalogBacklogReader) {
	s.catalogBacklog = r
}

// SetCatalogStrategyPicker wires the lookup of the active strategy id+version
// for a namespace. Required by the re-embed orchestration path; when nil the
// re-embed handler returns 503 Service Unavailable.
func (s *Service) SetCatalogStrategyPicker(p catalogStrategyPicker) {
	s.catalogPicker = p
}

// SetEventRateTracker wires the per-namespace ingest-rate tracker. Optional —
// when nil, ingest rates (fleet events/min, /metrics/summary) surface as zero.
func (s *Service) SetEventRateTracker(t eventRateReader) {
	s.eventRate = t
}

// SetStreamPublisher overrides the default Redis-backed XAdd publisher.
// Used by tests to inject a fake; production wiring leaves this as the
// default redisClient.
func (s *Service) SetStreamPublisher(p streamPublisher) {
	s.streamPublisher = p
}

// SetQdrantPointDeleter overrides the default Qdrant client adapter used by
// the DeleteCatalogItem path. Tests inject a fake.
func (s *Service) SetQdrantPointDeleter(d qdrantPointDeleter) {
	s.qdrantDeleter = d
}

// SetNowFn overrides the wall clock used to stamp catalog re-embed batch
// rows. Tests use this for deterministic timestamps.
func (s *Service) SetNowFn(fn func() time.Time) {
	if fn == nil {
		s.nowFn = time.Now
		return
	}
	s.nowFn = fn
}

// GetHealth proxies GET <apiURL>/healthz and returns the parsed response.
func (s *Service) GetHealth(ctx context.Context) (*HealthResponse, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.apiURL+"/healthz", http.NoBody)
	if err != nil {
		return nil, 0, fmt.Errorf("build health request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("health proxy: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing useful to do if close fails on a read-only response body

	var health HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode health response: %w", err)
	}
	return &health, resp.StatusCode, nil
}

// ListNamespaces returns all namespace configs from the database.
func (s *Service) ListNamespaces(ctx context.Context) ([]NamespaceConfig, error) {
	return s.repo.ListNamespaces(ctx)
}

// GetNamespace returns a single namespace config, or nil if not found.
func (s *Service) GetNamespace(ctx context.Context, namespace string) (*NamespaceConfig, error) {
	return s.repo.GetNamespace(ctx, namespace)
}

// UpsertNamespace creates or updates the namespace config by calling the injected
// nsConfigUpserter directly. Returns 201 on first-time create (when a plaintext API
// key is generated) and 200 on subsequent updates.
func (s *Service) UpsertNamespace(ctx context.Context, namespace string, req *NamespaceUpsertRequest) (*NamespaceUpsertResponse, int, error) {
	resp, err := s.nsConfigSvc.Upsert(ctx, namespace, req)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("upsert namespace: %w", err)
	}
	status := http.StatusOK
	if resp != nil && resp.APIKey != nil {
		status = http.StatusCreated
	}
	return resp, status, nil
}

// RotateNamespaceAPIKey mints a replacement API key for the namespace. The
// old key stops working immediately; the new plaintext is returned once.
// Returns (nil, nil) for unknown namespaces — handler maps to 404.
func (s *Service) RotateNamespaceAPIKey(ctx context.Context, namespace string) (*NamespaceKeyRotateResponse, error) {
	if s.nsConfigSvc == nil {
		return nil, errors.New("namespace config service is not wired")
	}
	resp, err := s.nsConfigSvc.RotateAPIKey(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("rotate namespace api key: %w", err)
	}
	return resp, nil
}

// ErrCatalogConfiguratorUnavailable signals that the catalog config adapter
// has not been wired (catalog feature disabled at this deployment). The
// handler maps this to 503 Service Unavailable so the admin UI can hide
// catalog controls gracefully rather than crash.
var ErrCatalogConfiguratorUnavailable = errors.New("admin: catalog configurator not wired")

// GetCatalogConfig returns the catalog config + available strategies +
// backlog snapshot for a namespace. Returns nil when the namespace does
// not exist; ErrCatalogConfiguratorUnavailable when the adapter is missing.
func (s *Service) GetCatalogConfig(ctx context.Context, namespace string) (*NamespaceCatalogResponse, error) {
	if s.catalogConfig == nil {
		return nil, ErrCatalogConfiguratorUnavailable
	}
	cfg, err := s.catalogConfig.GetCatalog(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get catalog config: %w", err)
	}
	if cfg == nil {
		return nil, nil
	}

	resp := &NamespaceCatalogResponse{
		Catalog:             *cfg,
		AvailableStrategies: s.catalogConfig.AvailableStrategies(cfg.EmbeddingDim),
	}
	if s.catalogBacklog != nil {
		// Backlog read failures are non-fatal; surface zero counts so the admin
		// UI still renders, but log so a persistent failure stays visible.
		if backlog, err := s.catalogBacklog.Read(ctx, namespace); err == nil {
			resp.Backlog = backlog
		} else {
			slog.WarnContext(ctx, "catalog backlog read failed",
				slog.String("namespace", namespace), slog.String("error", err.Error()))
		}
	}
	// Liveness signals — best-effort: each row surfaces as nil when its read
	// fails, but log the error so a real regression (e.g. a query/scan mismatch)
	// is not silently swallowed.
	if t, err := s.repo.GetLastCatalogEmbeddedAt(ctx, namespace); err == nil {
		resp.LastEmbeddedAt = t
	} else {
		slog.WarnContext(ctx, "catalog last-embedded-at read failed",
			slog.String("namespace", namespace), slog.String("error", err.Error()))
	}
	if run, err := s.repo.FindLatestReembedRun(ctx, namespace); err != nil {
		slog.WarnContext(ctx, "latest reembed run read failed",
			slog.String("namespace", namespace), slog.String("error", err.Error()))
	} else if run != nil {
		resp.LastReEmbed = summarizeReembedRun(run)
	}
	return resp, nil
}

// summarizeReembedRun derives the CatalogReEmbedSummary projection from a
// batch_run_logs row that was created with trigger_source='admin_reembed'.
// Reads target strategy from the dedicated columns added by migration 012;
// status comes from completed_at + success.
func summarizeReembedRun(row *BatchRunLog) *CatalogReEmbedSummary {
	if row == nil {
		return nil
	}
	strategyID, strategyVersion := ReembedTargetFromBatchRow(row)

	out := &CatalogReEmbedSummary{
		BatchRunID:      row.ID,
		StartedAt:       row.StartedAt,
		CompletedAt:     row.CompletedAt,
		ProcessedItems:  row.EntitiesProcessed,
		StrategyID:      strategyID,
		StrategyVersion: strategyVersion,
	}
	if row.DurationMs != nil {
		out.DurationMs = *row.DurationMs
	}
	switch {
	case row.CompletedAt == nil:
		out.Status = "running"
	case row.Success:
		out.Status = "success"
	default:
		out.Status = "failed"
		if row.ErrorMessage != nil {
			out.ErrorMessage = *row.ErrorMessage
		}
	}
	return out
}

// UpdateCatalogConfig applies the catalog auto-embedding config change
// for a namespace. Returns:
//   - the updated config on success
//   - nil result + nil error when the namespace does not exist
//   - *CatalogDimensionMismatch when the chosen strategy's dim != embedding_dim
//   - ErrCatalogConfiguratorUnavailable when the adapter is missing
//   - any other error (DB, registry) wrapped
func (s *Service) UpdateCatalogConfig(ctx context.Context, namespace string, req *NamespaceCatalogUpdateRequest) (*NamespaceCatalogConfig, error) {
	if s.catalogConfig == nil {
		return nil, ErrCatalogConfiguratorUnavailable
	}
	cfg, err := s.catalogConfig.UpdateCatalog(ctx, namespace, req)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// GetBatchRuns returns paginated batch run logs, filtered total, and aggregate
// stats. kind selects between CF runs (cron + admin "Run batch now") and
// catalog re-embed orchestration runs; an empty kind returns all kinds.
func (s *Service) GetBatchRuns(ctx context.Context, namespace, status, kind string, limit, offset int) ([]BatchRunLog, int, BatchRunStats, error) {
	return s.repo.GetBatchRunLogs(ctx, namespace, status, kind, limit, offset)
}

// GetSubjectRecommendations proxies a sub-resource recommendation request to
// cmd/api and returns the typed admin recommendation response.
func (s *Service) GetSubjectRecommendations(ctx context.Context, namespace, subjectID string, limit, offset int, debug bool) (*RecommendResponse, int, error) {
	if limit <= 0 {
		limit = 10
	}

	// Path-escape the caller-supplied segments: a subject id containing "/"
	// or "?" would otherwise steer this admin-authenticated proxy request at
	// an arbitrary path on the API host.
	url := fmt.Sprintf("%s/v1/namespaces/%s/subjects/%s/recommendations?limit=%d&offset=%d",
		s.apiURL, neturl.PathEscape(namespace), neturl.PathEscape(subjectID), limit, offset)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, 0, fmt.Errorf("build recommend request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("recommend proxy: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing useful to do if close fails on a read-only response body

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error context; primary error is the non-2xx status
		return nil, resp.StatusCode, fmt.Errorf("recommend proxy returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		SubjectID string `json:"subject_id"`
		Namespace string `json:"namespace"`
		Items     []struct {
			ObjectID string  `json:"object_id"`
			Score    float64 `json:"score"`
			Rank     int     `json:"rank"`
		} `json:"items"`
		Source      string    `json:"source"`
		Limit       int       `json:"limit"`
		Offset      int       `json:"offset"`
		Total       int       `json:"total"`
		GeneratedAt time.Time `json:"generated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode recommend response: %w", err)
	}

	items := make([]RecommendDebugItem, len(raw.Items))
	for i, it := range raw.Items {
		items[i] = RecommendDebugItem{ObjectID: it.ObjectID, Score: it.Score, Rank: it.Rank}
	}

	result := &RecommendResponse{
		SubjectID:   raw.SubjectID,
		Namespace:   raw.Namespace,
		Items:       items,
		Source:      raw.Source,
		Limit:       raw.Limit,
		Offset:      raw.Offset,
		Total:       raw.Total,
		GeneratedAt: raw.GeneratedAt,
	}
	if debug {
		result.Debug = s.recommendDebug(ctx, namespace, subjectID)
	}

	return result, http.StatusOK, nil
}

// GetQdrant returns point counts for the four Qdrant collections of a namespace.
func (s *Service) GetQdrant(ctx context.Context, namespace string) (*QdrantInspectResponse, error) {
	return &QdrantInspectResponse{
		Subjects:      s.qdrantCollection(ctx, namespace+"_subjects"),
		Objects:       s.qdrantCollection(ctx, namespace+"_objects"),
		SubjectsDense: s.qdrantCollection(ctx, namespace+"_subjects_dense"),
		ObjectsDense:  s.qdrantCollection(ctx, namespace+"_objects_dense"),
	}, nil
}

func (s *Service) qdrantCollection(ctx context.Context, name string) QdrantCollection {
	stat := QdrantCollection{}
	if s.qdrantClient == nil {
		return stat
	}
	exists, err := s.qdrantClient.CollectionExists(ctx, name)
	if err != nil || !exists {
		return stat
	}
	stat.Exists = true
	if info, err := s.qdrantClient.GetCollectionInfo(ctx, name); err == nil {
		stat.PointsCount = info.GetPointsCount()
	}
	return stat
}

func (s *Service) recommendDebug(ctx context.Context, namespace, subjectID string) *RecommendDebug {
	debug := &RecommendDebug{SparseNNZ: -1}

	ns, err := s.repo.GetNamespace(ctx, namespace)
	if err != nil || ns == nil {
		return debug
	}
	debug.Alpha = ns.Alpha

	stats, err := s.repo.GetSubjectStats(ctx, namespace, subjectID, ns.SeenItemsDays)
	if err != nil || stats == nil {
		return debug
	}
	debug.InteractionCount = stats.InteractionCount
	debug.SeenItemsCount = len(stats.SeenItems)
	if stats.NumericID != nil {
		debug.SparseNNZ = s.sparseNNZ(ctx, namespace, *stats.NumericID)
	}
	return debug
}

// ListSubjects returns a page of the subjects that have events in namespace,
// derived from the events table. prefix narrows to subject ids starting with
// it; sort falls back to last_seen when empty or unrecognised.
func (s *Service) ListSubjects(ctx context.Context, namespace, prefix, sort string, limit, offset int) (*SubjectsListResponse, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	if _, ok := subjectOrderBy[sort]; !ok {
		sort = SubjectSortLastSeen
	}

	items, total, err := s.repo.ListSubjects(ctx, namespace, prefix, sort, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list subjects: %w", err)
	}
	return &SubjectsListResponse{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
		Sort:   sort,
	}, nil
}

// GetSubjectProfile returns interaction count, seen items, and sparse vector NNZ for a subject.
func (s *Service) GetSubjectProfile(ctx context.Context, namespace, subjectID string) (*SubjectProfileResponse, error) {
	ns, err := s.repo.GetNamespace(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get namespace: %w", err)
	}
	seenItemsDays := 30
	if ns != nil {
		seenItemsDays = ns.SeenItemsDays
	}

	stats, err := s.repo.GetSubjectStats(ctx, namespace, subjectID, seenItemsDays)
	if err != nil {
		return nil, fmt.Errorf("get subject stats: %w", err)
	}

	nnz := -1
	if stats.NumericID != nil {
		nnz = s.sparseNNZ(ctx, namespace, *stats.NumericID)
	}

	seenItems := stats.SeenItems
	if seenItems == nil {
		seenItems = []string{}
	}

	return &SubjectProfileResponse{
		SubjectID:        subjectID,
		Namespace:        namespace,
		InteractionCount: stats.InteractionCount,
		SeenItems:        seenItems,
		SeenItemsDays:    seenItemsDays,
		SparseVectorNNZ:  nnz,
	}, nil
}

func (s *Service) sparseNNZ(ctx context.Context, namespace string, numericID uint64) int {
	if s.qdrantClient == nil {
		return -1
	}
	results, err := s.qdrantClient.Get(ctx, &qdrant.GetPoints{
		CollectionName: namespace + "_subjects",
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(numericID)},
		WithVectors:    qdrant.NewWithVectorsInclude("sparse_interactions"),
	})
	if err != nil || len(results) == 0 {
		return -1
	}
	sv := results[0].GetVectors().GetVectors().GetVectors()["sparse_interactions"].GetSparse()
	if sv == nil {
		return -1
	}
	return len(sv.GetIndices())
}

// GetTrending proxies the trending request to cmd/api, then fetches the Redis TTL.
func (s *Service) GetTrending(ctx context.Context, namespace string, limit, offset, windowHours int) (*TrendingAdminResponse, error) {
	params := fmt.Sprintf("?limit=%d&offset=%d", limit, offset)
	if windowHours > 0 {
		params += "&window_hours=" + strconv.Itoa(windowHours)
	}

	url := s.apiURL + "/v1/namespaces/" + neturl.PathEscape(namespace) + "/trending" + params
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build trending request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trending proxy: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing useful to do if close fails on a read-only response body

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error context; primary error is the non-2xx status
		return nil, fmt.Errorf("trending proxy returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		Namespace string `json:"namespace"`
		Items     []struct {
			ObjectID string  `json:"object_id"`
			Score    float64 `json:"score"`
		} `json:"items"`
		WindowHours int       `json:"window_hours"`
		Limit       int       `json:"limit"`
		Offset      int       `json:"offset"`
		Total       int       `json:"total"`
		GeneratedAt time.Time `json:"generated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode trending response: %w", err)
	}

	// Get Redis TTL for the trending key.
	// d.Seconds() must be used for both positive and negative durations:
	// time.Duration(-2)*time.Second gives int(d)==-2000000000 (nanoseconds), not -2.
	ttl := -2
	if s.redisClient != nil {
		d, err := s.redisClient.TTL(ctx, "trending:"+namespace).Result()
		if err == nil {
			ttl = int(d.Seconds())
		}
	}

	items := make([]TrendingAdminEntry, len(raw.Items))
	for i, it := range raw.Items {
		items[i] = TrendingAdminEntry{ObjectID: it.ObjectID, Score: it.Score, CacheTTLSec: ttl}
	}

	return &TrendingAdminResponse{
		Namespace:   raw.Namespace,
		Items:       items,
		WindowHours: raw.WindowHours,
		Limit:       raw.Limit,
		Offset:      raw.Offset,
		Total:       raw.Total,
		CacheTTLSec: ttl,
		GeneratedAt: raw.GeneratedAt,
	}, nil
}

// errBatchRunning is returned when a concurrent batch trigger is attempted for the same namespace.
var errBatchRunning = errors.New("batch already in progress")

// manualRunTimeout bounds an admin-triggered batch run. Generous on purpose:
// a large item2vec retrain can take a long time, and the run no longer holds
// an HTTP request open while it works.
const manualRunTimeout = time.Hour

// CreateBatchRun starts all batch phases for a namespace in the background
// and returns the created run immediately — true 202 Accepted semantics; the
// client's disconnect can no longer abort the run mid-phase.
//
// Returns 409-style errBatchRunning if a batch is already in progress for
// that namespace (cross-process, via the compute advisory lock).
// Returns 404-style nil,nil from GetNamespace when namespace does not exist.
func (s *Service) CreateBatchRun(ctx context.Context, ns string) (*BatchRunCreateResponse, error) {
	nsConfig, err := s.repo.GetNamespace(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("get namespace: %w", err)
	}
	if nsConfig == nil {
		return nil, nil // caller maps nil,nil → 404
	}
	if s.job == nil {
		return nil, errors.New("batch runner is not wired")
	}

	start := time.Now()
	runID, err := s.job.StartNamespaceRun(ctx, ns, batchrun.TriggerManual, manualRunTimeout)
	if err != nil {
		if errors.Is(err, batchrun.ErrRunInProgress) {
			return nil, fmt.Errorf("%w for namespace %s", errBatchRunning, ns)
		}
		return nil, fmt.Errorf("start batch run: %w", err)
	}

	return &BatchRunCreateResponse{
		ID:        runID,
		Namespace: ns,
		Status:    "running",
		StartedAt: start.UTC(),
	}, nil
}

// GetRecentEvents returns a paginated list of events for a namespace, newest first.
func (s *Service) GetRecentEvents(ctx context.Context, ns string, limit, offset int, subjectID string) (*EventsListResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	events, total, err := s.repo.GetRecentEvents(ctx, ns, limit, offset, subjectID)
	if err != nil {
		return nil, fmt.Errorf("get recent events: %w", err)
	}
	return &EventsListResponse{
		Items:  events,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

// InjectEvent proxies a test event injection to cmd/api and returns the id of
// the persisted event so the SPA can highlight it in the live tail.
func (s *Service) InjectEvent(ctx context.Context, ns string, req InjectEventRequest) (int64, error) {
	if req.OccurredAt == nil {
		now := time.Now().UTC().Format(time.RFC3339)
		req.OccurredAt = &now
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return 0, fmt.Errorf("marshal inject event: %w", err)
	}

	url := s.apiURL + "/v1/namespaces/" + neturl.PathEscape(ns) + "/events"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("build inject event request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("inject event proxy: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing useful to do if close fails on a read-only response body

	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body) //nolint:errcheck // best-effort read for error context
		return 0, fmt.Errorf("inject event upstream returned %d: %s", resp.StatusCode, string(body))
	}

	// cmd/api returns {"event_id": N}. Best-effort decode — a missing/garbled
	// body still means the event landed (202), so we return id=0, not an error.
	var upstream struct {
		EventID int64 `json:"event_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&upstream) //nolint:errcheck // id is a nice-to-have; absence is non-fatal
	return upstream.EventID, nil
}

// CreateDemoData creates the bundled demo namespace and loads deterministic sample events.
func (s *Service) CreateDemoData(ctx context.Context) (*DemoDatasetResponse, error) {
	demoReq := demoNamespaceConfig
	upsert, _, err := s.UpsertNamespace(ctx, demoNamespace, &demoReq)
	if err != nil {
		return nil, fmt.Errorf("create demo namespace: %w", err)
	}

	now := time.Now().UTC()
	created, err := s.repo.SeedDemoEvents(ctx, demoNamespace, demoDataset, now)
	if err != nil {
		return nil, fmt.Errorf("seed demo events: %w", err)
	}

	resp := &DemoDatasetResponse{
		Namespace:     demoNamespace,
		EventsCreated: created,
	}
	if upsert != nil && upsert.APIKey != nil {
		resp.APIKey = *upsert.APIKey
	}

	// Enable catalog auto-embedding and seed the bundled catalog content so
	// the admin catalog browse page has data to render. The configurator may
	// be unwired in trimmed deployments (e.g. some tests) — in that case the
	// catalog seeding is skipped silently rather than failing demo setup.
	if s.catalogConfig != nil {
		enable := true
		strategyID := demoCatalogStrategyID
		strategyVersion := demoCatalogStrategyVersion
		if _, err := s.catalogConfig.UpdateCatalog(ctx, demoNamespace, &NamespaceCatalogUpdateRequest{
			Enabled:         enable,
			StrategyID:      &strategyID,
			StrategyVersion: &strategyVersion,
			Params:          map[string]any{"dim": demoCatalogStrategyDim},
		}); err != nil {
			return nil, fmt.Errorf("enable demo catalog: %w", err)
		}

		catalogCreated, err := s.repo.SeedDemoCatalogItems(ctx, demoNamespace, demoCatalogDataset, now)
		if err != nil {
			return nil, fmt.Errorf("seed demo catalog items: %w", err)
		}
		resp.CatalogItemsCreated = catalogCreated

		// Best-effort: publish each seeded row to the embed stream so a
		// running cmd/embedder picks them up the same way a data-plane
		// ingest would. Publish failures are non-fatal — rows remain in
		// state='pending' and can be picked up later (e.g. via re-embed).
		if s.streamPublisher != nil {
			items, _, err := s.repo.ListCatalogItems(ctx, demoNamespace, "pending", len(demoCatalogDataset), 0, "", "")
			if err != nil {
				return nil, fmt.Errorf("list seeded demo catalog items: %w", err)
			}
			for _, it := range items {
				if err := s.publishCatalogEnqueue(ctx, demoNamespace, CatalogReembedTarget{
					ID:       it.ID,
					ObjectID: it.ObjectID,
				}, strategyID, strategyVersion); err != nil {
					slog.Warn("demo catalog xadd failed",
						"object_id", it.ObjectID, "error", err)
				}
			}
		}
	}

	return resp, nil
}

// DeleteDemoData removes the bundled demo namespace and its generated data.
func (s *Service) DeleteDemoData(ctx context.Context) (*DemoDatasetResponse, error) {
	deleted, err := s.clearNamespaceEverywhere(ctx, demoNamespace)
	if err != nil {
		return nil, err
	}
	return &DemoDatasetResponse{Namespace: demoNamespace, EventsDeleted: deleted}, nil
}

// DeleteNamespace removes a namespace and every trace of its data from
// PostgreSQL, Redis, and Qdrant. Returns (nil, nil) when the namespace does
// not exist so the handler can map that to 404.
func (s *Service) DeleteNamespace(ctx context.Context, namespace string) (*NamespaceDeleteResponse, error) {
	cfg, err := s.repo.GetNamespace(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get namespace: %w", err)
	}
	if cfg == nil {
		return nil, nil
	}

	// Hold the namespace's compute lock for the whole wipe: a cron tick or
	// manual run in flight would otherwise re-upsert Qdrant collections
	// after the delete finishes, leaving orphans nothing ever cleans.
	if s.job != nil {
		release, lockErr := s.job.LockNamespace(ctx, namespace)
		if lockErr != nil {
			return nil, fmt.Errorf("acquire namespace lock: %w", lockErr)
		}
		defer release()
	}

	deleted, err := s.clearNamespaceEverywhere(ctx, namespace)
	if err != nil {
		return nil, err
	}
	return &NamespaceDeleteResponse{Namespace: namespace, EventsDeleted: deleted}, nil
}

// ResetApp wipes every namespace and its associated data across PostgreSQL,
// Redis, and Qdrant. The caller is expected to gate this on a typed
// confirmation string at the HTTP layer.
//
// Unlike DeleteNamespace's per-namespace loop, this path TRUNCATEs all
// data tables and SCANs every namespace-prefixed key / Qdrant collection.
// That way orphan rows whose namespace_configs row was already gone (e.g.
// from a previous partial delete) also get cleaned up.
func (s *Service) ResetApp(ctx context.Context) (*ResetAppResponse, error) {
	// Snapshot the namespace list BEFORE truncating so the response body
	// can echo which namespaces were wiped.
	namespaces, err := s.repo.ListNamespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	names := make([]string, 0, len(namespaces))
	for _, ns := range namespaces {
		names = append(names, ns.Namespace)
	}

	eventsDeleted, nsDeleted, err := s.repo.TruncateAllNamespaceData(ctx)
	if err != nil {
		return nil, fmt.Errorf("truncate postgres data: %w", err)
	}

	if s.redisClient != nil {
		if err := s.flushAllNamespaceRedis(ctx); err != nil {
			return nil, fmt.Errorf("flush redis: %w", err)
		}
	}

	if s.qdrantClient != nil {
		if err := s.flushAllNamespaceQdrant(ctx); err != nil {
			return nil, fmt.Errorf("flush qdrant: %w", err)
		}
	}

	return &ResetAppResponse{
		NamespacesDeleted: nsDeleted,
		EventsDeleted:     eventsDeleted,
		Namespaces:        names,
	}, nil
}

// flushAllNamespaceRedis scans every namespace-prefixed key across the three
// known patterns and deletes them. Catches orphan keys whose namespace no
// longer has a config row.
func (s *Service) flushAllNamespaceRedis(ctx context.Context) error {
	for _, pattern := range []string{"trending:*", "catalog:embed:*", "rec:*"} {
		iter := s.redisClient.Scan(ctx, 0, pattern, 500).Iterator()
		batch := make([]string, 0, 500)
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			if err := s.redisClient.Del(ctx, batch...).Err(); err != nil {
				return fmt.Errorf("delete redis keys for %q: %w", pattern, err)
			}
			batch = batch[:0]
			return nil
		}
		for iter.Next(ctx) {
			batch = append(batch, iter.Val())
			if len(batch) >= 500 {
				if err := flush(); err != nil {
					return err
				}
			}
		}
		if err := iter.Err(); err != nil {
			return fmt.Errorf("scan redis keys for %q: %w", pattern, err)
		}
		if err := flush(); err != nil {
			return err
		}
	}
	return nil
}

// qdrantCollectionSuffixes enumerates the four per-namespace collection
// names Codohue creates. flushAllNamespaceQdrant only drops collections
// matching one of these suffixes so a Qdrant instance shared with other
// applications is left untouched.
var qdrantCollectionSuffixes = []string{
	"_subjects_dense",
	"_objects_dense",
	"_subjects",
	"_objects",
}

func (s *Service) flushAllNamespaceQdrant(ctx context.Context) error {
	collections, err := s.qdrantClient.ListCollections(ctx)
	if err != nil {
		return fmt.Errorf("list collections: %w", err)
	}
	for _, name := range collections {
		for _, suffix := range qdrantCollectionSuffixes {
			if strings.HasSuffix(name, suffix) {
				if err := s.qdrantClient.DeleteCollection(ctx, name); err != nil {
					return fmt.Errorf("delete collection %q: %w", name, err)
				}
				break
			}
		}
	}
	return nil
}

// clearNamespaceEverywhere removes a single namespace from PostgreSQL, Redis,
// and Qdrant. Shared by demo cleanup, single-namespace deletion, and the
// app-wide reset. Returns the number of events removed from Postgres.
func (s *Service) clearNamespaceEverywhere(ctx context.Context, namespace string) (int, error) {
	deleted, err := s.repo.ClearNamespaceData(ctx, namespace)
	if err != nil {
		return 0, fmt.Errorf("clear postgres data for %q: %w", namespace, err)
	}

	if s.redisClient != nil {
		if err := s.clearNamespaceRedis(ctx, namespace); err != nil {
			return 0, fmt.Errorf("clear redis data for %q: %w", namespace, err)
		}
	}

	if s.qdrantClient != nil {
		if err := s.clearNamespaceQdrant(ctx, namespace); err != nil {
			return 0, fmt.Errorf("clear qdrant data for %q: %w", namespace, err)
		}
	}
	return deleted, nil
}

func (s *Service) clearNamespaceRedis(ctx context.Context, namespace string) error {
	keys := []string{
		"trending:" + namespace,
		"catalog:embed:" + namespace,
	}
	iter := s.redisClient.Scan(ctx, 0, "rec:"+namespace+":*", 100).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("scan recommendation cache: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	if err := s.redisClient.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("delete redis keys: %w", err)
	}
	return nil
}

func (s *Service) clearNamespaceQdrant(ctx context.Context, namespace string) error {
	for _, collection := range []string{
		namespace + "_subjects",
		namespace + "_objects",
		namespace + "_subjects_dense",
		namespace + "_objects_dense",
	} {
		exists, err := s.qdrantClient.CollectionExists(ctx, collection)
		if err != nil {
			return fmt.Errorf("check collection %q: %w", collection, err)
		}
		if !exists {
			continue
		}
		if err := s.qdrantClient.DeleteCollection(ctx, collection); err != nil {
			return fmt.Errorf("delete collection %q: %w", collection, err)
		}
	}
	return nil
}
