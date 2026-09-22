package config

import "time"

// RuntimeReportTTL is how long a published snapshot stays readable. Reports are
// republished well within it, so a missing key means the process stopped reporting.
const RuntimeReportTTL = 2 * time.Minute

// RuntimeSetting is an explicitly allowlisted, non-secret effective startup value.
type RuntimeSetting struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// RuntimeSnapshot describes a single process; it is not deployment desired state.
type RuntimeSnapshot struct {
	Process    string           `json:"process"`
	Instance   string           `json:"instance"`
	StartedAt  time.Time        `json:"started_at"`
	ReportedAt time.Time        `json:"reported_at"`
	Settings   []RuntimeSetting `json:"settings"`
}

// RuntimeSettings excludes credentials and connection strings by construction.
func (c *AppConfig) RuntimeSettings(process string) []RuntimeSetting {
	settings := []RuntimeSetting{{"log_format", c.LogFormat}, {"observability_configured", c.ObservabilityToken != ""}}
	if process == "cron" {
		return append(settings, RuntimeSetting{"batch_interval_minutes", c.BatchIntervalMinutes}, RuntimeSetting{"batch_run_retention_days", c.BatchRunRetentionDays}, RuntimeSetting{"backlog_samples_retention_days", c.BacklogSamplesRetentionDays}, RuntimeSetting{"retention_interval", c.RetentionInterval.String()})
	}
	return append(settings, RuntimeSetting{"api_port", c.APIPort}, RuntimeSetting{"catalog_max_content_bytes", c.CatalogMaxContentBytes}, RuntimeSetting{"stream_retention_enabled", c.StreamRetentionEnabled}, RuntimeSetting{"stream_retention_interval", c.StreamRetentionInterval.String()})
}

// RuntimeSettings reports admin behavior without exposing bootstrap or service secrets.
func (c *AdminConfig) RuntimeSettings() []RuntimeSetting {
	return []RuntimeSetting{{"admin_port", c.AdminPort}, {"log_format", c.LogFormat}, {"secure_cookies", c.SecureCookies}, {"development_origin_configured", c.AllowDevOrigin != ""}, {"trusted_proxies_configured", c.TrustedProxies != ""}, {"proxy_token_configured", c.ProxyToken != ""}, {"observability_configured", c.ObservabilityToken != ""}}
}

// RuntimeSettings reports effective embedder defaults, not namespace overrides.
func (c *EmbedderConfig) RuntimeSettings() []RuntimeSetting {
	return []RuntimeSetting{{"health_port", c.HealthPort}, {"max_attempts_default", c.EmbedMaxAttempts}, {"namespace_poll_interval", c.NamespacePollInterval.String()}, {"stream_retention_enabled", c.StreamRetentionEnabled}, {"stream_retention_interval", c.StreamRetentionInterval.String()}, {"observability_configured", c.ObservabilityToken != ""}}
}
