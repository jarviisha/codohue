package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// adminRepo is the repository interface used by Service.
type adminRepo interface {
	ListNamespaces(ctx context.Context) ([]NamespaceConfig, error)
	GetNamespace(ctx context.Context, namespace string) (*NamespaceConfig, error)
	GetBatchRunLogs(ctx context.Context, namespace string, limit int) ([]BatchRunLog, error)
}

// Service implements admin business logic.
type Service struct {
	repo        adminRepo
	apiURL      string
	apiKey      string
	redisClient *goredis.Client
	httpClient  *http.Client
}

// NewService creates a new Service.
func NewService(repo adminRepo, apiURL, apiKey string, redisClient *goredis.Client) *Service {
	return &Service{
		repo:        repo,
		apiURL:      apiURL,
		apiKey:      apiKey,
		redisClient: redisClient,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, resp.StatusCode, fmt.Errorf("recommend proxy returned %d: %s", resp.StatusCode, string(body))
	}

	var raw struct {
		SubjectID   string    `json:"subject_id"`
		Namespace   string    `json:"namespace"`
		Items       []struct {
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
	defer resp.Body.Close()

	var raw struct {
		Namespace   string    `json:"namespace"`
		Items       []struct {
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
	ttl := -2
	if s.redisClient != nil {
		d, err := s.redisClient.TTL(ctx, "trending:"+namespace).Result()
		if err == nil {
			if d < 0 {
				ttl = int(d) // -1 or -2 from Redis
			} else {
				ttl = int(d.Seconds())
			}
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
