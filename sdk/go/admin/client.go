package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the Codohue admin server with the global admin key as a
// bearer token. It never handles session cookies.
type Client struct {
	baseURL  string
	adminKey string
	httpc    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying *http.Client (timeouts, transport).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpc = h
		}
	}
}

// New creates a Client for the admin server at baseURL (e.g.
// "http://localhost:2002") authenticating with adminKey.
func New(baseURL, adminKey string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("codohue/admin: base URL is required")
	}
	if adminKey == "" {
		return nil, fmt.Errorf("codohue/admin: admin key is required")
	}
	c := &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		adminKey: adminKey,
		httpc:    &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// ProvisionCatalogRequest describes a namespace to provision in catalog mode.
// Zero-value strategy fields default to the built-in hashing+ngrams strategy
// with the params the server expects for the given dimension.
type ProvisionCatalogRequest struct {
	// EmbeddingDim must match a dimension the chosen strategy supports
	// (built-in strategy: 64, 128, 256 or 512).
	EmbeddingDim int

	// StrategyID / StrategyVersion select the embedding strategy. Defaults:
	// "internal-hashing-ngrams" @ "v1".
	StrategyID      string
	StrategyVersion string
	// StrategyParams defaults to {"dim": EmbeddingDim}.
	StrategyParams map[string]any

	// ActionWeights, Alpha and DenseDistance are optional namespace tuning
	// applied in the same request; nil/zero leaves server defaults (or the
	// current values, PATCH-style) untouched.
	ActionWeights map[string]float64
	Alpha         *float64
	DenseDistance string
}

// ProvisionResult reports the outcome of a provisioning call.
type ProvisionResult struct {
	Namespace string    `json:"namespace"`
	UpdatedAt time.Time `json:"updated_at"`
	// APIKey is the namespace's data-plane key, returned exactly once on
	// first creation; empty on updates.
	APIKey string `json:"api_key"`
}

// ProvisionCatalogNamespace creates or updates ns in catalog mode with one
// PUT: dense_source=catalog plus the strategy fields, validated server-side
// against embedding_dim exactly like the dedicated catalog endpoint.
func (c *Client) ProvisionCatalogNamespace(ctx context.Context, ns string, req ProvisionCatalogRequest) (*ProvisionResult, error) {
	if ns == "" {
		return nil, fmt.Errorf("codohue/admin: namespace is required")
	}
	if req.EmbeddingDim <= 0 {
		return nil, fmt.Errorf("codohue/admin: embedding dimension must be positive")
	}
	strategyID := req.StrategyID
	if strategyID == "" {
		strategyID = "internal-hashing-ngrams"
	}
	strategyVersion := req.StrategyVersion
	if strategyVersion == "" {
		strategyVersion = "v1"
	}
	params := req.StrategyParams
	if params == nil {
		params = map[string]any{"dim": req.EmbeddingDim}
	}

	body := map[string]any{
		"dense_source":             "catalog",
		"embedding_dim":            req.EmbeddingDim,
		"catalog_strategy_id":      strategyID,
		"catalog_strategy_version": strategyVersion,
		"catalog_strategy_params":  params,
	}
	if req.ActionWeights != nil {
		body["action_weights"] = req.ActionWeights
	}
	if req.Alpha != nil {
		body["alpha"] = *req.Alpha
	}
	if req.DenseDistance != "" {
		body["dense_distance"] = req.DenseDistance
	}

	var out struct {
		Namespace string    `json:"namespace"`
		UpdatedAt time.Time `json:"updated_at"`
		APIKey    *string   `json:"api_key"`
	}
	if err := c.do(ctx, http.MethodPut, "/api/admin/v1/namespaces/"+url.PathEscape(ns), body, &out); err != nil {
		return nil, err
	}
	res := &ProvisionResult{Namespace: out.Namespace, UpdatedAt: out.UpdatedAt}
	if out.APIKey != nil {
		res.APIKey = *out.APIKey
	}
	return res, nil
}

// APIError is a non-2xx admin-plane response.
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("codohue/admin: %d %s: %s", e.StatusCode, e.Code, e.Message)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("codohue/admin: marshal request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("codohue/admin: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.adminKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("codohue/admin: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // body close errors are not actionable

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{StatusCode: resp.StatusCode, Code: "unknown"}
		var wire struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&wire); decodeErr == nil && wire.Error.Code != "" {
			apiErr.Code = wire.Error.Code
			apiErr.Message = wire.Error.Message
		}
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("codohue/admin: decode response: %w", err)
	}
	return nil
}
