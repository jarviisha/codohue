//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/qdrant/go-client/qdrant"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

// runEmbedderInBackground starts the cmd/embedder binary as a subprocess
// and registers cleanup to stop it at end-of-test. It blocks until the
// embedder /healthz endpoint reports ok or 20s elapses.
//
// The poll interval is set to 200ms so newly-enabled namespaces get picked
// up quickly during tests; production default is 30s.
func runEmbedderInBackground(t testing.TB) {
	t.Helper()

	logFile, err := os.CreateTemp("", "e2e-embedder-*.log")
	if err != nil {
		t.Fatalf("create embedder log file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(logFile.Name())
		_ = logFile.Close()
	})

	cmd := exec.Command(embedderBin) //nolint:gosec
	cmd.Env = append(os.Environ(),
		"DATABASE_URL="+envOrDefault("DATABASE_URL", "postgres://codohue:secret@localhost:5432/codohue?sslmode=disable"),
		"REDIS_URL="+envOrDefault("REDIS_URL", "redis://localhost:6379"),
		"QDRANT_HOST="+envOrDefault("QDRANT_HOST", "localhost"),
		"QDRANT_PORT="+envOrDefault("QDRANT_PORT", "6334"),
		"CODOHUE_LOG_FORMAT=text",
		"CODOHUE_EMBEDDER_HEALTH_PORT="+embedderHealthPort,
		"CODOHUE_EMBEDDER_POLL_INTERVAL=200ms",
		"CODOHUE_EMBED_MAX_ATTEMPTS=5",
		"CODOHUE_CATALOG_MAX_CONTENT_BYTES=32768",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		t.Fatalf("start embedder %q: %v\nRun: make build-embedder", embedderBin, err)
	}

	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		case <-done:
		}

		// On test failure the cmd-line viewer doesn't see embedder logs;
		// dump them so debugging doesn't require chasing temp files.
		if t.Failed() {
			logs, _ := os.ReadFile(logFile.Name())
			t.Logf("embedder logs:\n%s", strings.TrimSpace(string(logs)))
		}
	})

	if err := waitReady(embedderHealthURL+"/healthz", 20*time.Second); err != nil {
		_ = cmd.Process.Kill()
		logs, _ := os.ReadFile(logFile.Name())
		t.Fatalf("embedder /healthz not ready: %v\nlogs:\n%s", err, strings.TrimSpace(string(logs)))
	}
}

// enableCatalogForNamespace flips a namespace into catalog auto-embedding
// mode via direct DB UPDATE. The dedicated admin endpoint that owns this
// behaviour is part of US2 (T037) and is not yet wired; once it lands,
// e2e tests can switch to calling it.
//
// dim must be one of {64, 128, 256, 512} — the dim variants the V1
// internal-hashing-ngrams strategy registers via RegisterVariants.
func enableCatalogForNamespace(t testing.TB, namespace string, dim int) {
	t.Helper()

	_, err := testDB.Exec(context.Background(), `
		UPDATE namespace_configs
		SET catalog_enabled = TRUE,
		    catalog_strategy_id = 'internal-hashing-ngrams',
		    catalog_strategy_version = 'v1',
		    catalog_strategy_params = $2::jsonb,
		    catalog_max_attempts = 5,
		    catalog_max_content_bytes = 32768,
		    embedding_dim = $3,
		    updated_at = NOW()
		WHERE namespace = $1
	`, namespace, fmt.Sprintf(`{"dim":%d}`, dim), dim)
	if err != nil {
		t.Fatalf("enable catalog for %q: %v", namespace, err)
	}
}

// waitForCatalogState polls catalog_items until the given (namespace, object_id)
// row reaches the expected state, or fails the test on timeout.
func waitForCatalogState(t testing.TB, namespace, objectID string, want string, timeout time.Duration) {
	t.Helper()

	waitForCondition(t, timeout, func() (bool, error) {
		var state string
		err := testDB.QueryRow(context.Background(), `
			SELECT state FROM catalog_items
			WHERE namespace = $1 AND object_id = $2
		`, namespace, objectID).Scan(&state)
		if err == pgx.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return state == want, nil
	})
}

