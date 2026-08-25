package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	qdrantpb "github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"
)

func TestHealthzAllOK(t *testing.T) {
	withHealthzChecks(t,
		func(context.Context, *pgxpool.Pool) string { return "ok" },
		func(context.Context, *goredis.Client) string { return "ok" },
		func(context.Context, *qdrantpb.Client) string { return "ok" },
	)
	handler := healthzHandler(nil, nil, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status ok, got %q", body["status"])
	}
}

func TestHealthzPostgresDown(t *testing.T) {
	withHealthzChecks(t,
		func(context.Context, *pgxpool.Pool) string { return "error: connection refused" },
		func(context.Context, *goredis.Client) string { return "ok" },
		func(context.Context, *qdrantpb.Client) string { return "ok" },
	)
	handler := healthzHandler(nil, nil, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "degraded" {
		t.Errorf("expected status degraded, got %q", body["status"])
	}
}

func TestHealthzRedisDown(t *testing.T) {
	withHealthzChecks(t,
		func(context.Context, *pgxpool.Pool) string { return "ok" },
		func(context.Context, *goredis.Client) string { return "error: dial tcp" },
		func(context.Context, *qdrantpb.Client) string { return "ok" },
	)
	handler := healthzHandler(nil, nil, nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rr.Code)
	}
}

// Compile-time checks to ensure the real types are referenced, preventing drift.
var _ *pgxpool.Pool
var _ *goredis.Client
var _ *qdrantpb.Client

func withHealthzChecks(
	t *testing.T,
	pg func(context.Context, *pgxpool.Pool) string,
	redis func(context.Context, *goredis.Client) string,
	qdrant func(context.Context, *qdrantpb.Client) string,
) {
	t.Helper()
	origPG := checkPostgresFn
	origRedis := checkRedisFn
	origQdrant := checkQdrantFn
	t.Cleanup(func() {
		checkPostgresFn = origPG
		checkRedisFn = origRedis
		checkQdrantFn = origQdrant
	})
	checkPostgresFn = pg
	checkRedisFn = redis
	checkQdrantFn = qdrant
}

// Public /healthz is reachable without any credential, so it must carry no
// raw dependency data: a connection error string leaks the database host,
// port, and often the username to anyone who can reach the port.
func TestHealthz_PublicResponseIsSanitized(t *testing.T) {
	withHealthzChecks(t,
		func(context.Context, *pgxpool.Pool) string {
			return "error: dial tcp 10.0.3.7:5432: connect: connection refused"
		},
		func(context.Context, *goredis.Client) string { return "error: redis://cache-01:6379 unreachable" },
		func(context.Context, *qdrantpb.Client) string { return "ok" },
	)
	handler := observabilityHealthHandler("obs-token",
		healthzHandler(nil, nil, nil), healthzDetailsHandler(nil, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	for _, leak := range []string{"10.0.3.7", "5432", "cache-01", "6379", "postgres", "redis", "qdrant"} {
		if strings.Contains(body, leak) {
			t.Errorf("public health leaked %q: %s", leak, body)
		}
	}
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("aggregate status must still be reported: got %d", rr.Code)
	}
}

// details=true is the operator view: it names components and their errors, so
// it needs the observability credential — and only that credential.
func TestHealthz_DetailsRequiresTheObservabilityToken(t *testing.T) {
	withHealthzChecks(t,
		func(context.Context, *pgxpool.Pool) string { return "error: connection refused" },
		func(context.Context, *goredis.Client) string { return "ok" },
		func(context.Context, *qdrantpb.Client) string { return "ok" },
	)
	handler := observabilityHealthHandler("obs-token",
		healthzHandler(nil, nil, nil), healthzDetailsHandler(nil, nil, nil))

	for _, tc := range []struct {
		name     string
		header   string
		wantCode int
		wantLeak bool
	}{
		{"no credential", "", http.StatusUnauthorized, false},
		{"admin key is not a substitute", "Bearer dev-secret-key", http.StatusUnauthorized, false},
		{"observability token", "Bearer obs-token", http.StatusServiceUnavailable, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz?details=true", http.NoBody)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != tc.wantCode {
				t.Fatalf("status=%d, want %d (body %s)", rr.Code, tc.wantCode, rr.Body.String())
			}
			hasDetail := strings.Contains(rr.Body.String(), "connection refused")
			if hasDetail != tc.wantLeak {
				t.Errorf("component detail exposed=%v, want %v: %s", hasDetail, tc.wantLeak, rr.Body.String())
			}
		})
	}
}

// A bad Authorization header on plain /healthz must not turn the liveness
// probe into a 401 — the probe never sends one, and a rejected probe would
// take a healthy instance out of rotation.
func TestHealthz_InvalidHeaderOnPlainHealthIsIgnored(t *testing.T) {
	withHealthzChecks(t,
		func(context.Context, *pgxpool.Pool) string { return "ok" },
		func(context.Context, *goredis.Client) string { return "ok" },
		func(context.Context, *qdrantpb.Client) string { return "ok" },
	)
	handler := observabilityHealthHandler("obs-token",
		healthzHandler(nil, nil, nil), healthzDetailsHandler(nil, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", http.NoBody)
	req.Header.Set("Authorization", "Bearer nonsense")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("plain health with a bad header = %d, want 200", rr.Code)
	}
}

// With no token configured there is nothing to protect the detailed view, so
// it must not exist at all rather than fall back to the public body.
func TestHealthz_DetailsIs404WhenUnconfigured(t *testing.T) {
	withHealthzChecks(t,
		func(context.Context, *pgxpool.Pool) string { return "ok" },
		func(context.Context, *goredis.Client) string { return "ok" },
		func(context.Context, *qdrantpb.Client) string { return "ok" },
	)
	handler := observabilityHealthHandler("",
		healthzHandler(nil, nil, nil), healthzDetailsHandler(nil, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz?details=true", http.NoBody)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", rr.Code)
	}
}
