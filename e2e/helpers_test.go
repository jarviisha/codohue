//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/jarviisha/codohue/internal/nsconfig"
	qdrant "github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"
)

// doRequest fires an HTTP request and returns the response.
// The caller is responsible for closing resp.Body.
// If token is non-empty it is sent as a Bearer token.
// If body is non-nil it is JSON-encoded and Content-Type is set accordingly.
func doRequest(t testing.TB, method, url, token string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	return resp
}

// assertStatus fails the test if resp.StatusCode != want.
// On failure it reads and prints the response body before calling t.Fatalf.
func assertStatus(t testing.TB, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected HTTP %d, got %d: %s", want, resp.StatusCode, bytes.TrimSpace(body))
	}
}

// doRawPost fires a POST request with a raw string body (Content-Type: application/json).
// Use this when you need to send deliberately malformed JSON.
func doRawPost(t testing.TB, url, token, rawBody string) *http.Response {
	t.Helper()
	return doRawRequest(t, http.MethodPost, url, token, rawBody)
}

// doRawRequest fires a request with a raw string body (Content-Type: application/json).
// Use this when you need to send deliberately malformed JSON.
func doRawRequest(t testing.TB, method, url, token, rawBody string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(rawBody))
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	return resp
}

// decodeJSON asserts status 200 and decodes the JSON response body into v.
// It always closes resp.Body.
func decodeJSON(t testing.TB, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
}

func decodeErrorJSON(t testing.TB, resp *http.Response, wantStatus int) (string, string) {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != wantStatus {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP %d, got %d: %s", wantStatus, resp.StatusCode, bytes.TrimSpace(body))
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type = %q, want application/json", ct)
	}

	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON error response: %v", err)
	}
	return body.Error.Code, body.Error.Message
}

// createNamespace upserts a namespace config and returns the plaintext API key
// only when the namespace is created for the first time.
func createNamespace(t testing.TB, namespace string, payload map[string]any) string {
	t.Helper()

	if payload == nil {
		payload = defaultNamespaceConfig()
	}

	apiKey, updatedAt, err := createNamespaceRequest(namespace, payload)
	if err != nil {
		t.Fatalf("create namespace %q: %v", namespace, err)
	}
	if updatedAt.IsZero() {
		t.Fatal("updated_at is zero")
	}

	return apiKey
}

func defaultNamespaceConfig() map[string]any {
	return map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0, "LIKE": 2.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_source":   "byoe",
		"embedding_dim":  4,
		"alpha":          0.7,
		"dense_distance": "cosine",
	}
}

func createNamespaceRequest(namespace string, payload map[string]any) (string, time.Time, error) {
	if payload == nil {
		payload = defaultNamespaceConfig()
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal payload: %w", err)
	}

	var req nsconfig.UpsertRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return "", time.Time{}, fmt.Errorf("decode namespace config: %w", err)
	}

	svc := nsconfig.NewService(nsconfig.NewRepository(testDB))
	resp, err := svc.Upsert(context.Background(), namespace, &req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("upsert namespace config: %w", err)
	}
	if resp.Namespace != namespace {
		return "", time.Time{}, fmt.Errorf("namespace = %q, want %q", resp.Namespace, namespace)
	}

	return resp.APIKey, resp.UpdatedAt, nil
}

// createIsolatedNamespace provisions a namespace dedicated to a single test and
// schedules cleanup automatically.
func createIsolatedNamespace(t testing.TB, prefix string, payload map[string]any) (string, string) {
	t.Helper()

	namespace := newTestNamespace(t, prefix)
	cleanupNamespaceData(namespace)
	cleanupQdrantNamespace(t, namespace)
	t.Cleanup(func() {
		cleanupNamespaceData(namespace)
		cleanupQdrantNamespace(t, namespace)
	})

	apiKey := createNamespace(t, namespace, payload)
	if apiKey == "" {
		t.Fatalf("namespace %q did not return an api_key on create", namespace)
	}

	return namespace, apiKey
}