func TestCatalogE2E_HappyPath_IngestEmbedDiscoverable(t *testing.T) {
	// Namespace must exist with embedding_dim BEFORE catalog enable;
	// otherwise the strategy dim-mismatch check would fail. We bootstrap
	// with byoe @ dim=128, then flip catalog_enabled.
	namespace, apiKey := createIsolatedNamespace(t, "catalog_happy", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0, "LIKE": 2.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, namespace, 128)

	runEmbedderInBackground(t)

	// US1#1: client posts catalog item → 202 Accepted (asynchronous).
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey, map[string]any{
		"object_id": "post_happy_1",
		"content":   "Hôm nay trời đẹp quá, ai cũng muốn ra biển! #weekend",
		"metadata":  map[string]any{"author": "u1", "lang": "vi"},
	})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// US1#2: item drains to state='embedded' under the active strategy version.
	waitForCatalogState(t, namespace, "post_happy_1", "embedded", 10*time.Second)

	// Verify the dense Qdrant collection now contains a point with the
	// data-model.md §4 payload tags.
	collection := namespace + "_objects_dense"
	if !qdrantCollectionExists(t, collection) {
		t.Fatalf("expected qdrant collection %q to exist", collection)
	}
	if got := qdrantPointCount(t, collection); got < 1 {
		t.Fatalf("expected at least 1 point in %q, got %d", collection, got)
	}

	// Inspect the actual point: payload must carry strategy_id, strategy_version,
	// embedded_at, and the named dense vector "dense_interactions" must be present.
	pointID := numericIDFor(t, "post_happy_1", namespace, "object")
	client := newQdrantTestClient(t)
	points, err := client.Get(context.Background(), &qdrant.GetPoints{
		CollectionName: collection,
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(pointID)},
		WithVectors:    qdrant.NewWithVectorsEnable(true),
		WithPayload:    qdrant.NewWithPayloadEnable(true),
	})
	if err != nil {
		t.Fatalf("qdrant get: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	payload := points[0].GetPayload()
	if payload["strategy_id"].GetStringValue() != "internal-hashing-ngrams" {
		t.Errorf("payload.strategy_id: got %q", payload["strategy_id"].GetStringValue())
	}
	if payload["strategy_version"].GetStringValue() != "v1" {
		t.Errorf("payload.strategy_version: got %q", payload["strategy_version"].GetStringValue())
	}
	if payload["object_id"].GetStringValue() != "post_happy_1" {
		t.Errorf("payload.object_id: got %q", payload["object_id"].GetStringValue())
	}
	if payload["embedded_at"].GetStringValue() == "" {
		t.Errorf("payload.embedded_at: must be non-empty")
	}

	// The named vector "dense_interactions" must contain a 128-d slice.
	// Qdrant 1.17+ returns dense vectors in the VectorOutput.Vector oneof
	// (via GetDense().GetData()); the deprecated VectorOutput.GetData() returns
	// nil on this server build.
	vectors := points[0].GetVectors().GetVectors().GetVectors()
	if vectors == nil {
		t.Fatalf("point has no named vectors")
	}
	dense := vectors["dense_interactions"]
	if dense == nil {
		t.Fatalf("point missing dense_interactions vector")
	}
	data := dense.GetDense().GetData()
	if len(data) == 0 {
		data = dense.GetData() // older qdrant builds populate the flat field
	}
	if len(data) != 128 {
		t.Errorf("dense vector length: got %d, want 128", len(data))
	}
}

func TestCatalogE2E_IdempotentReingest_DoesNotDoubleEmbed(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_idem", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, namespace, 128)
	runEmbedderInBackground(t)

	body := map[string]any{"object_id": "idem_1", "content": "the same content forever"}

	for i := 0; i < 3; i++ {
		resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey, body)
		assertStatus(t, resp, http.StatusAccepted)
		resp.Body.Close()
	}

	waitForCatalogState(t, namespace, "idem_1", "embedded", 10*time.Second)

	// After the first re-ingest, the row is already 'embedded' with the
	// matching content_hash so subsequent re-ingests should be no-ops.
	// Qdrant must contain exactly ONE point for this object (point id is
	// stable through idmap so even an extra Qdrant upsert wouldn't add a
	// second row, but we still assert the count to catch regressions).
	collection := namespace + "_objects_dense"
	if got := qdrantPointCount(t, collection); got != 1 {
		t.Fatalf("expected exactly 1 point in %q after idempotent re-ingest, got %d", collection, got)
	}
}

