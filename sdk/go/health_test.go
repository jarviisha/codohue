package codohue

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestPing(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ping" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestHealthz(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","postgres":"ok","redis":"ok","qdrant":"ok"}`))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	out, err := c.Healthz(context.Background())
	if err != nil {
		t.Fatalf("Healthz: %v", err)
	}
	if out.Status != "ok" || out.Postgres != "ok" {
		t.Errorf("got %+v", out)
	}
}

func TestHealthzDegraded(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"degraded","postgres":"ok","redis":"error: dial tcp","qdrant":"ok"}`))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	out, err := c.Healthz(context.Background())
	if err == nil {
		t.Fatal("expected non-nil error for degraded response")
	}
	if !errors.Is(err, ErrDegraded) {
		t.Errorf("errors.Is(err, ErrDegraded) = false, got %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil HealthStatus on degraded response")
	}
	if out.Status != "degraded" || out.Redis != "error: dial tcp" || out.Postgres != "ok" {
		t.Errorf("got %+v", out)
	}
}

func TestHealthzDegradedUnparseableBody(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`upstream crashed`))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	out, err := c.Healthz(context.Background())
	if err == nil {
		t.Fatal("expected error for unparseable degraded body")
	}
	if out != nil {
		t.Errorf("expected nil status when body is not JSON, got %+v", out)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Status != http.StatusServiceUnavailable {
		t.Errorf("apiErr.Status = %d", apiErr.Status)
	}
	if errors.Is(err, ErrDegraded) {
		t.Errorf("errors.Is(err, ErrDegraded) = true, want false for unparseable body")
	}
}

func TestHealthzRetriesTransportFailure(t *testing.T) {
	t.Parallel()

	var calls int32
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
		_, _ = w.Write([]byte(`{"status":"ok","postgres":"ok","redis":"ok","qdrant":"ok"}`))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	out, err := c.Healthz(context.Background())
	if err != nil {
		t.Fatalf("Healthz: %v", err)
	}
	if out == nil || out.Status != "ok" {
		t.Fatalf("unexpected status: %+v", out)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("calls = %d, want 3 (2 retries + 1 success)", got)
	}
}

func TestHealthzDoesNotRetryDegraded(t *testing.T) {
	t.Parallel()

	var calls int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"degraded","postgres":"ok","redis":"error: dial tcp","qdrant":"ok"}`))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	out, err := c.Healthz(context.Background())
	if err == nil {
		t.Fatal("expected degraded error")
	}
	if !errors.Is(err, ErrDegraded) {
		t.Fatalf("expected ErrDegraded, got %v", err)
	}
	if out == nil || out.Status != "degraded" {
		t.Fatalf("unexpected status: %+v", out)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("calls = %d, want 1 (degraded response should return immediately)", got)
	}
}
