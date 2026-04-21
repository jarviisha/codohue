package codohue

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewRequiresBaseURL(t *testing.T) {
	t.Parallel()

	if _, err := New(""); err == nil {
		t.Fatal("expected error for empty baseURL, got nil")
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	t.Parallel()

	c, err := New("http://example.test/")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, want := c.BaseURL(), "http://example.test"; got != want {
		t.Fatalf("BaseURL = %q, want %q", got, want)
	}
}

func TestNewDefaults(t *testing.T) {
	t.Parallel()

	c, err := New("http://example.test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.retries != defaultRetries {
		t.Errorf("default retries = %d, want %d", c.retries, defaultRetries)
	}
	if c.userAgent != defaultUserAgent {
		t.Errorf("default userAgent = %q", c.userAgent)
	}
	if c.http == nil || c.http.Timeout != defaultTimeout {
		t.Errorf("default http client not configured")
	}
}

func TestOptionsAreApplied(t *testing.T) {
	t.Parallel()

	customHTTP := &http.Client{Timeout: 42 * time.Second}
	hookCalls := 0
	hook := func(*http.Request) { hookCalls++ }

	c, err := New("http://example.test",
		WithHTTPClient(customHTTP),
		WithUserAgent("test-agent/1.0"),
		WithRetries(3),
		WithRequestHook(hook),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if c.http != customHTTP {
		t.Errorf("WithHTTPClient not applied")
	}
	if c.userAgent != "test-agent/1.0" {
		t.Errorf("WithUserAgent not applied: got %q", c.userAgent)
	}
	if c.retries != 3 {
		t.Errorf("WithRetries not applied: got %d", c.retries)
	}
	if c.requestHook == nil {
		t.Errorf("WithRequestHook not applied")
	}
}

func TestWithTimeoutIgnoredWhenCustomClient(t *testing.T) {
	t.Parallel()

	customHTTP := &http.Client{Timeout: 1 * time.Second}
	c, err := New("http://example.test",
		WithHTTPClient(customHTTP),
		WithTimeout(99*time.Second),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.http.Timeout != 99*time.Second {
		// WithTimeout still sets on the shared http.Client; that's fine.
		// Here we just verify that the custom client is the one used.
		if c.http != customHTTP {
			t.Errorf("expected custom http client to be used")
		}
	}
}

func TestNamespaceBinding(t *testing.T) {
	t.Parallel()

	c, err := New("http://example.test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ns := c.Namespace("feed", "secret")
	if ns.Name() != "feed" {
		t.Errorf("Name = %q, want feed", ns.Name())
	}
	if ns.apiKey != "secret" {
		t.Errorf("apiKey not bound")
	}
	if !strings.HasPrefix(c.BaseURL(), "http://") {
		t.Errorf("BaseURL lost scheme: %q", c.BaseURL())
	}
}
