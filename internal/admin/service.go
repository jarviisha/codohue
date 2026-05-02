package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"
)

// adminRepo is the repository interface used by Service.
type adminRepo interface {
	ListNamespaces(ctx context.Context) ([]NamespaceConfig, error)
	GetNamespace(ctx context.Context, namespace string) (*NamespaceConfig, error)
	GetBatchRunLogs(ctx context.Context, namespace string, limit int) ([]BatchRunLog, error)
	GetLastBatchRunPerNamespace(ctx context.Context) (map[string]BatchRunLog, error)
	GetRecentEventCounts(ctx context.Context, windowHours int) (map[string]int, error)
	GetSubjectStats(ctx context.Context, namespace, subjectID string, seenItemsDays int) (*SubjectStats, error)
}

// Service implements admin business logic.
type Service struct {
	repo         adminRepo
	apiURL       string
	apiKey       string
	redisClient  *goredis.Client
	qdrantClient *qdrant.Client
	httpClient   *http.Client
}

// NewService creates a new Service.
func NewService(repo adminRepo, apiURL, apiKey string, redisClient *goredis.Client, qdrantClient *qdrant.Client) *Service {
	return &Service{
		repo:         repo,
		apiURL:       apiURL,
		apiKey:       apiKey,
		redisClient:  redisClient,
		qdrantClient: qdrantClient,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
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

// UpsertNamespace proxies a create/update request to cmd/api.
func (s *Service) UpsertNamespace(ctx context.Context, namespace string, body io.Reader) (*NamespaceUpsertResponse, int, error) {
	url := s.apiURL + "/v1/config/namespaces/" + namespace
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return nil, 0, fmt.Errorf("build upsert request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("upsert proxy: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // nothing useful to do if close fails on a read-only response body

	var result NamespaceUpsertResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode upsert response: %w", err)
	}
	return &result, resp.StatusCode, nil
}

// GetBatchRuns returns recent batch run logs.
func (s *Service) GetBatchRuns(ctx context.Context, namespace string, limit int) ([]BatchRunLog, error) {
	return s.repo.GetBatchRunLogs(ctx, namespace, limit)
}

// GetNamespacesOverview returns all namespaces with computed health status.
func (s *Service) GetNamespacesOverview(ctx context.Context) (*NamespacesOverviewResponse, error) {
	namespaces, err := s.repo.ListNamespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}

	lastRuns, err := s.repo.GetLastBatchRunPerNamespace(ctx)
	if err != nil {
		return nil, fmt.Errorf("get last batch runs: %w", err)
	}

	eventCounts, err := s.repo.GetRecentEventCounts(ctx, 24)
	if err != nil {
		return nil, fmt.Errorf("get recent event counts: %w", err)
	}

	out := make([]NamespaceHealth, 0, len(namespaces))
	for _, ns := range namespaces {
		h := NamespaceHealth{
			Config:          ns,
			ActiveEvents24h: eventCounts[ns.Namespace],
		}

		if run, ok := lastRuns[ns.Namespace]; ok {
			r := run
			h.LastRun = &r
			switch {
			case !run.Success:
				h.Status = NSStatusDegraded
			case h.ActiveEvents24h > 0:
				h.Status = NSStatusActive
			default:
				h.Status = NSStatusIdle
			}
		} else {
			h.Status = NSStatusCold
		}

		out = append(out, h)
	}

	return &NamespacesOverviewResponse{Namespaces: out}, nil
}

// DebugRecommend proxies a recommendation request to cmd/api and returns debug info.
func (s *Service) DebugRecommend(ctx context.Context, req *RecommendDebugRequest) (*RecommendDebugResponse, int, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	url := fmt.Sprintf("%s/v1/recommendations?namespace=%s&subject_id=%s&limit=%d&offset=%d",
		s.apiURL, req.Namespace, req.SubjectID, limit, req.Offset)

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

	return &RecommendDebugResponse{
		SubjectID:   raw.SubjectID,
		Namespace:   raw.Namespace,
		Items:       items,
		Source:      raw.Source,
		Limit:       raw.Limit,
		Offset:      raw.Offset,
		Total:       raw.Total,
		GeneratedAt: raw.GeneratedAt,
	}, http.StatusOK, nil
}

// GetQdrantStats returns point counts for the four Qdrant collections of a namespace.
func (s *Service) GetQdrantStats(ctx context.Context, namespace string) (*QdrantStatsResponse, error) {
	names := []string{
		namespace + "_subjects",
		namespace + "_objects",
		namespace + "_subjects_dense",
		namespace + "_objects_dense",
	}

	cols := make(map[string]QdrantCollectionStat, len(names))
	for _, col := range names {
		stat := QdrantCollectionStat{}
		if s.qdrantClient != nil {
			exists, err := s.qdrantClient.CollectionExists(ctx, col)
			if err == nil && exists {
				stat.Exists = true
				if info, err := s.qdrantClient.GetCollectionInfo(ctx, col); err == nil {
					stat.PointsCount = info.GetPointsCount()
					stat.IndexedVectorsCount = info.GetIndexedVectorsCount()
				}
			}
		}
		cols[col] = stat
	}

	return &QdrantStatsResponse{Namespace: namespace, Collections: cols}, nil
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
	if stats.NumericID != nil && s.qdrantClient != nil {
		results, err := s.qdrantClient.Get(ctx, &qdrant.GetPoints{
			CollectionName: namespace + "_subjects",
			Ids:            []*qdrant.PointId{qdrant.NewIDNum(*stats.NumericID)},
			WithVectors:    qdrant.NewWithVectorsInclude("sparse_interactions"),
		})
		if err == nil && len(results) > 0 {
			sv := results[0].GetVectors().GetVectors().GetVectors()["sparse_interactions"].GetSparse()
			if sv != nil {
				nnz = len(sv.GetIndices())
			}
		}
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

// GetTrending proxies the trending request to cmd/api, then fetches the Redis TTL.
func (s *Service) GetTrending(ctx context.Context, namespace string, limit, offset, windowHours int) (*TrendingAdminResponse, error) {
	params := fmt.Sprintf("?limit=%d&offset=%d", limit, offset)
	if windowHours > 0 {
		params += "&window_hours=" + strconv.Itoa(windowHours)
	}

	url := s.apiURL + "/v1/trending/" + namespace + params
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
