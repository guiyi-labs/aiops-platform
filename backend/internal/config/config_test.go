package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	t.Setenv("CREDENTIAL_DECRYPTION_KEYS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Environment != "development" {
		t.Fatalf("Environment = %q, want development", cfg.Environment)
	}
	if cfg.HTTPAddress != defaultHTTPAddress {
		t.Fatalf("HTTPAddress = %q, want %q", cfg.HTTPAddress, defaultHTTPAddress)
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("ShutdownTimeout = %s, want 10s", cfg.ShutdownTimeout)
	}
	if !cfg.MetricsHistoryEnabled || cfg.MetricsHistoryRetention != 7*24*time.Hour ||
		cfg.MetricsCollectionInterval != time.Minute || cfg.MetricsCollectionTimeout != 10*time.Second ||
		cfg.MetricsCleanupInterval != time.Hour || cfg.MetricsMaxClusters != 20 || cfg.MetricsMaxConcurrency != 4 {
		t.Fatalf("unexpected metrics history defaults: %#v", cfg)
	}
}

func TestLoadParsesCredentialDecryptionKeys(t *testing.T) {
	t.Setenv("CREDENTIAL_KEY_VERSION", "v2")
	t.Setenv("CREDENTIAL_DECRYPTION_KEYS", `{"v1":"cHJvZHVjdGlvbi1vbmx5LTMyLWJ5dGUta2V5ISE="}`)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.CredentialDecryptionKeys) != 1 || cfg.CredentialDecryptionKeys["v1"] == "" {
		t.Fatalf("CredentialDecryptionKeys = %#v", cfg.CredentialDecryptionKeys)
	}
}

func TestLoadRejectsInvalidCredentialDecryptionKeys(t *testing.T) {
	t.Run("invalid JSON", func(t *testing.T) {
		t.Setenv("CREDENTIAL_DECRYPTION_KEYS", "not-json")
		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want invalid legacy-key JSON error")
		}
	})
	t.Run("active version duplicated", func(t *testing.T) {
		t.Setenv("CREDENTIAL_KEY_VERSION", "v1")
		t.Setenv("CREDENTIAL_DECRYPTION_KEYS", `{"v1":"cHJvZHVjdGlvbi1vbmx5LTMyLWJ5dGUta2V5ISE="}`)
		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want active-version duplication error")
		}
	})
	t.Run("too many legacy keys", func(t *testing.T) {
		t.Setenv("CREDENTIAL_KEY_VERSION", "v10")
		t.Setenv("CREDENTIAL_DECRYPTION_KEYS", `{"v1":"a","v2":"b","v3":"c","v4":"d","v5":"e","v6":"f","v7":"g","v8":"h","v9":"i"}`)
		if _, err := Load(); err == nil {
			t.Fatal("Load() error = nil, want legacy-key count error")
		}
	})
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT", "later")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want duration parsing error")
	}
}

func TestLoadRejectsEnabledAIWithoutKey(t *testing.T) {
	t.Setenv("AI_ENABLED", "true")
	t.Setenv("AI_API_KEY", "")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing AI_API_KEY error")
	}
}

func TestLoadAllowsDisabledAIWithoutKey(t *testing.T) {
	t.Setenv("AI_ENABLED", "false")
	t.Setenv("AI_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AIEnabled || cfg.AIModel == "" || cfg.AIRequestTimeout != 30*time.Second || cfg.AIDailyTokenBudget != 100000 || cfg.AIMaxConcurrentRequests != 2 || cfg.AIMaxOutputTokens != 1200 {
		t.Fatalf("unexpected AI defaults: %#v", cfg)
	}
}

func TestLoadRejectsInvalidAIGuardrails(t *testing.T) {
	t.Setenv("AI_MAX_CONCURRENT_REQUESTS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid AI concurrency error")
	}
}

