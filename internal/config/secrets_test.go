package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSecret(t *testing.T) {
	file := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(file, []byte("private-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SECRET", "")
	t.Setenv("TEST_SECRET_FILE", file)
	got, err := LoadSecret("TEST_SECRET")
	if err != nil || got != "private-value" {
		t.Fatalf("file: %q %v", got, err)
	}
	t.Setenv("TEST_SECRET", "ambiguous")
	if _, err := LoadSecret("TEST_SECRET"); err == nil {
		t.Fatal("ambiguous secret accepted")
	}
	t.Setenv("TEST_SECRET", "")
	t.Setenv("TEST_SECRET_FILE", file+"missing")
	if _, err := LoadSecret("TEST_SECRET"); err == nil {
		t.Fatal("missing secret accepted")
	}
}
func TestLegacyAuthExplicitAndProductionForbidden(t *testing.T) {
	t.Setenv("CODOHUE_ADMIN_API_KEY_FILE", "")
	t.Setenv("CODOHUE_ADMIN_API_KEY", "old-key")
	t.Setenv("CODOHUE_LEGACY_ADMIN_AUTH", "")
	t.Setenv("CODOHUE_ENV", "")
	if _, err := legacyAdminKey(); err == nil {
		t.Fatal("implicit legacy auth accepted")
	}
	t.Setenv("CODOHUE_LEGACY_ADMIN_AUTH", "true")
	if key, err := legacyAdminKey(); err != nil || key != "old-key" {
		t.Fatal(err)
	}
	t.Setenv("CODOHUE_ENV", "production")
	if _, err := legacyAdminKey(); err == nil {
		t.Fatal("production legacy auth accepted")
	}
	t.Setenv("CODOHUE_LEGACY_ADMIN_AUTH", "")
	t.Setenv("CODOHUE_ADMIN_API_KEY", "")
	if key, err := legacyAdminKey(); err != nil || key != "" {
		t.Fatal(err)
	}
}

func TestLoadAdminSecretFiles(t *testing.T) {
	dir := t.TempDir()
	for name, value := range map[string]string{"DATABASE_URL": "postgres://db", "CODOHUE_BOOTSTRAP_PASSWORD": "private-owner-password", "CODOHUE_ADMIN_PROXY_TOKEN": "private-proxy-token"} {
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(name, "")
		t.Setenv(name+"_FILE", file)
	}
	t.Setenv("CODOHUE_ADMIN_API_KEY", "")
	t.Setenv("CODOHUE_ADMIN_API_KEY_FILE", "")
	t.Setenv("CODOHUE_LEGACY_ADMIN_AUTH", "false")
	t.Setenv("CODOHUE_BOOTSTRAP_USERNAME", "owner")
	t.Setenv("CODOHUE_ADMIN_COOKIE_SECURE", "true")
	cfg, err := LoadAdmin()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://db" || cfg.BootstrapPassword != "private-owner-password" || cfg.ProxyToken != "private-proxy-token" || !cfg.SecureCookies || cfg.AdminAPIKey != "" {
		t.Fatal("secret-file configuration did not reach admin")
	}
	t.Setenv("CODOHUE_ADMIN_COOKIE_SECURE", "typo")
	if _, err := LoadAdmin(); err == nil {
		t.Fatal("invalid secure-cookie setting accepted")
	}
}

func TestLoadAPIWithoutGlobalKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("DATABASE_URL_FILE", "")
	t.Setenv("CODOHUE_ADMIN_API_KEY", "")
	t.Setenv("CODOHUE_ADMIN_API_KEY_FILE", "")
	t.Setenv("CODOHUE_LEGACY_ADMIN_AUTH", "false")
	cfg, err := LoadAPI()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdminAPIKey != "" {
		t.Fatal("default global bypass enabled")
	}
}
