package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"

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
	RecommenderAPIKey    string
	BatchIntervalMinutes int
	LogFormat            string // "json" | "text" (default: "text")
	APIPort              string // HTTP listen port (default: "2001")
}

// LoadAPI reads and validates configuration for the API binary.
// It requires both DATABASE_URL and RECOMMENDER_API_KEY to be set.
func LoadAPI() (*AppConfig, error) {
	cfg, err := loadBase()
	if err != nil {
		return nil, err
	}

	cfg.RecommenderAPIKey = getEnv("RECOMMENDER_API_KEY", "")
	if cfg.RecommenderAPIKey == "" {
		return nil, fmt.Errorf("RECOMMENDER_API_KEY is required")
	}

	cfg.APIPort = getEnv("API_PORT", "2001")
	return cfg, nil
}

// LoadCron reads and validates configuration for the cron binary.
// It requires DATABASE_URL but not RECOMMENDER_API_KEY.
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
		LogFormat:   getEnv("LOG_FORMAT", "text"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	port, err := strconv.Atoi(getEnv("QDRANT_PORT", "6334"))
	if err != nil {
		return nil, fmt.Errorf("invalid QDRANT_PORT: %w", err)
	}
	cfg.QdrantPort = port

	batchInterval, err := strconv.Atoi(getEnv("BATCH_INTERVAL_MINUTES", "5"))
	if err != nil {
		return nil, fmt.Errorf("invalid BATCH_INTERVAL_MINUTES: %w", err)
	}
	cfg.BatchIntervalMinutes = batchInterval

	return cfg, nil
}

// AdminConfig holds configuration for the admin dashboard binary.
type AdminConfig struct {
	DatabaseURL       string
	RedisURL          string
	RecommenderAPIKey string
	APIURL            string // internal URL of cmd/api for proxying
	AdminPort         string // HTTP listen port (default: "2002")
	LogFormat         string // "json" | "text" (default: "text")
	QdrantHost        string
	QdrantPort        int
}

// LoadAdmin reads and validates configuration for the admin binary.
func LoadAdmin() (*AdminConfig, error) {
	loadDotenv()

	cfg := &AdminConfig{
		DatabaseURL:       getEnv("DATABASE_URL", ""),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379"),
		RecommenderAPIKey: getEnv("RECOMMENDER_API_KEY", ""),
		APIURL:            getEnv("API_URL", "http://localhost:2001"),
		AdminPort:         getEnv("ADMIN_PORT", "2002"),
		LogFormat:         getEnv("LOG_FORMAT", "text"),
		QdrantHost:        getEnv("QDRANT_HOST", "localhost"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RecommenderAPIKey == "" {
		return nil, fmt.Errorf("RECOMMENDER_API_KEY is required")
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
