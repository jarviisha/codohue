//go:build e2e

package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// newJSONRequest builds an HTTP request with an optional JSON-encoded body.
// Unlike doRequest it does not execute the request, so callers can attach
// cookies, a context, or extra headers first.
func newJSONRequest(t testing.TB, method, url string, body any) *http.Request {
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
	return req
}

// decodeJSONBody decodes resp.Body into v without asserting a status code, for
// non-200 success responses (e.g. 202 Accepted). The caller closes resp.Body.
func decodeJSONBody(resp *http.Response, v any) error {
	return json.NewDecoder(resp.Body).Decode(v)
}

// The admin plane (cmd/admin, port 2002) is booted on demand by the first
// admin test via ensureAdminServer. It is killed in TestMain teardown. Tests
// that only build cmd/api (the test-e2e-api subset) never call
// ensureAdminServer, so they do not require ../tmp/admin to exist.
const (
	adminBin     = "../tmp/admin"
	adminPort    = "12002"
	adminBaseURL = "http://localhost:" + adminPort
)

var (
	adminOnce    sync.Once
	adminCmd     *exec.Cmd
	adminLogPath string
	adminStartup error
)

// ensureAdminServer starts the cmd/admin binary exactly once for the whole
// e2e run, pointing it at the same infra plus the e2e api on testPort so its
// /healthz proxy and event-injection paths resolve. It blocks until the admin
// HTTP listener accepts a request or 20s elapse.
func ensureAdminServer(t testing.TB) {
	t.Helper()

	adminOnce.Do(func() {
		logFile, err := os.CreateTemp("", "e2e-admin-*.log")
		if err != nil {
			adminStartup = err
			return
		}
		adminLogPath = logFile.Name()

		cmd := exec.Command(adminBin) //nolint:gosec
		cmd.Env = append(os.Environ(),
			"DATABASE_URL="+envOrDefault("DATABASE_URL", "postgres://codohue:secret@localhost:5432/codohue?sslmode=disable"),
			"REDIS_URL="+envOrDefault("REDIS_URL", "redis://localhost:6379"),
			"QDRANT_HOST="+envOrDefault("QDRANT_HOST", "localhost"),
			"QDRANT_PORT="+envOrDefault("QDRANT_PORT", "6334"),
			"CODOHUE_ADMIN_API_KEY="+adminKey,
			"CODOHUE_ADMIN_PORT="+adminPort,
			"CODOHUE_API_URL="+baseURL,
			"CODOHUE_LOG_FORMAT=text",
		)
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		if err := cmd.Start(); err != nil {
			_ = logFile.Close()
			adminStartup = err
			return
		}
		adminCmd = cmd

		// The admin server exposes no unauthenticated 200 probe, so readiness
		// is "the listener answers": POST a bogus login and accept any HTTP
		// status (401 in practice) as proof the router is up.
		adminStartup = waitForHTTPResponse(adminBaseURL+"/api/v1/auth/sessions", 20*time.Second)
	})

	if adminStartup != nil {
		logs, _ := os.ReadFile(adminLogPath)
		t.Fatalf("admin server not ready: %v\nRun: make build-admin\nAdmin logs: %s",
			adminStartup, strings.TrimSpace(string(logs)))
	}
}

// stopAdminServer terminates the shared admin subprocess. It is a no-op when
// no admin test ran. Called from TestMain after m.Run().
func stopAdminServer() {
	if adminCmd == nil {
		return
	}
	_ = adminCmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- adminCmd.Wait() }()
	select {
	case <-time.After(5 * time.Second):
		_ = adminCmd.Process.Kill()
		<-done
	case <-done:
	}
	if adminLogPath != "" {
		_ = os.Remove(adminLogPath)
	}
}

// waitForHTTPResponse polls url with POST until any HTTP response is received
// (i.e. the listener is accepting) or the timeout expires. Unlike waitReady it
// does not require a 200 — it only proves the server is up and routing.
func waitForHTTPResponse(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Post(url, "application/json", strings.NewReader(`{}`)) //nolint:noctx
		if err == nil {
			resp.Body.Close()
			return nil
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
	return lastErr
}

// adminLogin performs the session-create handshake against the admin server
// and returns the session cookie for use on protected requests. It fails the
// test if the server does not mint a session cookie.
func adminLogin(t testing.TB) *http.Cookie {
	t.Helper()
	ensureAdminServer(t)

	resp := doRequest(t, http.MethodPost, adminBaseURL+"/api/v1/auth/sessions", "", map[string]any{
		"api_key": adminKey,
	})
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusCreated)

	for _, c := range resp.Cookies() {
		if c.Name == "codohue_admin_session" && c.Value != "" {
			return c
		}
	}
	t.Fatal("login did not set codohue_admin_session cookie")
	return nil
}

// adminRequest fires a request at the admin server with the session cookie
// attached. The caller closes resp.Body.
func adminRequest(t testing.TB, method, path string, cookie *http.Cookie, body any) *http.Response {
	t.Helper()

	req := newJSONRequest(t, method, adminBaseURL+path, body)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("admin request %s %s: %v", method, path, err)
	}
	return resp
}