func TestLoadParsesMetricsHistoryConfiguration(t *testing.T) {
	t.Setenv("METRICS_HISTORY_ENABLED", "false")
	t.Setenv("METRICS_HISTORY_RETENTION", "72h")
	t.Setenv("METRICS_COLLECTION_INTERVAL", "30s")
	t.Setenv("METRICS_COLLECTION_TIMEOUT", "5s")
	t.Setenv("METRICS_CLEANUP_INTERVAL", "30m")
	t.Setenv("METRICS_MAX_CLUSTERS", "12")
	t.Setenv("METRICS_MAX_CONCURRENCY", "3")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.MetricsHistoryEnabled || cfg.MetricsHistoryRetention != 72*time.Hour ||
		cfg.MetricsCollectionInterval != 30*time.Second || cfg.MetricsCollectionTimeout != 5*time.Second ||
		cfg.MetricsCleanupInterval != 30*time.Minute || cfg.MetricsMaxClusters != 12 || cfg.MetricsMaxConcurrency != 3 {
		t.Fatalf("unexpected metrics history configuration: %#v", cfg)
	}
}

func TestLoadRejectsMetricsHistoryConfigurationOutsideEnvelope(t *testing.T) {
	tests := map[string]map[string]string{
		"retention":   {"METRICS_HISTORY_RETENTION": "721h"},
		"timeout":     {"METRICS_COLLECTION_TIMEOUT": "61s"},
		"cleanup":     {"METRICS_CLEANUP_INTERVAL": "0s"},
		"concurrency": {"METRICS_MAX_CLUSTERS": "2", "METRICS_MAX_CONCURRENCY": "3"},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			for key, value := range values {
				t.Setenv(key, value)
			}
			if _, err := Load(); err == nil {
				t.Fatal("Load() error = nil, want metrics history configuration error")
			}
		})
	}
}

func TestLoadAllowsLocalAIWithoutKey(t *testing.T) {
	t.Setenv("AI_ENABLED", "true")
	t.Setenv("AI_BASE_URL", "http://127.0.0.1:18080/v1")
	t.Setenv("AI_API_KEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.AIEnabled || cfg.AIBaseURL != "http://127.0.0.1:18080/v1" {
		t.Fatalf("unexpected local AI config: %#v", cfg)
	}
}

func TestLoadRejectsEnabledNotificationsWithoutSecureConfiguration(t *testing.T) {
	t.Setenv("NOTIFICATION_ENABLED", "true")
	t.Setenv("NOTIFICATION_WEBHOOK_URL", "")
	t.Setenv("NOTIFICATION_WEBHOOK_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want notification configuration error")
	}
}

func TestLoadAllowsSignedLocalNotificationWebhook(t *testing.T) {
	t.Setenv("NOTIFICATION_ENABLED", "true")
	t.Setenv("NOTIFICATION_WEBHOOK_URL", "http://127.0.0.1:18081/hooks/diagnosis")
	t.Setenv("NOTIFICATION_WEBHOOK_SECRET", "0123456789abcdef0123456789abcdef")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.NotificationEnabled || cfg.NotificationWebhookURL == "" || cfg.NotificationMaxAttempts != 5 || cfg.NotificationBatchSize != 10 {
		t.Fatalf("unexpected notification config: %#v", cfg)
	}
}

func TestLoadRejectsInsecureProductionNotificationWebhook(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "production-admin-password")
	t.Setenv("CREDENTIAL_ENCRYPTION_KEY", "cHJvZHVjdGlvbi1vbmx5LTMyLWJ5dGUta2V5ISE=")
	t.Setenv("NOTIFICATION_ENABLED", "true")
	t.Setenv("NOTIFICATION_WEBHOOK_URL", "http://example.com/hooks")
	t.Setenv("NOTIFICATION_WEBHOOK_SECRET", "0123456789abcdef0123456789abcdef")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want HTTPS notification webhook error")
	}
}

func setValidOIDCEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OIDC_ENABLED", "true")
	t.Setenv("OIDC_ISSUER", "https://idp.example.com")
	t.Setenv("OIDC_CLIENT_ID", "aiops-platform")
	t.Setenv("OIDC_REDIRECT_URI", "https://platform.example.com/auth/oidc/callback")
	t.Setenv("OIDC_CLAIM_USERNAME", "preferred_username")
	t.Setenv("OIDC_CLAIM_DISPLAY_NAME", "name")
	t.Setenv("OIDC_CLAIM_GROUPS", "groups")
	t.Setenv("OIDC_ALLOWED_SIGNING_ALGORITHMS", "RS256,ES256")
	t.Setenv("OIDC_GROUP_TO_ROLES", `{"oidc-admins":["system_admin"],"oidc-viewers":["viewer"]}`)
	t.Setenv("OIDC_MFA_REQUIRED", "true")
	t.Setenv("OIDC_MFA_EVIDENCE_CLAIM", "acr")
	t.Setenv("OIDC_MFA_ACCEPTED_VALUES", "mfa,otp")
	t.Setenv("OIDC_AUTH_SESSION_SIGNING_KEY", "test-oidc-auth-session-signing-key-32b!")
}

