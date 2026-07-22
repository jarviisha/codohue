package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// adminClient talks to the cmd/admin plane over the session-cookie API. It is
// only used to provision the namespace and enable catalog auto-embedding — the
// data-plane traffic goes through the public SDK instead.
type adminClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func newAdminClient(baseURL, apiKey string) (*adminClient, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &adminClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 15 * time.Second, Jar: jar},
	}, nil
}

// login creates a session; the cookie jar then carries the session cookie on
// every subsequent admin request.
func (a *adminClient) login(ctx context.Context) error {
	body := map[string]string{"api_key": a.apiKey}
	return a.do(ctx, http.MethodPost, "/api/v1/auth/sessions", body, nil, http.StatusCreated)
}

// upsertNamespace creates or updates the namespace config. dim is the dense
// embedding dimension; it must match the catalog strategy's "dim" param.
func (a *adminClient) upsertNamespace(ctx context.Context, ns string, dim int) error {
	weights := map[string]float64{"VIEW": 1, "LIKE": 5, "COMMENT": 8, "SHARE": 10, "SKIP": -2}
	alpha := 0.7
	lambda := 0.92
	gamma := 0.12
	// Placeholder only — the enableCatalog call that follows flips
	// dense_source to "catalog"; that is what turns catalog auto-embedding on.
	denseSource := "disabled"
	distance := "cosine"
	maxResults := 20
	seenItemsDays := 30
	trendingWindow := 72
	trendingTTL := 3600
	lambdaTrending := 0.18
	// The pump attributes catalog items to simulated users, so turn the
	// authored-objects exclusion on to exercise that filter end-to-end.
	excludeAuthored := true

	body := map[string]any{
		"action_weights":   weights,
		"lambda":           &lambda,
		"gamma":            &gamma,
		"alpha":            &alpha,
		"max_results":      &maxResults,
		"seen_items_days":  &seenItemsDays,
		"dense_source":     &denseSource,
		"embedding_dim":    &dim,
		"dense_distance":   &distance,
		"trending_window":  &trendingWindow,
		"trending_ttl":     &trendingTTL,
		"lambda_trending":  &lambdaTrending,
		"exclude_authored": &excludeAuthored,
	}
	path := "/api/admin/v1/namespaces/" + ns
	// 200 (update) and 201 (create) are both success.
	return a.do(ctx, http.MethodPut, path, body, nil, http.StatusOK, http.StatusCreated)
}

// enableCatalog turns on catalog auto-embedding with the built-in hashing
// strategy at the given dimension.
func (a *adminClient) enableCatalog(ctx context.Context, ns string, dim int) error {
	strategyID := "internal-hashing-ngrams"
	strategyVersion := "v1"
	body := map[string]any{
		"enabled":          true,
		"strategy_id":      &strategyID,
		"strategy_version": &strategyVersion,
		"params":           map[string]any{"dim": dim},
	}
	path := "/api/admin/v1/namespaces/" + ns + "/catalog"
	return a.do(ctx, http.MethodPut, path, body, nil, http.StatusOK)
}

// do issues a JSON request, decodes the response into out (when non-nil), and
// fails when the status is not one of okStatuses.
func (a *adminClient) do(ctx context.Context, method, path string, in, out any, okStatuses ...int) error {
	var reader io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, reader)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	ok := false
	for _, s := range okStatuses {
		if resp.StatusCode == s {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("%s %s: unexpected status %d: %s", method, path, resp.StatusCode, string(body))
	}
	if out != nil && len(body) > 0 {
		return json.Unmarshal(body, out)
	}
	return nil
}