// newTestNamespace returns a namespace-safe name unique enough for repeated runs.
func newTestNamespace(t testing.TB, prefix string) string {
	t.Helper()

	if prefix == "" {
		prefix = "e2e"
	}

	name := strings.ToLower(prefix + "_" + t.Name())
	name = invalidNamespaceChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")

	if len(name) > 48 {
		name = name[:48]
	}

	return fmt.Sprintf("%s_%d", name, time.Now().UnixNano())
}

var invalidNamespaceChars = regexp.MustCompile(`[^a-z0-9_]+`)

func waitForCondition(t testing.TB, timeout time.Duration, fn func() (bool, error)) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ok, err := fn()
		if err == nil && ok {
			return
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("condition not met after %s: %v", timeout, lastErr)
	}
	t.Fatalf("condition not met after %s", timeout)
}

func waitForRowCount(t testing.TB, timeout time.Duration, query string, want int, args ...any) {
	t.Helper()

	waitForCondition(t, timeout, func() (bool, error) {
		var got int
		if err := testDB.QueryRow(context.Background(), query, args...).Scan(&got); err != nil {
			return false, err
		}
		return got == want, nil
	})
}

func publishEvent(t testing.TB, payload map[string]any) string {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal event payload: %v", err)
	}

	id, err := testRedis.XAdd(context.Background(), &goredis.XAddArgs{
		Stream: ingestStreamName,
		Values: map[string]any{"payload": string(data)},
	}).Result()
	if err != nil {
		t.Fatalf("publish event: %v", err)
	}
	return id
}

func publishRawEvent(t testing.TB, rawPayload string) string {
	t.Helper()

	id, err := testRedis.XAdd(context.Background(), &goredis.XAddArgs{
		Stream: ingestStreamName,
		Values: map[string]any{"payload": rawPayload},
	}).Result()
	if err != nil {
		t.Fatalf("publish raw event: %v", err)
	}
	return id
}

func waitForEventPersisted(t testing.TB, namespace, subjectID, objectID string) {
	t.Helper()
	waitForRowCount(t, 5*time.Second, `
		SELECT COUNT(*)
		FROM events
		WHERE namespace = $1 AND subject_id = $2 AND object_id = $3
	`, 1, namespace, subjectID, objectID)
}

