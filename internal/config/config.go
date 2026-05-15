package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// loadDotenv loads a .env file from the working directory if present.
// A missing file is expected when running in Docker (env vars are injected
// via env_file) and is silently ignored. Other errors (parse, permission)
// still surface so misconfigured files don't fail open.
func loadDotenv() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		fmt.Printf("warning: failed to load .env: %v\n", err)
	}
}

// AppConfig holds all application configuration loaded from environment variables.
type AppConfig struct {
	DatabaseURL          string
	RedisURL             string
	QdrantHost           string
	QdrantPort           int
	AdminAPIKey          string
	BatchIntervalMinutes int
	LogFormat            string // "json" | "text" (default: "text")
	APIPort              string // HTTP listen port (default: "2001")
}

// LoadAPI reads and validates configuration for the API binary.
// It requires both DATABASE_URL and CODOHUE_ADMIN_API_KEY to be set.
func LoadAPI() (*AppConfig, error) {
	cfg, err := loadBase()
	if err != nil {
		return nil, err
	}

	cfg.AdminAPIKey = getEnv("CODOHUE_ADMIN_API_KEY", "")
	if cfg.AdminAPIKey == "" {
		return nil, fmt.Errorf("CODOHUE_ADMIN_API_KEY is required")
	}

	cfg.APIPort = getEnv("CODOHUE_API_PORT", "2001")
	return cfg, nil
}

// LoadCron reads and validates configuration for the cron binary.
// It requires DATABASE_URL but not CODOHUE_ADMIN_API_KEY.
func LoadCron() (*AppConfig, error) {
	return loadBase()
}