func TestLoadOIDCDisabledByDefault(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OIDC.Enabled {
		t.Fatalf("OIDC.Enabled = true, want false by default")
	}
}

func TestLoadOIDCEnabledValid(t *testing.T) {
	setValidOIDCEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.OIDC.Enabled {
		t.Fatalf("OIDC.Enabled = false, want true")
	}
	if cfg.OIDC.Issuer != "https://idp.example.com" {
		t.Fatalf("OIDC.Issuer = %q", cfg.OIDC.Issuer)
	}
	if cfg.OIDC.ClaimMapping.Subject != "sub" {
		t.Fatalf("OIDC.ClaimMapping.Subject = %q, want sub", cfg.OIDC.ClaimMapping.Subject)
	}
	if len(cfg.OIDC.GroupToRoles) != 2 {
		t.Fatalf("OIDC.GroupToRoles = %#v", cfg.OIDC.GroupToRoles)
	}
	if cfg.OIDC.Sessions.MaxAge != 8*time.Hour || cfg.OIDC.Sessions.Reauthentication != time.Hour {
		t.Fatalf("unexpected OIDC session defaults: %#v", cfg.OIDC.Sessions)
	}
	if cfg.OIDC.JWKS.CacheTTL != time.Hour || cfg.OIDC.JWKS.RefreshTimeout != 10*time.Second {
		t.Fatalf("unexpected OIDC JWKS defaults: %#v", cfg.OIDC.JWKS)
	}
}

func TestLoadOIDCIgnoresValidationWhenDisabled(t *testing.T) {
	t.Setenv("OIDC_ENABLED", "false")
	t.Setenv("OIDC_ISSUER", "http://idp.example.com/")
	t.Setenv("OIDC_CLIENT_ID", "")
	t.Setenv("OIDC_GROUP_TO_ROLES", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil when OIDC is disabled", err)
	}
	if cfg.OIDC.Enabled {
		t.Fatal("OIDC.Enabled = true, want false")
	}
}

func TestLoadRejectsOIDCEnabledWithInvalidConfiguration(t *testing.T) {
	cases := map[string]map[string]string{
		"http issuer":            {"OIDC_ISSUER": "http://idp.example.com"},
		"trailing slash issuer":  {"OIDC_ISSUER": "https://idp.example.com/"},
		"missing client id":      {"OIDC_CLIENT_ID": ""},
		"http redirect":          {"OIDC_REDIRECT_URI": "http://platform.example.com/auth/oidc/callback"},
		"fragment redirect":      {"OIDC_REDIRECT_URI": "https://platform.example.com/auth/oidc/callback#state"},
		"non-sub subject":        {"OIDC_CLAIM_SUBJECT": "user_id"},
		"missing username claim": {"OIDC_CLAIM_USERNAME": ""},
		"missing scopes":         {"OIDC_REQUIRED_SCOPES": "email"},
		"no algorithms":          {"OIDC_ALLOWED_SIGNING_ALGORITHMS": ""},
		"bad algorithm":          {"OIDC_ALLOWED_SIGNING_ALGORITHMS": "HS256"},
		"no group mapping":       {"OIDC_GROUP_TO_ROLES": ""},
		"unknown role":           {"OIDC_GROUP_TO_ROLES": `{"g":["superuser"]}`},
		"mfa not required":       {"OIDC_MFA_REQUIRED": "false"},
		"bad evidence claim":     {"OIDC_MFA_EVIDENCE_CLAIM": "sid"},
		"no accepted values":     {"OIDC_MFA_ACCEPTED_VALUES": ""},
		"max age too short":      {"OIDC_SESSION_MAX_AGE": "1m"},
		"reauth too long":        {"OIDC_SESSION_REAUTHENTICATION": "999h"},
		"no revoke on disable":   {"OIDC_SESSION_REVOKE_ON_DISABLE": "false"},
		"break glass disabled":   {"OIDC_BREAK_GLASS_ENABLED": "false"},
		"too many break glass":   {"OIDC_BREAK_GLASS_MAX_ACCOUNTS": "3"},
		"jwks ttl too short":     {"OIDC_JWKS_CACHE_TTL": "1s"},
		"jwks refresh too long":  {"OIDC_JWKS_REFRESH_TIMEOUT": "999s"},
	}
	for name, overrides := range cases {
		t.Run(name, func(t *testing.T) {
			setValidOIDCEnv(t)
			for key, value := range overrides {
				t.Setenv(key, value)
			}
			if _, err := Load(); err == nil {
				t.Fatalf("Load() error = nil, want OIDC validation error for %q", name)
			}
		})
	}
}

