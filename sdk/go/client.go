package codohue

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	defaultUserAgent = "codohue-go-sdk/0.1"
	defaultTimeout   = 10 * time.Second
	defaultRetries   = 2
)

// Client is the HTTP client for the Codohue API.
type Client struct {
	baseURL     string
	http        *http.Client
	userAgent   string
	retries     int
	requestHook func(*http.Request)
}

// New creates a new Client pointing at the Codohue API at baseURL.
// baseURL must include the scheme, e.g. "http://localhost:2001".
func New(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("codohue: baseURL is required")
	}
	c := &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		http:      &http.Client{Timeout: defaultTimeout},
		userAgent: defaultUserAgent,
		retries:   defaultRetries,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// BaseURL returns the configured base URL with any trailing slash removed.
func (c *Client) BaseURL() string { return c.baseURL }
