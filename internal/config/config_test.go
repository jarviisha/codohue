package config

import "testing"

func TestLoadAPI_RequiresDatabaseURL(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":        "",
		"CODOHUE_ADMIN_API_KEY": "admin",
	}, func() {
		_, err := LoadAPI()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoadAPI_RequiresAdminAPIKey(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":        "postgres://db",
		"CODOHUE_ADMIN_API_KEY": "",
	}, func() {
		_, err := LoadAPI()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoadAPI_InvalidQdrantPort(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":        "postgres://db",
		"CODOHUE_ADMIN_API_KEY": "admin",
		"QDRANT_PORT":         "not-a-number",
	}, func() {
		_, err := LoadAPI()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoadAPI_InvalidBatchInterval(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":           "postgres://db",
		"CODOHUE_ADMIN_API_KEY":    "admin",
		"BATCH_INTERVAL_MINUTES": "not-a-number",
	}, func() {
		_, err := LoadAPI()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoadAPI_UsesDefaults(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":           "postgres://db",
		"CODOHUE_ADMIN_API_KEY":    "admin",
		"REDIS_URL":              "",
		"QDRANT_HOST":            "",
		"QDRANT_PORT":            "",
		"BATCH_INTERVAL_MINUTES": "",
		"LOG_FORMAT":             "",
		"API_PORT":               "",
	}, func() {
		cfg, err := LoadAPI()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RedisURL != "redis://localhost:6379" {
			t.Fatalf("RedisURL: got %q", cfg.RedisURL)
		}
		if cfg.QdrantHost != "localhost" {
			t.Fatalf("QdrantHost: got %q", cfg.QdrantHost)
		}
		if cfg.QdrantPort != 6334 {
			t.Fatalf("QdrantPort: got %d", cfg.QdrantPort)
		}
		if cfg.BatchIntervalMinutes != 5 {
			t.Fatalf("BatchIntervalMinutes: got %d", cfg.BatchIntervalMinutes)
		}
		if cfg.LogFormat != "text" {
			t.Fatalf("LogFormat: got %q", cfg.LogFormat)
		}
		if cfg.APIPort != "2001" {
			t.Fatalf("APIPort: got %q", cfg.APIPort)
		}
	})
}

func TestLoadAPI_UsesEnvironmentOverrides(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":           "postgres://custom-db",
		"CODOHUE_ADMIN_API_KEY":    "custom-admin",
		"REDIS_URL":              "redis://custom:6379",
		"QDRANT_HOST":            "qdrant.internal",
		"QDRANT_PORT":            "7000",
		"BATCH_INTERVAL_MINUTES": "15",
		"LOG_FORMAT":             "json",
		"API_PORT":               "8080",
	}, func() {
		cfg, err := LoadAPI()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.DatabaseURL != "postgres://custom-db" {
			t.Fatalf("DatabaseURL: got %q", cfg.DatabaseURL)
		}
		if cfg.AdminAPIKey != "custom-admin" {
			t.Fatalf("AdminAPIKey: got %q", cfg.AdminAPIKey)
		}
		if cfg.RedisURL != "redis://custom:6379" {
			t.Fatalf("RedisURL: got %q", cfg.RedisURL)
		}
		if cfg.QdrantHost != "qdrant.internal" {
			t.Fatalf("QdrantHost: got %q", cfg.QdrantHost)
		}
		if cfg.QdrantPort != 7000 {
			t.Fatalf("QdrantPort: got %d", cfg.QdrantPort)
		}
		if cfg.BatchIntervalMinutes != 15 {
			t.Fatalf("BatchIntervalMinutes: got %d", cfg.BatchIntervalMinutes)
		}
		if cfg.LogFormat != "json" {
			t.Fatalf("LogFormat: got %q", cfg.LogFormat)
		}
		if cfg.APIPort != "8080" {
			t.Fatalf("APIPort: got %q", cfg.APIPort)
		}
	})
}

