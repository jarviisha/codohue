//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	goredis "github.com/redis/go-redis/v9"
)

const (
	testPort = "12001"
	baseURL  = "http://localhost:" + testPort
	apiBin   = "../tmp/api"
	// testNS is a fixed namespace name used across all tests. It is created
	// in TestMain and deleted both before (to clear stale data) and after tests.
	testNS = "e2e_suite"
)

var (
	adminKey  string
	nsKey     string // plaintext API key for testNS, set by createTestNamespace
	testDB    *pgxpool.Pool
	testRedis *goredis.Client
)

func TestMain(m *testing.M) {
	// Load .env from project root. The e2e package runs with e2e/ as working dir.
	_ = godotenv.Load("../.env")

	adminKey = envOrDefault("RECOMMENDER_API_KEY", "dev-secret-key")
	dbURL := envOrDefault("DATABASE_URL", "postgres://codohue:secret@localhost:5432/codohue?sslmode=disable")
	redisURL := envOrDefault("REDIS_URL", "redis://localhost:6379")
	qdrantHost := envOrDefault("QDRANT_HOST", "localhost")
	qdrantPort := envOrDefault("QDRANT_PORT", "6334")

	var err error
	testDB, err = pgxpool.New(context.Background(), dbURL)
	if err != nil {
		fatalf("connect postgres: %v\nIs postgres running? Run: make up-infra", err)
	}
	defer testDB.Close()

	redisOpts, err := goredis.ParseURL(redisURL)
	if err != nil {
		fatalf("parse redis URL: %v", err)
	}
	testRedis = goredis.NewClient(redisOpts)
	defer testRedis.Close()

	// Clean up any data left by a previously interrupted run.
	cleanupTestNS()

	// Redirect subprocess output to a temp file. Using *os.File avoids the
	// "WaitDelay expired before I/O complete" error that occurs when Go's exec
	// package creates drain goroutines for pipes that outlive os.Exit().
	logFile, err := os.CreateTemp("", "e2e-api-*.log")
	if err != nil {
		fatalf("create server log file: %v", err)
	}
	defer os.Remove(logFile.Name())

	cmd := exec.Command(apiBin) //nolint:gosec
	cmd.Env = append(os.Environ(),
		"DATABASE_URL="+dbURL,
		"REDIS_URL="+redisURL,
		"QDRANT_HOST="+qdrantHost,
		"QDRANT_PORT="+qdrantPort,
		"RECOMMENDER_API_KEY="+adminKey,
		"API_PORT="+testPort,
		"LOG_FORMAT=text",
		"BATCH_INTERVAL_MINUTES=60",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		fatalf("start api binary %q: %v\nRun: make build-api", apiBin, err)
	}

	if err := waitReady(baseURL+"/ping", 20*time.Second); err != nil {
		_ = cmd.Process.Kill()
		fatalf("api not ready after 20s: %v\nServer logs: %s", err, logFile.Name())
	}

	nsKey, err = createTestNamespace()
	if err != nil {
		_ = cmd.Process.Kill()
		fatalf("create test namespace: %v", err)
	}

	code := m.Run()
	cleanupTestNS()

	// Kill the subprocess and wait for it to exit cleanly before os.Exit so that
	// all file descriptors are released and no "WaitDelay expired" warning appears.
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	logFile.Close()

	os.Exit(code)
}

// waitReady polls url until it returns HTTP 200 or timeout expires.
func waitReady(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("not ready after %s", timeout)
}

// createTestNamespace upserts testNS with a known config and returns the plaintext API key.
func createTestNamespace() (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"action_weights": map[string]float64{"click": 1.0, "like": 2.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  4,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})

	req, _ := http.NewRequest(http.MethodPut,
		baseURL+"/v1/config/namespaces/"+testNS,
		bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+adminKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}

	var body struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if body.APIKey == "" {
		return "", fmt.Errorf("api_key empty — namespace may already exist; run cleanupTestNS manually")
	}
	return body.APIKey, nil
}

// cleanupTestNS removes all postgres data and Redis cache keys for testNS.
// Qdrant collections are left in place; they are idempotently re-created by the
// BYOE endpoints and do not affect test correctness across runs.
func cleanupTestNS() {
	ctx := context.Background()
	_, _ = testDB.Exec(ctx, "DELETE FROM events WHERE namespace = $1", testNS)
	_, _ = testDB.Exec(ctx, "DELETE FROM id_mappings WHERE namespace = $1", testNS)
	_, _ = testDB.Exec(ctx, "DELETE FROM namespace_configs WHERE namespace = $1", testNS)

	// Delete recommendation cache (rec:<ns>:*) and trending cache for testNS.
	// Without this, stale cached responses from previous runs bleed into new runs.
	for _, pattern := range []string{"rec:" + testNS + ":*", "trending:" + testNS} {
		var cursor uint64
		for {
			keys, next, err := testRedis.Scan(ctx, cursor, pattern, 100).Result()
			if err != nil {
				break
			}
			if len(keys) > 0 {
				testRedis.Del(ctx, keys...) //nolint:errcheck
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "e2e setup: "+format+"\n", args...)
	os.Exit(1)
}
