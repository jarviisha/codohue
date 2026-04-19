package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

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

// Load reads and validates configuration from environment variables.
// If a .env file is present it is loaded first; variables already set in the
// environment take precedence (godotenv does not overwrite existing values).
func Load() (*AppConfig, error) {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("No .env file found, relying on environment variables")
	}
	cfg := &AppConfig{
		DatabaseURL:       getEnv("DATABASE_URL", ""),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379"),
		QdrantHost:        getEnv("QDRANT_HOST", "localhost"),
		RecommenderAPIKey: getEnv("RECOMMENDER_API_KEY", ""),
		LogFormat:         getEnv("LOG_FORMAT", "text"),
		APIPort:           getEnv("API_PORT", "2001"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RecommenderAPIKey == "" {
		return nil, fmt.Errorf("RECOMMENDER_API_KEY is required")
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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
