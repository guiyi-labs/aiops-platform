package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

const (
	defaultDatabaseURL = "postgres://aiops:change_me@localhost:5432/aiops?sslmode=disable"
	defaultHTTPAddress = ":8080"
)

type Config struct {
	Environment                string
	HTTPAddress                string
	DatabaseURL                string
	ShutdownTimeout            time.Duration
	ReadHeaderTimeout          time.Duration
	ReadTimeout                time.Duration
	WriteTimeout               time.Duration
	IdleTimeout                time.Duration
	JWTSigningKey              string
	AccessTokenTTL             time.Duration
	RefreshTokenTTL            time.Duration
	BootstrapUsername          string
	BootstrapPassword          string
	SecureCookies              bool
	CredentialEncryptionKey    string
	CredentialKeyVersion       string
	ClusterProbeTimeout        time.Duration
	AIEnabled                  bool
	AIBaseURL                  string
	AIAPIKey                   string
	AIModel                    string
	AIRequestTimeout           time.Duration
	AIDailyTokenBudget         int
	AIMaxConcurrentRequests    int
	AIMaxOutputTokens          int
	NotificationEnabled        bool
	NotificationWebhookURL     string
	NotificationWebhookSecret  string
	NotificationPollInterval   time.Duration
	NotificationRequestTimeout time.Duration
	NotificationRetryBase      time.Duration
	NotificationMaxAttempts    int
	NotificationBatchSize      int
}

