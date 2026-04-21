package codohue

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.Handler, opts ...Option) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, opts...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c, srv
}

func TestDoInjectsAuthAndUserAgent(t *testing.T) {
	t.Parallel()

	gotAuth := ""
	gotUA := ""
	gotAccept := ""
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusNoContent)
	})
	c, _ := newTestClient(t, handler, WithUserAgent("test-ua"))

	err := c.do(context.Background(), http.MethodGet, "/thing", "sekret", nil, nil, nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if gotAuth != "Bearer sekret" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotUA != "test-ua" {
		t.Errorf("User-Agent = %q", gotUA)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
}

func TestDoSkipsAuthWhenEmpty(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("Authorization"); v != "" {
			t.Errorf("expected no Authorization header, got %q", v)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	c, _ := newTestClient(t, handler)

	if err := c.do(context.Background(), http.MethodGet, "/", "", nil, nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
}

func TestDoDecodesJSONResponse(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"hello":"world"}`)
	})
	c, _ := newTestClient(t, handler)

	var out struct {
		Hello string `json:"hello"`
	}
	if err := c.do(context.Background(), http.MethodGet, "/", "", nil, nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if out.Hello != "world" {
		t.Errorf("decoded = %+v", out)
	}
}

func TestDoMapsErrorEnvelope(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":"bad","message":"boom"}}`)
	})
	c, _ := newTestClient(t, handler)

	err := c.do(context.Background(), http.MethodGet, "/", "", nil, nil, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %v", err)
	}
	if apiErr.Status != 400 || apiErr.Code != "bad" || apiErr.Message != "boom" {
		t.Errorf("apiErr = %+v", apiErr)
	}
	if !errors.Is(err, ErrBadRequest) {
		t.Errorf("expected ErrBadRequest match")
	}
}

func TestDoFallbackErrorMessageWhenBodyMissing(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	c, _ := newTestClient(t, handler)

	err := c.do(context.Background(), http.MethodPost, "/", "", nil, struct{}{}, nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.Code != "unknown" || !strings.Contains(apiErr.Message, "500") {
		t.Errorf("fallback error fields wrong: %+v", apiErr)
	}
}

func TestDoRetriesGETOn5xx(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	c, _ := newTestClient(t, handler, WithRetries(3))

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.do(context.Background(), http.MethodGet, "/", "", nil, nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
	if !out.OK {
		t.Errorf("expected success payload")
	}
}

func TestDoDefaultRetriesTwoOn5xx(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	// No WithRetries — expect default (2) to apply, yielding 1 + 2 = 3 total calls.
	c, _ := newTestClient(t, handler)

	err := c.do(context.Background(), http.MethodGet, "/", "", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3 (1 initial + 2 default retries)", got)
	}
}

func TestDoDoesNotRetryPOST(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	c, _ := newTestClient(t, handler, WithRetries(5))

	err := c.do(context.Background(), http.MethodPost, "/", "", nil, struct{}{}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (POST must not retry)", got)
	}
}

func TestDoRespectsContextCancel(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	c, _ := newTestClient(t, handler)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.do(ctx, http.MethodGet, "/", "", nil, nil, nil)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestDoCallsRequestHook(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Trace-ID") != "abc" {
			t.Errorf("hook did not set header; got %q", r.Header.Get("X-Trace-ID"))
		}
		w.WriteHeader(http.StatusNoContent)
	})
	hook := func(req *http.Request) { req.Header.Set("X-Trace-ID", "abc") }
	c, _ := newTestClient(t, handler, WithRequestHook(hook))

	if err := c.do(context.Background(), http.MethodGet, "/", "", nil, nil, nil); err != nil {
		t.Fatalf("do: %v", err)
	}
}

func TestDoRetriesOnTransportFailure(t *testing.T) {
	t.Parallel()

	var calls int32
	// Hijack the connection and close it with no response for the first two
	// calls to force a transport-level error on the client, then respond
	// normally so the retry loop can recover.
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n <= 2 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Errorf("ResponseWriter does not implement Hijacker")
				return
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("Hijack: %v", err)
				return
			}
			_ = conn.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	})
	c, _ := newTestClient(t, handler, WithRetries(3))

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.do(context.Background(), http.MethodGet, "/", "", nil, nil, &out); err != nil {
		t.Fatalf("do: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3 (2 transport failures + 1 success)", got)
	}
	if !out.OK {
		t.Errorf("expected ok payload after retry recovery")
	}
}

func TestDoDoesNotRetryDecodeError(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"oops":`) // malformed JSON, 200 OK
	})
	c, _ := newTestClient(t, handler, WithRetries(3))

	var out struct{}
	err := c.do(context.Background(), http.MethodGet, "/", "", nil, nil, &out)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (decode error must not retry)", got)
	}
}

func TestIsRetryable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		method string
		err    error
		want   bool
	}{
		{"POST never retries even on 5xx", http.MethodPost, &APIError{Status: 503}, false},
		{"GET retries 500", http.MethodGet, &APIError{Status: 500}, true},
		{"GET retries 503", http.MethodGet, &APIError{Status: 503}, true},
		{"GET does not retry 4xx", http.MethodGet, &APIError{Status: 429}, false},
		{"GET retries transport error", http.MethodGet, &transportError{err: errors.New("connection reset")}, true},
		{"GET does not retry plain error (e.g. decode)", http.MethodGet, errors.New("decode failed"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryable(tc.method, tc.err); got != tc.want {
				t.Errorf("isRetryable(%s, %v) = %v, want %v", tc.method, tc.err, got, tc.want)
			}
		})
	}
}

// sanity: backoffDelay never returns a non-positive duration.
func TestBackoffDelayPositive(t *testing.T) {
	t.Parallel()

	for attempt := 1; attempt <= 10; attempt++ {
		d := backoffDelay(attempt)
		if d <= 0 {
			t.Errorf("backoffDelay(%d) = %v, want > 0", attempt, d)
		}
	}
}
