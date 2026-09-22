package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeSettingsNeverExposeSecrets(t *testing.T) {
	secret := "do-not-expose-this-value"
	app := &AppConfig{DatabaseURL: secret, RedisURL: secret, AdminAPIKey: secret, ObservabilityToken: secret, BatchIntervalMinutes: 5}
	admin := &AdminConfig{DatabaseURL: secret, RedisURL: secret, APIURL: secret, ProxyToken: secret, BootstrapPassword: secret, ObservabilityToken: secret, AdminAPIKey: secret}
	embedder := &EmbedderConfig{DatabaseURL: secret, RedisURL: secret, ObservabilityToken: secret}
	for _, settings := range [][]RuntimeSetting{app.RuntimeSettings("api"), app.RuntimeSettings("cron"), admin.RuntimeSettings(), embedder.RuntimeSettings()} {
		body, err := json.Marshal(settings)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), secret) {
			t.Fatal("runtime report exposed a credential or connection string")
		}
	}
	settings := app.RuntimeSettings("cron")
	found := false
	for _, setting := range settings {
		if setting.Name == "batch_interval_minutes" {
			found = setting.Value == 5
		}
	}
	if !found {
		t.Fatal("effective batch interval missing")
	}
}