// loadBase loads the config fields shared by all binaries and validates them.
func loadBase() (*AppConfig, error) {
	loadDotenv()

	cfg := &AppConfig{
		DatabaseURL: getEnv("DATABASE_URL", ""),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		QdrantHost:  getEnv("QDRANT_HOST", "localhost"),
		LogFormat:   getEnv("CODOHUE_LOG_FORMAT", "text"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	port, err := strconv.Atoi(getEnv("QDRANT_PORT", "6334"))
	if err != nil {
		return nil, fmt.Errorf("invalid QDRANT_PORT: %w", err)
	}
	cfg.QdrantPort = port

	batchInterval, err := strconv.Atoi(getEnv("CODOHUE_BATCH_INTERVAL_MINUTES", "5"))
	if err != nil {
		return nil, fmt.Errorf("invalid CODOHUE_BATCH_INTERVAL_MINUTES: %w", err)
	}
	cfg.BatchIntervalMinutes = batchInterval

	return cfg, nil
}

// AdminConfig holds configuration for the admin dashboard binary.
type AdminConfig struct {
	DatabaseURL string
	RedisURL    string
	AdminAPIKey string
	APIURL      string // internal URL of cmd/api for proxying
	AdminPort   string // HTTP listen port (default: "2002")
	LogFormat   string // "json" | "text" (default: "text")
	QdrantHost  string
	QdrantPort  int
}

// LoadAdmin reads and validates configuration for the admin binary.
func LoadAdmin() (*AdminConfig, error) {
	loadDotenv()

	cfg := &AdminConfig{
		DatabaseURL: getEnv("DATABASE_URL", ""),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		AdminAPIKey: getEnv("CODOHUE_ADMIN_API_KEY", ""),
		APIURL:      getEnv("CODOHUE_API_URL", "http://localhost:2001"),
		AdminPort:   getEnv("CODOHUE_ADMIN_PORT", "2002"),
		LogFormat:   getEnv("CODOHUE_LOG_FORMAT", "text"),
		QdrantHost:  getEnv("QDRANT_HOST", "localhost"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.AdminAPIKey == "" {
		return nil, fmt.Errorf("CODOHUE_ADMIN_API_KEY is required")
	}

	qdrantPort, err := strconv.Atoi(getEnv("QDRANT_PORT", "6334"))
	if err != nil {
		return nil, fmt.Errorf("invalid QDRANT_PORT: %w", err)
	}
	cfg.QdrantPort = qdrantPort

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// EmbedderConfig holds configuration for the embedder binary (cmd/embedder).
type EmbedderConfig struct {
	DatabaseURL string
	RedisURL    string
	QdrantHost  string
	QdrantPort  int
	LogFormat   string

	// CatalogMaxContentBytes is the global default per-namespace cap on the
	// size of catalog item content. Per-namespace overrides live in
	// namespace_configs.catalog_max_content_bytes.
	CatalogMaxContentBytes int

	// EmbedMaxAttempts is the global default for the number of transient
	// retries before an item is moved to dead_letter. Overridable per
	// namespace via namespace_configs.catalog_max_attempts.
	EmbedMaxAttempts int

	// HealthPort is the listen port for the embedder's /healthz and /metrics
	// endpoints (default "2003").
	HealthPort string

	// ReplicaName is the consumer name used when joining the Redis Streams
	// consumer group. Empty string means "use the hostname at runtime".
	ReplicaName string

	// NamespacePollInterval is how often the embedder polls
	// namespace_configs for newly-enabled namespaces.
	NamespacePollInterval time.Duration
}

// LoadEmbedder reads and validates configuration for the embedder binary.
// It requires DATABASE_URL but not CODOHUE_ADMIN_API_KEY (the embedder is a
// background worker that does not expose authenticated endpoints).
func LoadEmbedder() (*EmbedderConfig, error) {
	loadDotenv()

	cfg := &EmbedderConfig{
		DatabaseURL: getEnv("DATABASE_URL", ""),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379"),
		QdrantHost:  getEnv("QDRANT_HOST", "localhost"),
		LogFormat:   getEnv("CODOHUE_LOG_FORMAT", "text"),
		HealthPort:  getEnv("CODOHUE_EMBEDDER_HEALTH_PORT", "2003"),
		ReplicaName: getEnv("CODOHUE_EMBEDDER_REPLICA_NAME", ""),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	qdrantPort, err := strconv.Atoi(getEnv("QDRANT_PORT", "6334"))
	if err != nil {
		return nil, fmt.Errorf("invalid QDRANT_PORT: %w", err)
	}
	cfg.QdrantPort = qdrantPort

	maxBytes, err := strconv.Atoi(getEnv("CODOHUE_CATALOG_MAX_CONTENT_BYTES", "32768"))
	if err != nil {
		return nil, fmt.Errorf("invalid CODOHUE_CATALOG_MAX_CONTENT_BYTES: %w", err)
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("CODOHUE_CATALOG_MAX_CONTENT_BYTES must be positive, got %d", maxBytes)
	}
	cfg.CatalogMaxContentBytes = maxBytes

	maxAttempts, err := strconv.Atoi(getEnv("CODOHUE_EMBED_MAX_ATTEMPTS", "5"))
	if err != nil {
		return nil, fmt.Errorf("invalid CODOHUE_EMBED_MAX_ATTEMPTS: %w", err)
	}
	if maxAttempts <= 0 {
		return nil, fmt.Errorf("CODOHUE_EMBED_MAX_ATTEMPTS must be positive, got %d", maxAttempts)
	}
	cfg.EmbedMaxAttempts = maxAttempts

	pollInterval, err := time.ParseDuration(getEnv("CODOHUE_EMBEDDER_POLL_INTERVAL", "30s"))
	if err != nil {
		return nil, fmt.Errorf("invalid CODOHUE_EMBEDDER_POLL_INTERVAL: %w", err)
	}
	if pollInterval <= 0 {
		return nil, fmt.Errorf("CODOHUE_EMBEDDER_POLL_INTERVAL must be positive, got %s", pollInterval)
	}
	cfg.NamespacePollInterval = pollInterval

	return cfg, nil
}
