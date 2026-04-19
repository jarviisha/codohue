package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
