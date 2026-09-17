//go:build e2e

package e2e

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/jarviisha/codohue/internal/auth"
	"github.com/jarviisha/codohue/internal/core/access"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

func TestOperatorSessionsAndRevocation(t *testing.T) {
	ctx := context.Background()
	a, b := access.NewStore(testDB), access.NewStore(testDB)
	username := "e2e_identity_owner"
	if err := a.SetAccount(ctx, "test", username, "test-owner-password", "owner", false, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testDB.Exec(ctx, `DELETE FROM admin_sessions WHERE username=$1`, username)
		_, _ = testDB.Exec(ctx, `DELETE FROM admin_accounts WHERE username=$1`, username)
	})
	actor, err := a.Login(ctx, "replica-login", username, "test-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := a.IssueSession(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := b.Session(ctx, token); err != nil || got.Name != username {
		t.Fatalf("replica cannot resolve actor: %+v %v", got, err)
	}
	if err := b.RevokeSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Session(ctx, token); !errors.Is(err, access.ErrCredentials) {
		t.Fatal("logout did not revoke across stores")
	}
	token, _, err = a.IssueSession(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetAccount(ctx, "test", username, "changed-owner-password", "owner", false, false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Session(ctx, token); !errors.Is(err, access.ErrCredentials) {
		t.Fatal("password reset did not revoke session")
	}
	if _, _, err := a.IssueSession(ctx, actor); !errors.Is(err, access.ErrCredentials) {
		t.Fatal("login/reset race created stale session")
	}
	actor, err = a.Login(ctx, "replica-login", username, "changed-owner-password")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err = a.IssueSession(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetAccount(ctx, "test", username, "", "admin", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Session(ctx, token); !errors.Is(err, access.ErrCredentials) {
		t.Fatal("disabled account kept session")
	}
}

func TestOperatorSharedThrottle(t *testing.T) {
	ctx := context.Background()
	s := access.NewStore(testDB)
	username := "e2e_throttle"
	if err := s.SetAccount(ctx, "test", username, "correct-test-password", "admin", false, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testDB.Exec(ctx, `DELETE FROM admin_accounts WHERE username=$1`, username) })
	ip := "shared-test-ip"
	_, _ = testDB.Exec(ctx, `DELETE FROM admin_login_buckets WHERE bucket=$1`, access.Digest(ip))
	var wg sync.WaitGroup
	errs := make(chan error, 5)
	for range 5 {
		wg.Go(func() { _, err := access.NewStore(testDB).Login(ctx, ip, username, "wrong"); errs <- err })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, access.ErrCredentials) {
			t.Fatalf("first attempts: %v", err)
		}
	}
	if _, err := s.Login(ctx, ip, username, "correct-test-password"); !errors.Is(err, access.ErrThrottled) {
		t.Fatalf("matching guess bypassed shared throttle: %v", err)
	}
}

func TestServiceTokenIsolationAndRevocation(t *testing.T) {
	ctx := context.Background()
	s := access.NewStore(testDB)
	token, err := access.RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	name := "e2e_consumer"
	t.Cleanup(func() { _, _ = testDB.Exec(ctx, `DELETE FROM admin_service_tokens WHERE name=$1`, name) })
	if err := s.ProvisionToken(ctx, "test", name, token, []string{"data:read"}, []string{testNS}); err != nil {
		t.Fatal(err)
	}
	if err := s.ProvisionToken(ctx, "test", name, token, []string{"data:read"}, []string{testNS}); err != nil {
		t.Fatal("retry failed", err)
	}
	if err := s.ProvisionToken(ctx, "test", name, strings.Repeat("ab", 32), []string{"data:read"}, []string{testNS}); !errors.Is(err, access.ErrConflict) {
		t.Fatal("conflicting token overwrote credential", err)
	}
	if err := s.ProvisionToken(ctx, "test", name+"_duplicate", token, []string{"data:read"}, []string{testNS}); !errors.Is(err, access.ErrConflict) {
		t.Fatal("duplicate credential did not report conflict", err)
	}
	resp := doRequest(t, "GET", baseURL+"/v1/namespaces/"+testNS+"/trending", token, nil)
	assertStatus(t, resp, 200)
	resp.Body.Close()
	resp = doRequest(t, "GET", baseURL+"/v1/namespaces/other/trending", token, nil)
	assertStatus(t, resp, 403)
	resp.Body.Close()
	resp = doRequest(t, "POST", baseURL+"/v1/namespaces/"+testNS+"/events", token, map[string]any{})
	assertStatus(t, resp, 403)
	resp.Body.Close()
	ensureAdminServer(t)
	resp = doRequest(t, "GET", adminBaseURL+"/api/admin/v1/health", token, nil)
	assertStatus(t, resp, 403)
	resp.Body.Close()
	resp = doRequest(t, "POST", adminBaseURL+"/api/admin/v1/reset", token, map[string]any{})
	assertStatus(t, resp, 403)
	resp.Body.Close()
	if err := s.RevokeToken(ctx, "test", name); err != nil {
		t.Fatal(err)
	}
	if _, err := access.NewStore(testDB).ServiceToken(ctx, token); !errors.Is(err, access.ErrCredentials) {
		t.Fatal("revocation failed")
	}
	if err := s.ProvisionToken(ctx, "test", name, token, []string{"data:read"}, []string{testNS}); !errors.Is(err, access.ErrConflict) {
		t.Fatal("restart resurrected revoked token")
	}
}

func TestNamespaceProvisionConcurrentAndRetry(t *testing.T) {
	ctx := context.Background()
	ns := "e2e_provision"
	cleanupNamespaceData(ns)
	t.Cleanup(func() { cleanupNamespaceData(ns) })
	svc := nsconfig.NewService(nsconfig.NewRepository(testDB))
	key, err := access.RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	req := &nsconfig.UpsertRequest{ProvisionAPIKey: key}
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Go(func() {
			response, err := svc.Upsert(ctx, ns, req)
			if err == nil && response.APIKey != "" {
				err = errors.New("provision echoed plaintext key")
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := svc.Get(ctx, ns)
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(context.Context, string) (string, error) { return cfg.APIKeyHash, nil }
	if !auth.ValidateNamespaceKey(ctx, key, "", lookup, ns) {
		t.Fatal("pre-generated key unusable")
	}
	if auth.ValidateNamespaceKey(ctx, adminKey, "", lookup, ns) {
		t.Fatal("global key bypass exists without compatibility mode")
	}
	before := cfg.UpdatedAt
	changed := *req
	changed.ProvisionAPIKey = strings.Repeat("cd", 32)
	if _, err := svc.Upsert(ctx, ns, &changed); !errors.Is(err, nsconfig.ErrProvisionConflict) {
		t.Fatal("conflict did not fail", err)
	}
	if _, err := svc.Upsert(ctx, ns, req); err != nil {
		t.Fatal("lost-response retry failed", err)
	}
	cfg, err = svc.Get(ctx, ns)
	if err != nil || !cfg.UpdatedAt.Equal(before) {
		t.Fatal("retry changed configuration", err)
	}
}

func TestOperatorHTTPIdentityAndCSRF(t *testing.T) {
	cookie := adminLogin(t)
	response := adminRequest(t, "GET", "/api/v1/auth/sessions/current", cookie, nil)
	assertStatus(t, response, 200)
	response.Body.Close()
	req := newJSONRequest(t, "POST", adminBaseURL+"/api/admin/v1/reset", nil)
	req.AddCookie(cookie)
	req.Header.Del("X-Codohue-CSRF")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp, 403)
	resp.Body.Close()
	response = adminRequest(t, "DELETE", "/api/v1/auth/sessions/current", cookie, nil)
	assertStatus(t, response, 204)
	response.Body.Close()
	response = adminRequest(t, "GET", "/api/v1/auth/sessions/current", cookie, nil)
	assertStatus(t, response, 401)
	response.Body.Close()
}

func TestOperatorBootstrapConcurrentAndRecovery(t *testing.T) {
	ctx := context.Background()
	// A private schema gives bootstrap a truly empty account store while the suite remains active.
	if _, err := testDB.Exec(ctx, `CREATE SCHEMA e2e_bootstrap`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testDB.Exec(ctx, `DROP SCHEMA e2e_bootstrap CASCADE`) })
	for _, table := range []string{"admin_accounts", "admin_sessions", "admin_audit"} {
		if _, err := testDB.Exec(ctx, `CREATE TABLE e2e_bootstrap.`+table+` (LIKE public.`+table+` INCLUDING ALL)`); err != nil {
			t.Fatal(err)
		}
	}
	cfg := testDB.Config()
	cfg.ConnConfig.RuntimeParams["search_path"] = "e2e_bootstrap"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := access.NewStore(db)
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for range 4 {
		wg.Go(func() { errs <- s.Bootstrap(ctx, "owner", "initial-owner-password") })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Bootstrap(ctx, "owner", "new-startup-password"); err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := db.QueryRow(ctx, `SELECT password_hash FROM admin_accounts WHERE username='owner'`).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("initial-owner-password")) != nil {
		t.Fatal("restart changed password")
	}
	if err := s.SetAccount(ctx, "test", "owner", "", "admin", true, false); !errors.Is(err, access.ErrConflict) {
		t.Fatal("last owner disabled", err)
	}
	if err := s.SetAccount(ctx, "local-recovery", "owner", "recovered-owner-password", "owner", false, true); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT password_hash FROM admin_accounts WHERE username='owner'`).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("recovered-owner-password")) != nil {
		t.Fatal("explicit recovery failed")
	}
}

func TestOperatorCLIProvisioning(t *testing.T) {
	ns := "e2e_cli_provision"
	cleanupNamespaceData(ns)
	t.Cleanup(func() { cleanupNamespaceData(ns) })
	secret, err := access.RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "application-token")
	if err := os.WriteFile(file, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"access", "provision", "--name", ns, "--secret-file", file}
	for range 2 {
		command := exec.CommandContext(t.Context(), adminBin, args...)
		command.Env = append(os.Environ(), "DATABASE_URL="+envOrDefault("DATABASE_URL", ""))
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("provision command: %v: %s", err, output)
		}
		if len(output) != 0 {
			t.Fatal("provision unexpectedly wrote output")
		}
	}
	changed, err := access.RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(changed), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), adminBin, args...)
	command.Env = append(os.Environ(), "DATABASE_URL="+envOrDefault("DATABASE_URL", ""))
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("CLI accepted conflicting key")
	}
	if strings.Contains(string(output), secret) || strings.Contains(string(output), changed) {
		t.Fatal("CLI leaked a credential")
	}
}
