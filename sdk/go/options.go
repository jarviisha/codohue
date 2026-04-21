package codohue

import (
	"net/http"
	"time"
)

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the http.Client used for all requests. Useful for
// injecting custom transports, mTLS config, or fake clients in tests.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// WithTimeout sets the http.Client timeout. Has no effect when a custom
// http.Client is supplied via WithHTTPClient.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.http.Timeout = d
		}
	}
}

// WithUserAgent overrides the User-Agent header sent on every request.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithRetries sets the number of automatic retries for idempotent GET
// requests on transient failures (network errors or 5xx responses). Defaults
// to 2 when this option is not supplied. A value of 0 disables retries.
// Non-idempotent methods (POST/PUT/DELETE) are never auto-retried.
func WithRetries(n int) Option {
	return func(c *Client) {
		if n >= 0 {
			c.retries = n
		}
	}
}

// WithRequestHook installs a hook that runs just before each HTTP request is
// sent. Typical uses include injecting tracing headers or correlation IDs.
// The hook must not retain the request after it returns.
func WithRequestHook(hook func(*http.Request)) Option {
	return func(c *Client) { c.requestHook = hook }
}