func TestLoadRejectsOIDCEnabledInProductionWithoutClientSecret(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "production-admin-password")
	t.Setenv("CREDENTIAL_ENCRYPTION_KEY", "cHJvZHVjdGlvbi1vbmx5LTMyLWJ5dGUta2V5ISE=")
	setValidOIDCEnv(t)
	t.Setenv("OIDC_CLIENT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing OIDC_CLIENT_SECRET error in production")
	}
}

func TestLoadAcceptsOIDCEnabledInProductionWithClientSecret(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "production-admin-password")
	t.Setenv("CREDENTIAL_ENCRYPTION_KEY", "cHJvZHVjdGlvbi1vbmx5LTMyLWJ5dGUta2V5ISE=")
	setValidOIDCEnv(t)
	t.Setenv("OIDC_CLIENT_SECRET", "production-confidential-client-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.OIDC.ClientSecret == "" {
		t.Fatal("OIDC.ClientSecret = empty, want configured secret")
	}
}

func TestLoadOIDCGroupToRolesDeduplicatesRoles(t *testing.T) {
	setValidOIDCEnv(t)
	t.Setenv("OIDC_GROUP_TO_ROLES", `{"oidc-admins":["system_admin","system_admin","viewer"]}`)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	roles := cfg.OIDC.GroupToRoles["oidc-admins"]
	if len(roles) != 2 {
		t.Fatalf("deduplicated roles = %v, want 2 entries", roles)
	}
}

func TestLoadOIDCGroupToRolesRejectsEmptyMapping(t *testing.T) {
	setValidOIDCEnv(t)
	t.Setenv("OIDC_GROUP_TO_ROLES", `{"oidc-admins":[]}`)
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want empty role mapping error")
	}
}

func TestLoadCapabilityDisabledByDefault(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Capability.Enabled {
		t.Fatalf("Capability.Enabled = true, want false by default")
	}
	if cfg.Capability.PrometheusTimeout != 10*time.Second || cfg.Capability.LokiTimeout != 15*time.Second {
		t.Fatalf("unexpected capability defaults: %#v", cfg.Capability)
	}
}

func TestLoadRejectsEnabledCapabilityWithoutEndpoints(t *testing.T) {
	t.Setenv("CAPABILITY_ENABLED", "true")
	t.Setenv("CAPABILITY_PROMETHEUS_ENDPOINT", "")
	t.Setenv("CAPABILITY_LOKI_ENDPOINT", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want missing capability endpoint error")
	}
}

func TestLoadRejectsInsecureProductionCapabilityEndpoints(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "production-admin-password")
	t.Setenv("CREDENTIAL_ENCRYPTION_KEY", "cHJvZHVjdGlvbi1vbmx5LTMyLWJ5dGUta2V5ISE=")
	t.Setenv("CAPABILITY_ENABLED", "true")
	t.Setenv("CAPABILITY_PROMETHEUS_ENDPOINT", "http://prometheus.example.com")
	t.Setenv("CAPABILITY_LOKI_ENDPOINT", "")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want HTTPS capability endpoint error")
	}
}

func TestLoadAlertRouteDisabledByDefault(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AlertRoute.Enabled {
		t.Fatalf("AlertRoute.Enabled = true, want false by default")
	}
	if cfg.AlertRoute.PollInterval != 5*time.Second || cfg.AlertRoute.RequestTimeout != 10*time.Second ||
		cfg.AlertRoute.RetryBase != 10*time.Second || cfg.AlertRoute.MaxAttempts != 5 || cfg.AlertRoute.BatchSize != 10 {
		t.Fatalf("unexpected alert route defaults: %#v", cfg.AlertRoute)
	}
}

func TestLoadRejectsAlertRouteNonPositiveIntervals(t *testing.T) {
	t.Setenv("ALERT_ROUTE_POLL_INTERVAL", "0s")
	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want non-positive interval error")
	}
}