func Load() (Config, error) {
	shutdownTimeout, err := durationFromEnv("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	accessTokenTTL, err := durationFromEnv("ACCESS_TOKEN_TTL", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	refreshTokenTTL, err := durationFromEnv("REFRESH_TOKEN_TTL", 7*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	clusterProbeTimeout, err := durationFromEnv("CLUSTER_PROBE_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	aiRequestTimeout, err := durationFromEnv("AI_REQUEST_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	aiEnabled, err := boolFromEnv("AI_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	aiDailyTokenBudget, err := intFromEnv("AI_DAILY_TOKEN_BUDGET", 100000, 0, 1000000000)
	if err != nil {
		return Config{}, err
	}
	aiMaxConcurrentRequests, err := intFromEnv("AI_MAX_CONCURRENT_REQUESTS", 2, 1, 20)
	if err != nil {
		return Config{}, err
	}
	aiMaxOutputTokens, err := intFromEnv("AI_MAX_OUTPUT_TOKENS", 1200, 128, 8192)
	if err != nil {
		return Config{}, err
	}
	notificationPollInterval, err := durationFromEnv("NOTIFICATION_POLL_INTERVAL", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	notificationRequestTimeout, err := durationFromEnv("NOTIFICATION_REQUEST_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	notificationRetryBase, err := durationFromEnv("NOTIFICATION_RETRY_BASE", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	if notificationPollInterval <= 0 || notificationRequestTimeout <= 0 || notificationRetryBase <= 0 {
		return Config{}, fmt.Errorf("notification intervals and timeouts must be positive")
	}
	notificationMaxAttempts, err := intFromEnv("NOTIFICATION_MAX_ATTEMPTS", 5, 1, 20)
	if err != nil {
		return Config{}, err
	}
	notificationBatchSize, err := intFromEnv("NOTIFICATION_BATCH_SIZE", 10, 1, 100)
	if err != nil {
		return Config{}, err
	}

	environment := stringFromEnv("APP_ENV", "development")
	jwtSigningKey := stringFromEnv("JWT_SIGNING_KEY", "dev-only-signing-key-change-before-production")
	if len(jwtSigningKey) < 32 {
		return Config{}, fmt.Errorf("JWT_SIGNING_KEY must contain at least 32 characters")
	}
	bootstrapPassword := stringFromEnv("BOOTSTRAP_ADMIN_PASSWORD", "change_me_now")
	if environment == "production" && bootstrapPassword == "change_me_now" {
		return Config{}, fmt.Errorf("BOOTSTRAP_ADMIN_PASSWORD must be changed in production")
	}
	credentialKey := stringFromEnv("CREDENTIAL_ENCRYPTION_KEY", "ZGV2LW9ubHktMzItYnl0ZS1rZXktY2hhbmdlLW5vdyE=")
	if environment == "production" && credentialKey == "ZGV2LW9ubHktMzItYnl0ZS1rZXktY2hhbmdlLW5vdyE=" {
		return Config{}, fmt.Errorf("CREDENTIAL_ENCRYPTION_KEY must be changed in production")
	}
	aiBaseURL := stringFromEnv("AI_BASE_URL", "https://api.openai.com/v1")
	parsedAIBaseURL, err := url.Parse(aiBaseURL)
	if err != nil || parsedAIBaseURL.Host == "" || (parsedAIBaseURL.Scheme != "https" && parsedAIBaseURL.Scheme != "http") {
		return Config{}, fmt.Errorf("AI_BASE_URL must be an absolute HTTP(S) URL")
	}
	if environment == "production" && parsedAIBaseURL.Scheme != "https" {
		return Config{}, fmt.Errorf("AI_BASE_URL must use HTTPS in production")
	}
	aiAPIKey := stringFromEnv("AI_API_KEY", "")
	if aiEnabled && aiAPIKey == "" && parsedAIBaseURL.Hostname() != "localhost" && parsedAIBaseURL.Hostname() != "127.0.0.1" && parsedAIBaseURL.Hostname() != "::1" {
		return Config{}, fmt.Errorf("AI_API_KEY is required when AI_ENABLED is true")
	}
	notificationEnabled, err := boolFromEnv("NOTIFICATION_ENABLED", false)
	if err != nil {
		return Config{}, err
	}
	notificationWebhookURL := stringFromEnv("NOTIFICATION_WEBHOOK_URL", "")
	notificationWebhookSecret := stringFromEnv("NOTIFICATION_WEBHOOK_SECRET", "")
	if notificationEnabled {
		parsedWebhookURL, parseErr := url.Parse(notificationWebhookURL)
		if parseErr != nil || parsedWebhookURL.Host == "" || parsedWebhookURL.User != nil || (parsedWebhookURL.Scheme != "https" && parsedWebhookURL.Scheme != "http") {
			return Config{}, fmt.Errorf("NOTIFICATION_WEBHOOK_URL must be an absolute HTTP(S) URL when notifications are enabled")
		}
		if environment == "production" && parsedWebhookURL.Scheme != "https" {
			return Config{}, fmt.Errorf("NOTIFICATION_WEBHOOK_URL must use HTTPS in production")
		}
		if len(notificationWebhookSecret) < 32 {
			return Config{}, fmt.Errorf("NOTIFICATION_WEBHOOK_SECRET must contain at least 32 characters when notifications are enabled")
		}
	}

	return Config{
		Environment:                environment,
		HTTPAddress:                stringFromEnv("HTTP_ADDR", defaultHTTPAddress),
		DatabaseURL:                stringFromEnv("DATABASE_URL", defaultDatabaseURL),
		ShutdownTimeout:            shutdownTimeout,
		ReadHeaderTimeout:          5 * time.Second,
		ReadTimeout:                15 * time.Second,
		WriteTimeout:               30 * time.Second,
		IdleTimeout:                60 * time.Second,
		JWTSigningKey:              jwtSigningKey,
		AccessTokenTTL:             accessTokenTTL,
		RefreshTokenTTL:            refreshTokenTTL,
		BootstrapUsername:          stringFromEnv("BOOTSTRAP_ADMIN_USERNAME", "admin"),
		BootstrapPassword:          bootstrapPassword,
		SecureCookies:              environment == "production",
		CredentialEncryptionKey:    credentialKey,
		CredentialKeyVersion:       stringFromEnv("CREDENTIAL_KEY_VERSION", "v1"),
		ClusterProbeTimeout:        clusterProbeTimeout,
		AIEnabled:                  aiEnabled,
		AIBaseURL:                  aiBaseURL,
		AIAPIKey:                   aiAPIKey,
		AIModel:                    stringFromEnv("AI_MODEL", "gpt-5.4-mini"),
		AIRequestTimeout:           aiRequestTimeout,
		AIDailyTokenBudget:         aiDailyTokenBudget,
		AIMaxConcurrentRequests:    aiMaxConcurrentRequests,
		AIMaxOutputTokens:          aiMaxOutputTokens,
		NotificationEnabled:        notificationEnabled,
		NotificationWebhookURL:     notificationWebhookURL,
		NotificationWebhookSecret:  notificationWebhookSecret,
		NotificationPollInterval:   notificationPollInterval,
		NotificationRequestTimeout: notificationRequestTimeout,
		NotificationRetryBase:      notificationRetryBase,
		NotificationMaxAttempts:    notificationMaxAttempts,
		NotificationBatchSize:      notificationBatchSize,
	}, nil
}

func stringFromEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return value, nil
}

func boolFromEnv(key string, fallback bool) (bool, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}
	return value, nil
}

func intFromEnv(key string, fallback, minimum, maximum int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", key, minimum, maximum)
	}
	return value, nil
}
