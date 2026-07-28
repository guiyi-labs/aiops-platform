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