func TestLoadCron_RequiresDatabaseURL(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL": "",
	}, func() {
		_, err := LoadCron()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoadCron_DoesNotRequireAdminAPIKey(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":        "postgres://db",
		"CODOHUE_ADMIN_API_KEY": "",
	}, func() {
		_, err := LoadCron()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestLoadCron_InvalidQdrantPort(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL": "postgres://db",
		"QDRANT_PORT":  "not-a-number",
	}, func() {
		_, err := LoadCron()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoadCron_InvalidBatchInterval(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":           "postgres://db",
		"BATCH_INTERVAL_MINUTES": "not-a-number",
	}, func() {
		_, err := LoadCron()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoadCron_UsesDefaults(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":           "postgres://db",
		"REDIS_URL":              "",
		"QDRANT_HOST":            "",
		"QDRANT_PORT":            "",
		"BATCH_INTERVAL_MINUTES": "",
		"LOG_FORMAT":             "",
	}, func() {
		cfg, err := LoadCron()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RedisURL != "redis://localhost:6379" {
			t.Fatalf("RedisURL: got %q", cfg.RedisURL)
		}
		if cfg.QdrantHost != "localhost" {
			t.Fatalf("QdrantHost: got %q", cfg.QdrantHost)
		}
		if cfg.QdrantPort != 6334 {
			t.Fatalf("QdrantPort: got %d", cfg.QdrantPort)
		}
		if cfg.BatchIntervalMinutes != 5 {
			t.Fatalf("BatchIntervalMinutes: got %d", cfg.BatchIntervalMinutes)
		}
		if cfg.LogFormat != "text" {
			t.Fatalf("LogFormat: got %q", cfg.LogFormat)
		}
		if cfg.AdminAPIKey != "" {
			t.Fatalf("AdminAPIKey should be empty, got %q", cfg.AdminAPIKey)
		}
		if cfg.APIPort != "" {
			t.Fatalf("APIPort should be empty, got %q", cfg.APIPort)
		}
	})
}

func TestGetEnv_Fallback(t *testing.T) {
	withEnv(t, map[string]string{"SOME_TEST_ENV": ""}, func() {
		if got := getEnv("SOME_TEST_ENV", "fallback"); got != "fallback" {
			t.Fatalf("getEnv fallback: got %q", got)
		}
	})
}

func withEnv(t *testing.T, values map[string]string, fn func()) {
	t.Helper()
	for key, value := range values {
		t.Setenv(key, value)
	}
	fn()
}

func TestLoadEmbedder_RequiresDatabaseURL(t *testing.T) {
	withEnv(t, map[string]string{"DATABASE_URL": ""}, func() {
		_, err := LoadEmbedder()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoadEmbedder_UsesDefaults(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":                     "postgres://db",
		"REDIS_URL":                        "",
		"QDRANT_HOST":                      "",
		"QDRANT_PORT":                      "",
		"LOG_FORMAT":                       "",
		"CATALOG_MAX_CONTENT_BYTES":        "",
		"EMBED_MAX_ATTEMPTS":               "",
		"EMBEDDER_HEALTH_PORT":             "",
		"EMBEDDER_REPLICA_NAME":            "",
		"EMBEDDER_NAMESPACE_POLL_INTERVAL": "",
	}, func() {
		cfg, err := LoadEmbedder()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RedisURL != "redis://localhost:6379" {
			t.Fatalf("RedisURL: got %q", cfg.RedisURL)
		}
		if cfg.QdrantHost != "localhost" {
			t.Fatalf("QdrantHost: got %q", cfg.QdrantHost)
		}
		if cfg.QdrantPort != 6334 {
			t.Fatalf("QdrantPort: got %d", cfg.QdrantPort)
		}
		if cfg.LogFormat != "text" {
			t.Fatalf("LogFormat: got %q", cfg.LogFormat)
		}
		if cfg.CatalogMaxContentBytes != 32768 {
			t.Fatalf("CatalogMaxContentBytes: got %d", cfg.CatalogMaxContentBytes)
		}
		if cfg.EmbedMaxAttempts != 5 {
			t.Fatalf("EmbedMaxAttempts: got %d", cfg.EmbedMaxAttempts)
		}
		if cfg.HealthPort != "2003" {
			t.Fatalf("HealthPort: got %q", cfg.HealthPort)
		}
		if cfg.NamespacePollInterval.String() != "30s" {
			t.Fatalf("NamespacePollInterval: got %s", cfg.NamespacePollInterval)
		}
	})
}

func TestLoadEmbedder_UsesEnvironmentOverrides(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":                     "postgres://custom-db",
		"REDIS_URL":                        "redis://custom:6379",
		"QDRANT_HOST":                      "qdrant.internal",
		"QDRANT_PORT":                      "7000",
		"LOG_FORMAT":                       "json",
		"CATALOG_MAX_CONTENT_BYTES":        "65536",
		"EMBED_MAX_ATTEMPTS":               "10",
		"EMBEDDER_HEALTH_PORT":             "9003",
		"EMBEDDER_REPLICA_NAME":            "embedder-1",
		"EMBEDDER_NAMESPACE_POLL_INTERVAL": "1m",
	}, func() {
		cfg, err := LoadEmbedder()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.DatabaseURL != "postgres://custom-db" {
			t.Fatalf("DatabaseURL: got %q", cfg.DatabaseURL)
		}
		if cfg.CatalogMaxContentBytes != 65536 {
			t.Fatalf("CatalogMaxContentBytes: got %d", cfg.CatalogMaxContentBytes)
		}
		if cfg.EmbedMaxAttempts != 10 {
			t.Fatalf("EmbedMaxAttempts: got %d", cfg.EmbedMaxAttempts)
		}
		if cfg.HealthPort != "9003" {
			t.Fatalf("HealthPort: got %q", cfg.HealthPort)
		}
		if cfg.ReplicaName != "embedder-1" {
			t.Fatalf("ReplicaName: got %q", cfg.ReplicaName)
		}
		if cfg.NamespacePollInterval.String() != "1m0s" {
			t.Fatalf("NamespacePollInterval: got %s", cfg.NamespacePollInterval)
		}
	})
}

func TestLoadEmbedder_InvalidQdrantPort(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL": "postgres://db",
		"QDRANT_PORT":  "not-a-number",
	}, func() {
		_, err := LoadEmbedder()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoadEmbedder_InvalidMaxContentBytes(t *testing.T) {
	cases := []string{"not-a-number", "0", "-1"}
	for _, v := range cases {
		v := v
		t.Run(v, func(t *testing.T) {
			withEnv(t, map[string]string{
				"DATABASE_URL":              "postgres://db",
				"CATALOG_MAX_CONTENT_BYTES": v,
			}, func() {
				_, err := LoadEmbedder()
				if err == nil {
					t.Fatalf("expected error for value %q, got nil", v)
				}
			})
		})
	}
}

func TestLoadEmbedder_InvalidMaxAttempts(t *testing.T) {
	cases := []string{"not-a-number", "0", "-3"}
	for _, v := range cases {
		v := v
		t.Run(v, func(t *testing.T) {
			withEnv(t, map[string]string{
				"DATABASE_URL":       "postgres://db",
				"EMBED_MAX_ATTEMPTS": v,
			}, func() {
				_, err := LoadEmbedder()
				if err == nil {
					t.Fatalf("expected error for value %q, got nil", v)
				}
			})
		})
	}
}

func TestLoadEmbedder_InvalidPollInterval(t *testing.T) {
	cases := []string{"not-a-duration", "0s", "-5s"}
	for _, v := range cases {
		v := v
		t.Run(v, func(t *testing.T) {
			withEnv(t, map[string]string{
				"DATABASE_URL":                     "postgres://db",
				"EMBEDDER_NAMESPACE_POLL_INTERVAL": v,
			}, func() {
				_, err := LoadEmbedder()
				if err == nil {
					t.Fatalf("expected error for value %q, got nil", v)
				}
			})
		})
	}
}
