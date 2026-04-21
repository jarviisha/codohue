package codohue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

const (
	maxErrorBodyBytes = 1 << 20 // 1 MiB cap on error body read
	retryBaseDelay    = 100 * time.Millisecond
)

// transportError marks a failure whose root cause is the HTTP transport —
// the request never got a full response from the server. Only these are
// considered transient for GET retry. Errors from request construction or
// response decoding are permanent and must not be retried.
type transportError struct{ err error }

func (e *transportError) Error() string { return e.err.Error() }
func (e *transportError) Unwrap() error { return e.err }

// do executes an HTTP request with optional JSON body, decodes the JSON
// response into out (when non-nil), and maps non-2xx responses to *APIError.
//
// bearer, when non-empty, is sent as "Authorization: Bearer <token>".
// GET requests are retried up to c.retries times on transient failures
// (network errors or 5xx responses).
func (c *Client) do(ctx context.Context, method, path, bearer string, query url.Values, body, out any) error {
	reqURL := c.baseURL + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("codohue: marshal request: %w", err)
		}
		bodyBytes = b
	}

	retries := 0
	if method == http.MethodGet {
		retries = c.retries
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			if err := sleepWithContext(ctx, backoffDelay(attempt)); err != nil {
				return err
			}
		}

		err := c.sendOnce(ctx, method, reqURL, bearer, bodyBytes, out)
		if err == nil {
			return nil
		}
		if !isRetryable(method, err) || attempt == retries {
			return err
		}
		lastErr = err
	}
	return lastErr
}

func (c *Client) sendOnce(ctx context.Context, method, reqURL, bearer string, bodyBytes []byte, out any) error {
	var reqBody io.Reader
	if bodyBytes != nil {
		reqBody = bytes.NewReader(bodyBytes)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
	if err != nil {
		return fmt.Errorf("codohue: build request: %w", err)
	}
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if c.requestHook != nil {
		c.requestHook(req)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return &transportError{err: fmt.Errorf("codohue: send request: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // body close errors are not actionable

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || resp.StatusCode == http.StatusNoContent {
			_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // draining body; copy error is not actionable
			return nil
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("codohue: decode response: %w", err)
		}
		return nil
	}

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes)) //nolint:errcheck // best-effort read of error body; parseAPIError tolerates empty input
	return parseAPIError(resp.StatusCode, raw)
}

func parseAPIError(status int, body []byte) *APIError {
	e := &APIError{Status: status}
	if len(body) > 0 {
		var wire codohuetypes.ErrorResponse
		if err := json.Unmarshal(body, &wire); err == nil {
			e.Code = wire.Error.Code
			e.Message = wire.Error.Message
		}
	}
	if e.Code == "" {
		e.Code = "unknown"
	}
	if e.Message == "" {
		e.Message = fmt.Sprintf("HTTP %d", status)
	}
	return e
}

// isRetryable returns true when a failed request should be retried. Only GET
// requests that failed with a 5xx APIError or a transport failure qualify.
// Permanent errors (bad request construction, decode failures on a 2xx body)
// are never retried.
func isRetryable(method string, err error) bool {
	if method != http.MethodGet {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status >= 500
	}
	var transportErr *transportError
	return errors.As(err, &transportErr)
}

// backoffDelay returns a jittered exponential delay for the given retry attempt
// (1-indexed). Base doubles each attempt and up to 50% jitter is added.
func backoffDelay(attempt int) time.Duration {
	shift := attempt - 1
	if shift > 6 {
		shift = 6 // cap to avoid overflow / excessive delay
	}
	base := retryBaseDelay << uint(shift)
	jitter := time.Duration(rand.Int64N(int64(base / 2))) //nolint:gosec // non-crypto jitter
	return base + jitter
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("codohue: backoff aborted: %w", ctx.Err())
	}
}