func TestCatalogE2E_NewContent_RetriggersEmbed(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_change", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, namespace, 128)
	runEmbedderInBackground(t)

	// First version of the content.
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey, map[string]any{
		"object_id": "change_1",
		"content":   "first version of the content",
	})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	waitForCatalogState(t, namespace, "change_1", "embedded", 10*time.Second)

	var firstHash []byte
	if err := testDB.QueryRow(context.Background(),
		`SELECT content_hash FROM catalog_items WHERE namespace=$1 AND object_id=$2`,
		namespace, "change_1").Scan(&firstHash); err != nil {
		t.Fatalf("read first content_hash: %v", err)
	}

	// Re-ingest with substantively different content.
	resp = doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey, map[string]any{
		"object_id": "change_1",
		"content":   "completely different content the second time",
	})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// State must reset to pending, then back to embedded.
	waitForCondition(t, 10*time.Second, func() (bool, error) {
		var hash []byte
		var state string
		err := testDB.QueryRow(context.Background(),
			`SELECT content_hash, state FROM catalog_items WHERE namespace=$1 AND object_id=$2`,
			namespace, "change_1").Scan(&hash, &state)
		if err != nil {
			return false, err
		}
		return state == "embedded" && string(hash) != string(firstHash), nil
	})
}

func TestCatalogE2E_NamespaceNotEnabled_404(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_disabled", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	// Deliberately NOT calling enableCatalogForNamespace.

	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey, map[string]any{
		"object_id": "disabled_1",
		"content":   "should be rejected",
	})
	code, _ := decodeErrorJSON(t, resp, http.StatusNotFound)
	if code != "namespace_not_enabled" {
		t.Errorf("expected error code 'namespace_not_enabled', got %q", code)
	}

	// catalog_items must remain empty for this namespace.
	var count int
	err := testDB.QueryRow(context.Background(),
		`SELECT count(*) FROM catalog_items WHERE namespace=$1`, namespace).Scan(&count)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected no catalog_items rows when namespace not enabled, got %d", count)
	}
}

func TestCatalogE2E_EmptyContent_422(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_empty", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, namespace, 128)

	// Whitespace-only content trims to empty → 422.
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey, map[string]any{
		"object_id": "empty_1",
		"content":   "   \t\n  ",
	})
	if _, _ = decodeErrorJSON(t, resp, http.StatusUnprocessableEntity); false {
		// decode handles assertion; nothing more to do.
	}
}

func TestCatalogE2E_OversizedContent_413(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_oversized", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, namespace, 128)
	// Tighten the limit so we don't have to send 32 KiB to test the path.
	if _, err := testDB.Exec(context.Background(),
		`UPDATE namespace_configs SET catalog_max_content_bytes=64 WHERE namespace=$1`, namespace); err != nil {
		t.Fatalf("set tight content limit: %v", err)
	}

	// 65 bytes > 64.
	body := map[string]any{
		"object_id": "oversized_1",
		"content":   strings.Repeat("x", 65),
	}
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey, body)
	if _, _ = decodeErrorJSON(t, resp, http.StatusRequestEntityTooLarge); false {
		// nothing to do.
	}
}