func TestAdmin_SessionLifecycleGuardsProtectedRoutes(t *testing.T) {
	ensureAdminServer(t)

	// Wrong key is rejected.
	wrong := doRequest(t, http.MethodPost, adminBaseURL+"/api/v1/auth/sessions", "", map[string]any{
		"api_key": "definitely-not-the-key",
	})
	assertStatus(t, wrong, http.StatusUnauthorized)
	wrong.Body.Close()

	// Protected route without a session is rejected.
	noSession := adminRequest(t, http.MethodGet, "/api/admin/v1/overview", nil, nil)
	assertStatus(t, noSession, http.StatusUnauthorized)
	noSession.Body.Close()

	// Correct key mints a session.
	cookie := adminLogin(t)

	// Protected route with the session now succeeds.
	ok := adminRequest(t, http.MethodGet, "/api/admin/v1/overview", cookie, nil)
	assertStatus(t, ok, http.StatusOK)
	ok.Body.Close()

	// Logout clears the session.
	logout := adminRequest(t, http.MethodDelete, "/api/v1/auth/sessions/current", cookie, nil)
	assertStatus(t, logout, http.StatusNoContent)
	logout.Body.Close()
}

func TestAdmin_OverviewAggregateShape(t *testing.T) {
	cookie := adminLogin(t)

	resp := adminRequest(t, http.MethodGet, "/api/admin/v1/overview", cookie, nil)
	var body struct {
		GeneratedAt string `json:"generated_at"`
		Health      struct {
			Status string `json:"status"`
		} `json:"health"`
		Namespaces []struct {
			Namespace string `json:"namespace"`
		} `json:"namespaces"`
	}
	decodeJSON(t, resp, &body)

	if body.GeneratedAt == "" {
		t.Fatal("overview generated_at is empty")
	}
	if body.Health.Status == "" {
		t.Fatal("overview health.status is empty")
	}
	if body.Namespaces == nil {
		t.Fatal("overview namespaces must not be null")
	}
}

func TestAdmin_PingStreamEmitsTickFrame(t *testing.T) {
	cookie := adminLogin(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := newJSONRequest(t, http.MethodGet, adminBaseURL+"/api/admin/v1/ping/stream", nil)
	req = req.WithContext(ctx)
	req.AddCookie(cookie)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open ping stream: %v", err)
	}
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	// PingStream sends an immediate `event: tick` frame on connect; read until
	// we see it (or the context deadline aborts the blocking Scan).
	scanner := bufio.NewScanner(resp.Body)
	sawTick := false
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "event: tick" {
			sawTick = true
			break
		}
	}
	if !sawTick {
		t.Fatalf("did not receive an SSE tick frame: %v", scanner.Err())
	}
}

func TestAdmin_BatchRunLifecycleCreatesAndExposesRun(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "admin_batch_run", map[string]any{
		"action_weights":  map[string]float64{"VIEW": 1.0, "LIKE": 4.0},
		"lambda":          0.01,
		"gamma":           0.5,
		"max_results":     20,
		"dense_source":    "disabled",
		"seen_items_days": 30,
	})

	cookie := adminLogin(t)

	// Trigger a batch run for the namespace. CreateBatchRun runs the cron job
	// synchronously, so the response already carries the persisted run id.
	create := adminRequest(t, http.MethodPost,
		"/api/admin/v1/namespaces/"+namespace+"/batch-runs", cookie, nil)
	var created struct {
		ID        int64  `json:"id"`
		Namespace string `json:"namespace"`
		Status    string `json:"status"`
	}
	defer create.Body.Close()
	assertStatus(t, create, http.StatusAccepted)
	if err := decodeJSONBody(create, &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	if created.Namespace != namespace {
		t.Fatalf("namespace = %q, want %q", created.Namespace, namespace)
	}
	if created.ID <= 0 {
		t.Fatalf("batch run id = %d, want > 0", created.ID)
	}
	if loc := create.Header.Get("Location"); !strings.HasSuffix(loc, "/batch-runs/"+strconv.FormatInt(created.ID, 10)) {
		t.Fatalf("Location = %q, want suffix /batch-runs/%d", loc, created.ID)
	}

	// The run is retrievable through the detail endpoint.
	detail := adminRequest(t, http.MethodGet,
		"/api/admin/v1/batch-runs/"+strconv.FormatInt(created.ID, 10), cookie, nil)
	var got struct {
		ID        int64  `json:"id"`
		Namespace string `json:"namespace"`
	}
	decodeJSON(t, detail, &got)

	if got.ID != created.ID {
		t.Fatalf("detail id = %d, want %d", got.ID, created.ID)
	}
	if got.Namespace != namespace {
		t.Fatalf("detail namespace = %q, want %q", got.Namespace, namespace)
	}
}

// TestAdmin_ProxyReadsReachKeyedNamespace guards the admin panel's proxied
// data-plane reads. testNS has a provisioned api key, and the admin server
// proxies to cmd/api with the global admin key — which must be accepted for
// every namespace, or the subject-recommendations / trending panels 401 for
// any properly provisioned namespace.
func TestAdmin_ProxyReadsReachKeyedNamespace(t *testing.T) {
	cookie := adminLogin(t)

	for _, path := range []string{
		"/api/admin/v1/namespaces/" + testNS + "/subjects/proxy_probe_subj/recommendations?limit=20&debug=true",
		"/api/admin/v1/namespaces/" + testNS + "/trending?limit=20",
	} {
		resp := adminRequest(t, http.MethodGet, path, cookie, nil)
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Fatalf("%s: proxied read 401'd for a keyed namespace — the admin global key must reach it: %s", path, body)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d: %s", path, resp.StatusCode, body)
		}
	}
}
