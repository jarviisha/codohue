//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

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
		"dense_strategy": "byoe",
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

func cleanupNamespace(t testing.TB, namespace string) {
	t.Helper()
	cleanupNamespaceData(namespace)
	cleanupQdrantNamespace(t, namespace)
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
		"RECOMMENDER_API_KEY="+adminKey,
		"LOG_FORMAT=text",
		"BATCH_INTERVAL_MINUTES=60",
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

func newQdrantTestClient(t testing.TB) *qdrant.Client {
	t.Helper()

	port := mustAtoi(t, envOrDefault("QDRANT_PORT", "6334"))
	client, err := qdrant.NewClient(&qdrant.Config{
		Host: envOrDefault("QDRANT_HOST", "localhost"),
		Port: port,
	})
	if err != nil {
		t.Fatalf("new qdrant client: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})
	return client
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

	// id_mappings keys numeric_id by string_id alone (PRIMARY KEY) — namespaces
	// share the same numeric id for the same string id and multi-tenant
	// separation is achieved at the Qdrant collection level instead. The
	// namespace param is kept for call-site clarity but excluded from the
	// query so multi-tenant tests don't trip over the first-writer-wins
	// namespace stamp in the row.
	_ = namespace
	var id uint64
	err := testDB.QueryRow(context.Background(), `
		SELECT numeric_id
		FROM id_mappings
		WHERE string_id = $1 AND entity_type = $2
	`, stringID, entityType).Scan(&id)
	if err != nil {
		t.Fatalf("numeric id for %q/%q: %v", stringID, entityType, err)
	}
	return id
}

func qdrantGetSparseVector(t testing.TB, collection string, numericID uint64) *qdrant.SparseVector {
	t.Helper()

	client := newQdrantTestClient(t)
	points, err := client.Get(context.Background(), &qdrant.GetPoints{
		CollectionName: collection,
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(numericID)},
		WithVectors:    qdrant.NewWithVectorsEnable(true),
	})
	if err != nil {
		t.Fatalf("qdrant get %q/%d: %v", collection, numericID, err)
	}
	if len(points) == 0 {
		return nil
	}
	vectors := points[0].GetVectors().GetVectors().GetVectors()
	if vectors == nil {
		return nil
	}
	if vec := vectors["sparse_interactions"]; vec != nil {
		return vec.GetSparse()
	}
	return nil
}

func qdrantQuerySparse(t testing.TB, collection string, queryVec *qdrant.SparseVector, filter *qdrant.Filter, limit uint64) []*qdrant.ScoredPoint {
	t.Helper()

	client := newQdrantTestClient(t)
	resp, err := client.GetPointsClient().Search(context.Background(), &qdrant.SearchPoints{
		CollectionName: collection,
		Vector:         queryVec.Values,
		SparseIndices:  &qdrant.SparseIndices{Data: queryVec.Indices},
		VectorName:     qdrant.PtrOf("sparse_interactions"),
		Filter:         filter,
		Limit:          limit,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	if err != nil {
		t.Fatalf("qdrant sparse query %q: %v", collection, err)
	}
	return resp.GetResult()
}

func sparseVecLen(vec *qdrant.SparseVector) int {
	if vec == nil {
		return 0
	}
	return len(vec.Indices)
}

func sparseIndices(vec *qdrant.SparseVector) []uint32 {
	if vec == nil {
		return nil
	}
	return vec.Indices
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

func scanRedisKeys(t testing.TB, pattern string) []string {
	t.Helper()

	var (
		cursor uint64
		keys   []string
	)
	for {
		batch, next, err := testRedis.Scan(context.Background(), cursor, pattern, 100).Result()
		if err != nil {
			t.Fatalf("scan redis keys %q: %v", pattern, err)
		}
		keys = append(keys, batch...)
		cursor = next
		if cursor == 0 {
			return keys
		}
	}
}

func assertJSONStatus(t testing.TB, resp *http.Response, want int) {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected HTTP %d, got %d: %s", want, resp.StatusCode, bytes.TrimSpace(body))
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
}

func waitForNamespaceCacheKeys(t testing.TB, namespace string, min int) []string {
	t.Helper()

	var keys []string
	waitForCondition(t, 5*time.Second, func() (bool, error) {
		keys = scanRedisKeys(t, "rec:"+namespace+":*")
		trending := scanRedisKeys(t, "trending:"+namespace)
		keys = append(keys, trending...)
		return len(keys) >= min, nil
	})
	return keys
}

func mustAtoi(t testing.TB, value string) int {
	t.Helper()
	n, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("atoi %q: %v", value, err)
	}
	return n
}