func seedEvent(t testing.TB, namespace, subjectID, objectID, action string, weight float64, occurredAt time.Time, objectCreatedAt *time.Time) {
	t.Helper()

	_, err := testDB.Exec(context.Background(), `
		INSERT INTO events (namespace, subject_id, object_id, action, weight, occurred_at, object_created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, namespace, subjectID, objectID, action, weight, occurredAt, objectCreatedAt)
	if err != nil {
		t.Fatalf("seed event: %v", err)
	}
}

func runCronOnceUntil(t testing.TB, timeout time.Duration, condition func() (bool, error)) {
	t.Helper()

	logFile, err := os.CreateTemp("", "e2e-cron-*.log")
	if err != nil {
		t.Fatalf("create cron log file: %v", err)
	}
	defer os.Remove(logFile.Name())
	defer logFile.Close()

	cmd := exec.Command(cronBin) //nolint:gosec
	cmd.Env = append(os.Environ(),
		"DATABASE_URL="+envOrDefault("DATABASE_URL", ""),
		"REDIS_URL="+envOrDefault("REDIS_URL", ""),
		"QDRANT_HOST="+envOrDefault("QDRANT_HOST", "localhost"),
		"QDRANT_PORT="+envOrDefault("QDRANT_PORT", "6334"),
		"CODOHUE_ADMIN_API_KEY="+adminKey,
		"CODOHUE_LOG_FORMAT=text",
		"CODOHUE_BATCH_INTERVAL_MINUTES=60",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		t.Fatalf("start cron binary %q: %v", cronBin, err)
	}

	success := false
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ok, err := condition()
		if err == nil && ok {
			success = true
			break
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(200 * time.Millisecond)
	}

	_ = cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	case <-done:
	}

	if success {
		return
	}

	logs, _ := os.ReadFile(logFile.Name())
	if lastErr != nil {
		t.Fatalf("cron condition not met after %s: %v\nCron logs: %s", timeout, lastErr, strings.TrimSpace(string(logs)))
	}
	t.Fatalf("cron condition not met after %s\nCron logs: %s", timeout, strings.TrimSpace(string(logs)))
}

var (
	sharedQdrantOnce   sync.Once
	sharedQdrantClient *qdrant.Client
	sharedQdrantErr    error
)

// newQdrantTestClient returns one process-wide client. Every call used to dial
// a fresh connection and pay a version-check round trip, and the per-call
// cleanup meant a test asserting in a loop held every connection it had ever
// opened until it finished — the lifecycle race alone opened 400. Closed once
// in TestMain via closeSharedQdrantClient.
func newQdrantTestClient(t testing.TB) *qdrant.Client {
	t.Helper()

	sharedQdrantOnce.Do(func() {
		port, err := strconv.Atoi(envOrDefault("QDRANT_PORT", "6334"))
		if err != nil {
			sharedQdrantErr = fmt.Errorf("parse QDRANT_PORT: %w", err)
			return
		}
		sharedQdrantClient, sharedQdrantErr = qdrant.NewClient(&qdrant.Config{
			Host: envOrDefault("QDRANT_HOST", "localhost"),
			Port: port,
		})
	})
	if sharedQdrantErr != nil {
		t.Fatalf("new qdrant client: %v", sharedQdrantErr)
	}
	return sharedQdrantClient
}

// closeSharedQdrantClient releases the shared connection at suite teardown.
func closeSharedQdrantClient() {
	if sharedQdrantClient != nil {
		_ = sharedQdrantClient.Close()
	}
}

func cleanupQdrantNamespace(t testing.TB, namespace string) {
	t.Helper()

	client := newQdrantTestClient(t)
	for _, name := range []string{
		namespace + "_subjects",
		namespace + "_objects",
		namespace + "_subjects_dense",
		namespace + "_objects_dense",
	} {
		exists, err := client.CollectionExists(context.Background(), name)
		if err != nil {
			t.Fatalf("check qdrant collection %q: %v", name, err)
		}
		if exists {
			if err := client.DeleteCollection(context.Background(), name); err != nil {
				t.Fatalf("delete qdrant collection %q: %v", name, err)
			}
		}
	}
}

func qdrantCollectionExists(t testing.TB, collection string) bool {
	t.Helper()

	client := newQdrantTestClient(t)
	exists, err := client.CollectionExists(context.Background(), collection)
	if err != nil {
		t.Fatalf("qdrant collection exists %q: %v", collection, err)
	}
	return exists
}

func qdrantPointCount(t testing.TB, collection string) uint64 {
	t.Helper()

	client := newQdrantTestClient(t)
	count, err := client.Count(context.Background(), &qdrant.CountPoints{
		CollectionName: collection,
	})
	if err != nil {
		t.Fatalf("qdrant count %q: %v", collection, err)
	}
	return count
}

func numericIDFor(t testing.TB, stringID, namespace, entityType string) uint64 {
	t.Helper()

	// id_mappings is keyed on the composite (namespace, entity_type,
	// string_id) since migration 022, so the same string id in two
	// namespaces gets two distinct numeric ids. The namespace MUST scope the
	// lookup or a multi-tenant test resolves to the wrong tenant's id.
	var id uint64
	err := testDB.QueryRow(context.Background(), `
		SELECT numeric_id
		FROM id_mappings
		WHERE namespace = $1 AND entity_type = $2 AND string_id = $3
	`, namespace, entityType, stringID).Scan(&id)
	if err != nil {
		t.Fatalf("numeric id for %q/%q/%q: %v", namespace, stringID, entityType, err)
	}
	return id
}

func trendingKeyState(t testing.TB, namespace string) (int64, time.Duration) {
	t.Helper()

	ctx := context.Background()
	key := "trending:" + namespace
	card, err := testRedis.ZCard(ctx, key).Result()
	if err != nil {
		t.Fatalf("redis zcard %q: %v", key, err)
	}
	ttl, err := testRedis.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("redis ttl %q: %v", key, err)
	}
	return card, ttl
}

type redisGroupProgress struct {
	Name            string
	Pending         int64
	OldestPendingID string
	LastDeliveredID string
}

func redisGroupProgressFor(t testing.TB, stream, group string) redisGroupProgress {
	t.Helper()
	ctx := context.Background()
	groups, err := testRedis.XInfoGroups(ctx, stream).Result()
	if err != nil {
		t.Fatalf("xinfo groups %q: %v", stream, err)
	}
	for _, info := range groups {
		if info.Name != group {
			continue
		}
		progress := redisGroupProgress{
			Name:            info.Name,
			Pending:         info.Pending,
			LastDeliveredID: info.LastDeliveredID,
		}
		if info.Pending > 0 {
			pending, err := testRedis.XPending(ctx, stream, group).Result()
			if err != nil {
				t.Fatalf("xpending %q/%q: %v", stream, group, err)
			}
			progress.OldestPendingID = pending.Lower
		}
		return progress
	}
	t.Fatalf("consumer group %q not found on stream %q", group, stream)
	return redisGroupProgress{}
}

func ensureRedisGroup(t testing.TB, stream, group, start string) {
	t.Helper()
	err := testRedis.XGroupCreateMkStream(context.Background(), stream, group, start).Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		t.Fatalf("create consumer group %q on %q: %v", group, stream, err)
	}
}

func assertNoStreamEntryBelow(t testing.TB, stream, frontier string) {
	t.Helper()
	entries, err := testRedis.XRangeN(context.Background(), stream, "-", "("+frontier, 1).Result()
	if err != nil {
		t.Fatalf("xrange below frontier %q on %q: %v", frontier, stream, err)
	}
	if len(entries) != 0 {
		t.Fatalf("stream %q still contains entry %q below frontier %q", stream, entries[0].ID, frontier)
	}
}

func namespaceLifecycleGeneration(t testing.TB, namespace string) int64 {
	t.Helper()
	var generation int64
	if err := testDB.QueryRow(context.Background(), `
		SELECT generation FROM namespace_lifecycles WHERE namespace = $1
	`, namespace).Scan(&generation); err != nil {
		t.Fatalf("lifecycle generation for %q: %v", namespace, err)
	}
	return generation
}

func assertNoNamespaceRows(t testing.TB, table, namespace string) {
	t.Helper()
	allowed := map[string]bool{
		"events": true, "catalog_items": true, "objects": true,
		"id_mappings": true, "namespace_configs": true,
	}
	if !allowed[table] {
		t.Fatalf("unsupported namespace-owned table %q", table)
	}
	var count int
	query := "SELECT COUNT(*) FROM " + table + " WHERE namespace = $1" //nolint:gosec // table is allowlisted above.
	if err := testDB.QueryRow(context.Background(), query, namespace).Scan(&count); err != nil {
		t.Fatalf("count namespace rows in %q: %v", table, err)
	}
	if count != 0 {
		t.Fatalf("table %q has %d rows for namespace %q, want zero", table, count, namespace)
	}
}

func assertQdrantNamespaceAbsent(t testing.TB, namespace string) {
	t.Helper()
	for _, suffix := range []string{"_subjects", "_objects", "_subjects_dense", "_objects_dense"} {
		collection := namespace + suffix
		if qdrantCollectionExists(t, collection) {
			t.Errorf("Qdrant collection %q still exists", collection)
		}
	}
}

func unavailableTCPAddress(t testing.TB) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve unavailable TCP address: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close reserved TCP address: %v", err)
	}
	return address
}

// mustLifecycleLocker builds a lifecycle locker over the shared test pool. The
// locker owns its own connection pool, so the test must close it.
func mustLifecycleLocker(t testing.TB) *nslifecycle.PostgresLocker {
	t.Helper()
	locker, err := nslifecycle.NewPostgresLocker(testDB)
	if err != nil {
		t.Fatalf("new lifecycle locker: %v", err)
	}
	t.Cleanup(locker.Close)
	return locker
}
