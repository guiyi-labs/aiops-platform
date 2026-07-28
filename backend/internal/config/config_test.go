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
