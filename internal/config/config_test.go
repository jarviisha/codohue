package config

import "testing"

func TestLoad_RequiresDatabaseURL(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":        "",
		"RECOMMENDER_API_KEY": "admin",
	}, func() {
		_, err := Load()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoad_RequiresRecommenderAPIKey(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":        "postgres://db",
		"RECOMMENDER_API_KEY": "",
	}, func() {
		_, err := Load()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoad_InvalidQdrantPort(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":        "postgres://db",
		"RECOMMENDER_API_KEY": "admin",
		"QDRANT_PORT":         "not-a-number",
	}, func() {
		_, err := Load()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoad_InvalidBatchInterval(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":           "postgres://db",
		"RECOMMENDER_API_KEY":    "admin",
		"BATCH_INTERVAL_MINUTES": "not-a-number",
	}, func() {
		_, err := Load()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestLoad_UsesDefaults(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":           "postgres://db",
		"RECOMMENDER_API_KEY":    "admin",
		"REDIS_URL":              "",
		"QDRANT_HOST":            "",
		"QDRANT_PORT":            "",
		"BATCH_INTERVAL_MINUTES": "",
		"LOG_FORMAT":             "",
		"API_PORT":               "",
	}, func() {
		cfg, err := Load()
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

func TestLoad_UsesEnvironmentOverrides(t *testing.T) {
	withEnv(t, map[string]string{
		"DATABASE_URL":           "postgres://custom-db",
		"RECOMMENDER_API_KEY":    "custom-admin",
		"REDIS_URL":              "redis://custom:6379",
		"QDRANT_HOST":            "qdrant.internal",
		"QDRANT_PORT":            "7000",
		"BATCH_INTERVAL_MINUTES": "15",
		"LOG_FORMAT":             "json",
		"API_PORT":               "8080",
	}, func() {
		cfg, err := Load()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.DatabaseURL != "postgres://custom-db" {
			t.Fatalf("DatabaseURL: got %q", cfg.DatabaseURL)
		}
		if cfg.RecommenderAPIKey != "custom-admin" {
			t.Fatalf("RecommenderAPIKey: got %q", cfg.RecommenderAPIKey)
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
