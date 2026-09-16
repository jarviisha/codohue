package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// bootstrapActionWeights scores the Bluesky interactions this feeder produces.
// A repost carries more intent than a like because it republishes to the
// actor's own followers, and a reply costs more effort still.
var bootstrapActionWeights = map[string]float64{
	"VIEW":    1,
	"LIKE":    3,
	"SHARE":   6,
	"COMMENT": 8,
	"SKIP":    -2,
}

// bootstrap provisions the target namespace through the admin plane. The
// upsert has PATCH semantics, so re-running it on every container restart is
// safe and leaves any operator edits to unrelated fields alone.
func bootstrap(ctx context.Context, cfg config) error {
	admin, err := newAdminClient(cfg.adminURL, cfg.adminKey)
	if err != nil {
		return err
	}
	if err := admin.login(ctx); err != nil {
		return err
	}
	if err := admin.upsertNamespace(ctx, cfg.namespace, cfg.embeddingDim); err != nil {
		return err
	}
	log.Printf("namespace %q ready (dense_source=catalog, internal-hashing-ngrams@v1, dim=%d)",
		cfg.namespace, cfg.embeddingDim)
	return nil
}

// adminClient talks to cmd/admin over the session-cookie API. It exists only
// to provision the namespace — all data-plane traffic goes to Redis instead.
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

// login creates a session; the cookie jar carries it on later requests.
func (a *adminClient) login(ctx context.Context) error {
	return a.do(ctx, http.MethodPost, "/api/v1/auth/sessions",
		map[string]string{"api_key": a.apiKey}, http.StatusCreated)
}

// upsertNamespace creates or updates the namespace with catalog auto-embedding
// as the dense source, provisioned in a single request: dense_source=catalog
// is accepted here only when the strategy fields accompany it.
func (a *adminClient) upsertNamespace(ctx context.Context, ns string, dim int) error {
	var (
		alpha           = 0.7
		lambda          = 0.92
		gamma           = 0.12
		maxResults      = 20
		seenItemsDays   = 30
		trendingWindow  = 72
		trendingTTL     = 3600
		lambdaTrending  = 0.18
		denseSource     = "catalog"
		denseDistance   = "cosine"
		strategyID      = "internal-hashing-ngrams"
		strategyVersion = "v1"
	)

	body := map[string]any{
		"action_weights":           bootstrapActionWeights,
		"lambda":                   &lambda,
		"gamma":                    &gamma,
		"alpha":                    &alpha,
		"max_results":              &maxResults,
		"seen_items_days":          &seenItemsDays,
		"dense_source":             &denseSource,
		"embedding_dim":            &dim,
		"dense_distance":           &denseDistance,
		"trending_window":          &trendingWindow,
		"trending_ttl":             &trendingTTL,
		"lambda_trending":          &lambdaTrending,
		"catalog_strategy_id":      &strategyID,
		"catalog_strategy_version": &strategyVersion,
		"catalog_strategy_params":  map[string]any{"dim": dim},
	}
	// 200 on update, 201 on create.
	return a.do(ctx, http.MethodPut, "/api/admin/v1/namespaces/"+ns, body,
		http.StatusOK, http.StatusCreated)
}

// do issues a JSON request and fails when the status is not one of okStatuses.
func (a *adminClient) do(ctx context.Context, method, path string, in any, okStatuses ...int) error {
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
	for _, s := range okStatuses {
		if resp.StatusCode == s {
			return nil
		}
	}
	return fmt.Errorf("%s %s: unexpected status %d: %s", method, path, resp.StatusCode, body)
}