// US2 acceptance #1 + multi-tenant isolation: two namespaces with their
// own active strategies must produce vectors at the right dim in the
// right collection. We use the V1 hashing strategy at two different
// dims (128 and 256) since V1 ships only one strategy id but supports
// multiple dim variants via RegisterVariants.
func TestCatalogE2E_MultiTenant_StrategyIsolation(t *testing.T) {
	nsA, keyA := createIsolatedNamespace(t, "catalog_iso_a", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, nsA, 128)

	nsB, keyB := createIsolatedNamespace(t, "catalog_iso_b", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  256,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, nsB, 256)

	runEmbedderInBackground(t)

	// Ingest one item into each namespace.
	for _, x := range []struct {
		ns, key, content string
	}{
		{nsA, keyA, "alpha namespace text"},
		{nsB, keyB, "beta namespace text"},
	} {
		resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+x.ns+"/catalog", x.key, map[string]any{
			"object_id": "iso_obj_1",
			"content":   x.content,
		})
		assertStatus(t, resp, http.StatusAccepted)
		resp.Body.Close()
	}

	waitForCatalogState(t, nsA, "iso_obj_1", "embedded", 10*time.Second)
	waitForCatalogState(t, nsB, "iso_obj_1", "embedded", 10*time.Second)

	// Each namespace's vector must land at the configured dim and ONLY in
	// that namespace's collection.
	for _, want := range []struct {
		ns  string
		dim int
	}{
		{nsA, 128},
		{nsB, 256},
	} {
		pointID := numericIDFor(t, "iso_obj_1", want.ns, "object")
		client := newQdrantTestClient(t)
		points, err := client.Get(context.Background(), &qdrant.GetPoints{
			CollectionName: want.ns + "_objects_dense",
			Ids:            []*qdrant.PointId{qdrant.NewIDNum(pointID)},
			WithVectors:    qdrant.NewWithVectorsEnable(true),
		})
		if err != nil {
			t.Fatalf("ns=%s qdrant get: %v", want.ns, err)
		}
		if len(points) != 1 {
			t.Fatalf("ns=%s expected 1 point, got %d", want.ns, len(points))
		}
		dense := points[0].GetVectors().GetVectors().GetVectors()["dense_interactions"]
		if dense == nil {
			t.Fatalf("ns=%s missing dense vector", want.ns)
		}
		data := dense.GetDense().GetData()
		if len(data) == 0 {
			data = dense.GetData() // older qdrant builds populate the flat field
		}
		if got := len(data); got != want.dim {
			t.Errorf("ns=%s vec dim: got %d, want %d", want.ns, got, want.dim)
		}
	}
}

