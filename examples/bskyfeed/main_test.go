package main

import "testing"

func TestBootstrapRequiresExplicitAuthority(t *testing.T) {
	t.Setenv("CODOHUE_ADMIN_API_KEY", "")
	t.Setenv("CODOHUE_BSKY_BOOTSTRAP", "")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.bootstrap || cfg.adminKey != "" {
		t.Fatal("feeder defaults grant administrative authority")
	}
	t.Setenv("CODOHUE_BSKY_BOOTSTRAP", "true")
	if _, err := loadConfig(); err == nil {
		t.Fatal("bootstrap without explicit token accepted")
	}
}
