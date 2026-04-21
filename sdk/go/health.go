package codohue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// HealthStatus is the response from GET /healthz. Status is "ok" when all
// dependencies report healthy; otherwise "degraded". The per-component fields
// contain either "ok" or "error: <detail>".
type HealthStatus struct {
	Status   string `json:"status"`
	Postgres string `json:"postgres"`
	Redis    string `json:"redis"`
	Qdrant   string `json:"qdrant"`
}

// Ping calls GET /ping. Returns nil when the server responds 2xx.
func (c *Client) Ping(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, "/ping", "", nil, nil, nil)
}

// Healthz calls GET /healthz. On a healthy response (200), it returns the
// parsed status and a nil error. On a degraded response (503), it still
// returns the parsed *HealthStatus (so callers can see which component is
// down) alongside an *APIError that matches ErrDegraded via errors.Is. Any
// other failure (transport error, unparseable body, other status) returns a
// nil status and a non-nil error.
func (c *Client) Healthz(ctx context.Context) (*HealthStatus, error) {
	var lastErr error
	for attempt := 0; attempt <= c.retries; attempt++ {
		if attempt > 0 {
			if err := sleepWithContext(ctx, backoffDelay(attempt)); err != nil {
				return nil, err
			}
		}

		out, err := c.healthzOnce(ctx)
		if err == nil || errors.Is(err, ErrDegraded) {
			return out, err
		}
		if !isRetryable(http.MethodGet, err) || attempt == c.retries {
			return nil, err
		}
		lastErr = err
	}

	return nil, lastErr
}

func (c *Client) healthzOnce(ctx context.Context) (*HealthStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("codohue: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if c.requestHook != nil {
		c.requestHook(req)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &transportError{err: fmt.Errorf("codohue: send request: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // body close errors are not actionable

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("codohue: read response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var out HealthStatus
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("codohue: decode response: %w", err)
		}
		return &out, nil
	case http.StatusServiceUnavailable:
		var out HealthStatus
		if err := json.Unmarshal(raw, &out); err != nil {
			// Body was not the expected health shape — fall back to the
			// generic error path so callers still get an *APIError.
			return nil, parseAPIError(resp.StatusCode, raw)
		}
		return &out, &APIError{
			Status:  resp.StatusCode,
			Code:    codeServiceDegraded,
			Message: "one or more dependencies are unhealthy",
		}
	default:
		return nil, parseAPIError(resp.StatusCode, raw)
	}
}