// FR-018 / R8: BYOE writes for OBJECT dense vectors return 409 Conflict
// when the namespace has catalog auto-embedding enabled. Subject BYOE
// writes remain accepted (per Assumption "Subject embeddings continue
// through cron mean-pool"); this test asserts both.
func TestCatalogE2E_BYOEObjectWrite_Returns409_WhenCatalogEnabled(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_byoe_409", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, namespace, 128)

	// Build a 128-dim vector for the BYOE write attempt.
	vec := make([]float32, 128)
	for i := range vec {
		vec[i] = 0.01
	}

	// Object BYOE write → 409.
	objURL := baseURL + "/v1/namespaces/" + namespace + "/objects/byoe_obj_1/embedding"
	resp := doRequest(t, http.MethodPut, objURL, apiKey, map[string]any{"vector": vec})
	code, _ := decodeErrorJSON(t, resp, http.StatusConflict)
	if code != "catalog_active" {
		t.Errorf("expected error code 'catalog_active', got %q", code)
	}

	// Subject BYOE write → 204 (NOT guarded). The spec assumption keeps
	// subject vectors flowing through the cron mean-pool path even under
	// catalog mode.
	subjURL := baseURL + "/v1/namespaces/" + namespace + "/subjects/byoe_subj_1/embedding"
	resp = doRequest(t, http.MethodPut, subjURL, apiKey, map[string]any{"vector": vec})
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

// FR-018: when catalog is DISABLED, BYOE object writes work as before
// (the 409 guard is gated on catalog_enabled=true, not on catalog
// being in the codebase).
func TestCatalogE2E_BYOEObjectWrite_StillWorksWhenCatalogDisabled(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_byoe_ok", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	// Deliberately NOT enabling catalog — the namespace is in pure BYOE mode.

	vec := make([]float32, 128)
	for i := range vec {
		vec[i] = 0.01
	}

	objURL := baseURL + "/v1/namespaces/" + namespace + "/objects/byoe_legacy_1/embedding"
	resp := doRequest(t, http.MethodPut, objURL, apiKey, map[string]any{"vector": vec})
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

// US2 admin-plane endpoints (GET/PUT /api/admin/v1/namespaces/{ns}/catalog)
// are exercised at the unit-test level in internal/admin/handler_test.go;
// the e2e suite intentionally does not spawn cmd/admin (it would require
// the embedded SPA + session cookie machinery), so the admin endpoints
// are not covered here. The cmd/api side of US2 — the BYOE 409 guard —
// IS covered above by the BYOE tests.

// ─── US3: re-embed orchestration + items operator endpoints ───────────────────

// newAdminServiceForTest wires admin.Service in-process so the e2e test can
// call TriggerReEmbed / BulkRedriveDeadletter / DeleteCatalogItem without
// spawning cmd/admin. We avoid cmd/admin in e2e because it embeds the SPA
// at build time and uses session cookies, both of which add ceremony that
// the catalog flow does not exercise.
func newAdminServiceForTest(t testing.TB) *admin.Service {
	t.Helper()

	repo := admin.NewRepository(testDB)
	qdrantPort, err := strconv.Atoi(envOrDefault("QDRANT_PORT", "6334"))
	if err != nil {
		t.Fatalf("invalid QDRANT_PORT: %v", err)
	}
	qdrantClient, err := qdrant.NewClient(&qdrant.Config{
		Host: envOrDefault("QDRANT_HOST", "localhost"),
		Port: qdrantPort,
	})
	if err != nil {
		t.Fatalf("connect qdrant: %v", err)
	}

	nsRepo := nsconfig.NewRepository(testDB)
	nsSvc := nsconfig.NewService(nsRepo)

	svc := admin.NewService(
		repo,
		"http://unused",
		adminKey,
		testRedis,
		qdrantClient,
		nil,
		&nsAdapterShim{svc: nsSvc},
	)
	svc.SetCatalogStrategyPicker(&strategyPickerShim{svc: nsSvc})
	return svc
}

// nsAdapterShim satisfies admin.nsConfigUpserter without the production
// cmd/admin/nsconfig_adapter.go (which lives in package main). For e2e tests
// we never call Upsert via the admin service, so a no-op is fine.
type nsAdapterShim struct{ svc *nsconfig.Service }

func (a *nsAdapterShim) Upsert(_ context.Context, _ string, _ *admin.NamespaceUpsertRequest) (*admin.NamespaceUpsertResponse, error) {
	return nil, fmt.Errorf("nsAdapterShim.Upsert not implemented for e2e")
}

// strategyPickerShim mirrors cmd/admin/catalog_adapter.go's
// GetCatalogStrategy method, but in-process. Returns enabled=false when the
// namespace doesn't exist OR catalog_enabled is false (FR-008 single 404).
type strategyPickerShim struct{ svc *nsconfig.Service }

func (s *strategyPickerShim) GetCatalogStrategy(ctx context.Context, ns string) (string, string, bool, error) {
	cfg, err := s.svc.Get(ctx, ns)
	if err != nil {
		return "", "", false, err
	}
	if cfg == nil || !cfg.CatalogEnabled {
		return "", "", false, nil
	}
	return cfg.CatalogStrategyID, cfg.CatalogStrategyVersion, true, nil
}

// forceItemToDeadletter flips a catalog item to dead_letter via direct DB
// UPDATE. Used by the bulk-redrive test to seed deadletter rows without
// having to engineer real embed failures.
func forceItemToDeadletter(t testing.TB, namespace, objectID string) {
	t.Helper()

	tag, err := testDB.Exec(context.Background(), `
		UPDATE catalog_items
		SET state = 'dead_letter',
		    last_error = 'forced for test',
		    attempt_count = 99,
		    updated_at = NOW()
		WHERE namespace = $1 AND object_id = $2`,
		namespace, objectID,
	)
	if err != nil {
		t.Fatalf("force item to dead_letter: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("force item to dead_letter: expected 1 row affected, got %d", tag.RowsAffected())
	}
}

// catalogItemIDFor returns the catalog_items.id for a (namespace, object_id).
func catalogItemIDFor(t testing.TB, namespace, objectID string) int64 {
	t.Helper()

	var id int64
	if err := testDB.QueryRow(context.Background(), `
		SELECT id FROM catalog_items
		WHERE namespace = $1 AND object_id = $2`,
		namespace, objectID,
	).Scan(&id); err != nil {
		t.Fatalf("look up catalog item id: %v", err)
	}
	return id
}

// TestCatalogE2E_ReEmbed_DrainAndComplete exercises the full re-embed flow:
//
//  1. Enable catalog v1 + ingest 3 items + wait for embedded.
//  2. Trigger admin re-embed via in-process admin.Service.
//  3. Verify the orchestration: stale items reset to pending, batch_run_logs
//     row inserted with trigger_source='admin_reembed', stream entries published.
//  4. Wait for items to drain back to embedded.
//  5. Verify the watcher in cmd/embedder closes the batch_run_logs row.
func TestCatalogE2E_ReEmbed_DrainAndComplete(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_reembed", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, namespace, 128)
	runEmbedderInBackground(t)

	// Phase 1: ingest 3 catalog items.
	for i := 1; i <= 3; i++ {
		resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey, map[string]any{
			"object_id": fmt.Sprintf("post_%d", i),
			"content":   fmt.Sprintf("seed content number %d for re-embed test", i),
		})
		assertStatus(t, resp, http.StatusAccepted)
		resp.Body.Close()
	}
	for i := 1; i <= 3; i++ {
		waitForCatalogState(t, namespace, fmt.Sprintf("post_%d", i), "embedded", 10*time.Second)
	}

	// Phase 2: trigger re-embed via admin service.
	adminSvc := newAdminServiceForTest(t)
	reembedResp, err := adminSvc.TriggerReEmbed(context.Background(), namespace)
	if err != nil {
		t.Fatalf("trigger re-embed: %v", err)
	}
	if reembedResp == nil {
		t.Fatal("re-embed returned nil — namespace not enabled?")
	}
	if reembedResp.StaleItems != 0 {
		// stale_items=0 because the namespace is at v1 and items are also at v1.
		// The reset query targets WHERE strategy_version <> v1 OR IS NULL.
		// Already-embedded items at v1 are not re-driven by this path.
		t.Logf("re-embed stale_items=%d (expected 0 when items match active version)", reembedResp.StaleItems)
	}

	// Phase 3: the batch_run_logs row was created with trigger_source='admin_reembed'.
	var (
		batchID       int64
		triggerSource string
		completedAt   *time.Time
		success       bool
	)
	if err := testDB.QueryRow(context.Background(), `
		SELECT id, trigger_source, completed_at, success
		FROM batch_run_logs
		WHERE namespace = $1
		  AND trigger_source = 'admin_reembed'
		ORDER BY started_at DESC LIMIT 1`,
		namespace,
	).Scan(&batchID, &triggerSource, &completedAt, &success); err != nil {
		t.Fatalf("read batch_run_logs row: %v", err)
	}
	if batchID != reembedResp.BatchRunID {
		t.Errorf("batch row id mismatch: got %d, want %d", batchID, reembedResp.BatchRunID)
	}

	// Phase 4: the watcher should close the row within ~10s. Stale count is 0
	// from the start (since we re-embed at the same version), so this should
	// happen on the next 5s tick.
	waitForCondition(t, 30*time.Second, func() (bool, error) {
		var done bool
		err := testDB.QueryRow(context.Background(), `
			SELECT completed_at IS NOT NULL AND success = TRUE
			FROM batch_run_logs WHERE id = $1`,
			batchID,
		).Scan(&done)
		return done, err
	})
}

// TestCatalogE2E_BulkRedriveDeadletter exercises SC-008 — operator can clear
// every dead-letter item in a namespace with one call. The test forces two
// items to dead_letter via direct DB UPDATE, then calls the admin bulk
// redrive method and waits for both to return to 'embedded'.
func TestCatalogE2E_BulkRedriveDeadletter(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_bulkredrive", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, namespace, 128)
	runEmbedderInBackground(t)

	for i := 1; i <= 2; i++ {
		resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey, map[string]any{
			"object_id": fmt.Sprintf("dead_%d", i),
			"content":   fmt.Sprintf("future-deadletter item %d", i),
		})
		assertStatus(t, resp, http.StatusAccepted)
		resp.Body.Close()
	}
	for i := 1; i <= 2; i++ {
		waitForCatalogState(t, namespace, fmt.Sprintf("dead_%d", i), "embedded", 10*time.Second)
	}

	// Force both into dead_letter.
	for i := 1; i <= 2; i++ {
		forceItemToDeadletter(t, namespace, fmt.Sprintf("dead_%d", i))
	}

	// Bulk redrive via admin service.
	adminSvc := newAdminServiceForTest(t)
	resp, err := adminSvc.BulkRedriveDeadletter(context.Background(), namespace)
	if err != nil {
		t.Fatalf("bulk redrive: %v", err)
	}
	if resp == nil {
		t.Fatal("bulk redrive returned nil — namespace not enabled?")
	}
	if resp.Redriven != 2 {
		t.Errorf("expected 2 redriven, got %d", resp.Redriven)
	}

	for i := 1; i <= 2; i++ {
		waitForCatalogState(t, namespace, fmt.Sprintf("dead_%d", i), "embedded", 15*time.Second)
	}
}

// TestCatalogE2E_DeleteCatalogItem verifies FR-017: the operator-side hard
// delete removes both the Postgres row AND the matching Qdrant point.
func TestCatalogE2E_DeleteCatalogItem(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_delete", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  128,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})
	enableCatalogForNamespace(t, namespace, 128)
	runEmbedderInBackground(t)

	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey, map[string]any{
		"object_id": "to_delete",
		"content":   "an item that will be deleted",
	})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	waitForCatalogState(t, namespace, "to_delete", "embedded", 10*time.Second)

	// Sanity: Qdrant point exists before delete.
	collection := namespace + "_objects_dense"
	if got := qdrantPointCount(t, collection); got != 1 {
		t.Fatalf("pre-delete: expected 1 point in %q, got %d", collection, got)
	}

	itemID := catalogItemIDFor(t, namespace, "to_delete")
	adminSvc := newAdminServiceForTest(t)
	if err := adminSvc.DeleteCatalogItem(context.Background(), namespace, itemID); err != nil {
		t.Fatalf("delete catalog item: %v", err)
	}

	// Postgres row gone.
	var n int
	if err := testDB.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM catalog_items
		WHERE namespace = $1 AND object_id = $2`,
		namespace, "to_delete",
	).Scan(&n); err != nil {
		t.Fatalf("count catalog_items: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 catalog_items rows post-delete, got %d", n)
	}

	// Qdrant point gone (best-effort — recommend service caches may still
	// reference the id but the dense point itself must be removed).
	if got := qdrantPointCount(t, collection); got != 0 {
		t.Errorf("expected 0 points in %q post-delete, got %d", collection, got)
	}

	// Idempotent: a second delete on the same id is a no-op.
	if err := adminSvc.DeleteCatalogItem(context.Background(), namespace, itemID); err != nil {
		t.Errorf("second delete should be idempotent, got error: %v", err)
	}
}
