package config

import (
	"fmt"
	"os"
	"strings"
)

// LoadSecret resolves a value or its Docker secret file. Ambiguous configuration is rejected.
func LoadSecret(name string) (string, error) {
	value, file := os.Getenv(name), os.Getenv(name+"_FILE")
	if value != "" && file != "" {
		return "", fmt.Errorf("set only one of %s and %s_FILE", name, name)
	}
	if file == "" {
		return value, nil
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", name, err)
	}
	value = strings.TrimRight(string(b), "\r\n")
	if value == "" {
		return "", fmt.Errorf("%s_FILE is empty", name)
	}
	return value, nil
}

func legacyAdminKey() (string, error) {
	key, err := LoadSecret("CODOHUE_ADMIN_API_KEY")
	if err != nil {
		return "", err
	}
	legacy := os.Getenv("CODOHUE_LEGACY_ADMIN_AUTH") == "true"
	if os.Getenv("CODOHUE_ENV") == "production" && (legacy || key == "dev-secret-key") {
		return "", fmt.Errorf("production forbids legacy admin authentication and development credentials")
	}
	if key != "" && !legacy {
		return "", fmt.Errorf("CODOHUE_ADMIN_API_KEY requires explicit CODOHUE_LEGACY_ADMIN_AUTH=true during migration; see deploy/operator-auth.md")
	}
	if legacy && key == "" {
		return "", fmt.Errorf("legacy authentication requires CODOHUE_ADMIN_API_KEY")
	}
	if !legacy {
		return "", nil
	}
	return key, nil
}

func validateSecretFiles() error {
	for _, name := range []string{"DATABASE_URL", "REDIS_URL", "CODOHUE_ADMIN_API_KEY", "CODOHUE_ADMIN_PROXY_TOKEN", "CODOHUE_BOOTSTRAP_PASSWORD", "CODOHUE_OBSERVABILITY_TOKEN"} {
		if _, err := LoadSecret(name); err != nil {
			return err
		}
	}
	return nil
}
