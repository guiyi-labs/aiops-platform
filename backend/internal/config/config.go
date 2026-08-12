package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
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
	CredentialDecryptionKeys   map[string]string
	ClusterProbeTimeout        time.Duration
	MetricsHistoryEnabled      bool
	MetricsHistoryRetention    time.Duration
	MetricsCollectionInterval  time.Duration
	MetricsCollectionTimeout   time.Duration
	MetricsCleanupInterval     time.Duration
	MetricsMaxClusters         int
	MetricsMaxConcurrency      int
	CorrelationInterval        time.Duration
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
	AlertEnabled               bool
	AlertPollInterval          time.Duration
	AlertClaimBatch            int
	AlertWorkerConcurrency     int
	AlertEvaluationTimeout     time.Duration
	AlertClaimLease            time.Duration
	AlertMinEvaluationInterval time.Duration
	AlertMaxRulesPerCluster    int
	Capability                 CapabilityConfig
	AlertRoute                 AlertRouteConfig
	OIDC                       OIDCConfig
	Signal                     SignalConfig
}

// allowedOIDCRoleCodes mirrors auth.SystemAdmin/OperationsAdmin/SecurityAuditor/
// Viewer. It is kept local to avoid a config -> auth dependency.
var allowedOIDCRoleCodes = map[string]struct{}{
	"system_admin":     {},
	"operations_admin": {},
	"security_auditor": {},
	"viewer":           {},
}

var allowedOIDCSigningAlgorithms = map[string]struct{}{
	"RS256": {}, "PS256": {}, "ES256": {}, "EdDSA": {},
}

// OIDCConfig holds the optional production OIDC provider configuration. When
// Enabled is false the platform authenticates local accounts only. When Enabled
// is true the configuration must satisfy the same fail-closed rules as the
// offline identity-readiness gate (ADR 0032): canonical HTTPS issuer, PKCE
// S256, immutable subject mapping, identity-provider-enforced MFA and
// admin-prelinked subjects (never automatic email linking).
//
// The provider client secret (when the registered client is confidential) is
// supplied exclusively through ClientSecret from external configuration and
// never enters the browser, audit trail, logs or policy file.
type OIDCConfig struct {
	Enabled                  bool
	Issuer                   string
	ClientID                 string
	ClientSecret             string
	RedirectURI              string
	RequiredScopes           []string
	ClaimMapping             OIDCClaimMapping
	AllowedSigningAlgorithms []string
	GroupToRoles             map[string][]string
	MFA                      OIDCMFAConfig
	Sessions                 OIDCSessionConfig
	BreakGlass               OIDCBreakGlassConfig
	JWKS                     OIDCJWKSConfig
	// AuthSessionSigningKey signs the short-lived auth-session cookie that
	// carries PKCE state/nonce between the authorization redirect and the
	// callback. It must be at least 32 bytes and is supplied exclusively from
	// environment configuration, never from the browser, audit trail or logs.
	AuthSessionSigningKey []byte
}

// OIDCClaimMapping binds immutable OIDC claims to local account attributes.
// Subject must always be "sub"; the other fields name provider-specific claims.
type OIDCClaimMapping struct {
	Subject     string
	Username    string
	DisplayName string
	Groups      string
}

// OIDCMFAConfig requires identity-provider-enforced MFA with an accepted acr or
// amr evidence value present in every issued ID token.
type OIDCMFAConfig struct {
	Required       bool
	EvidenceClaim  string
	AcceptedValues []string
}

// OIDCSessionConfig bounds the local OIDC session lifetime and forces local
// session revocation when an identity is disabled.
type OIDCSessionConfig struct {
	MaxAge           time.Duration
	Reauthentication time.Duration
	RevokeOnDisable  bool
}

// OIDCBreakGlassConfig retains one or two local break-glass accounts so that
// operator access survives a provider outage.
type OIDCBreakGlassConfig struct {
	Enabled     bool
	MaxAccounts int
}

// OIDCJWKSConfig bounds the runtime JWKS cache and refresh behaviour so key
// rotation takes effect without a restart.
type OIDCJWKSConfig struct {
	CacheTTL       time.Duration
	RefreshTimeout time.Duration
}

// CapabilityConfig holds the optional M37 capability adapter configuration for
// external metric and log sources (Prometheus, Loki). When Enabled is false the
// platform relies on its internal collectors only.
type CapabilityConfig struct {
	Enabled            bool
	PrometheusEndpoint string
	PrometheusTimeout  time.Duration
	LokiEndpoint       string
	LokiTimeout        time.Duration
}

// AlertRouteConfig holds the optional M37 alert route delivery configuration
// that fans evaluated alerts out to registered receivers.
type AlertRouteConfig struct {
	Enabled        bool
	PollInterval   time.Duration
	RequestTimeout time.Duration
	RetryBase      time.Duration
	MaxAttempts    int
	BatchSize      int
}

// SignalConfig holds the optional M39 AIOps signal model configuration. The
// signal service is disabled by default; when enabled it normalizes existing
// M21-M31 outputs into the unified signal_occurrences table.
type SignalConfig struct {
	Enabled         bool
	RetentionBatch  int
	ListLimit       int
	OverviewTopN    int
	OverviewWindow  time.Duration
	CleanupInterval time.Duration
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
	metricsHistoryEnabled, err := boolFromEnv("METRICS_HISTORY_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	metricsHistoryRetention, err := durationFromEnv("METRICS_HISTORY_RETENTION", 7*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	metricsCollectionInterval, err := durationFromEnv("METRICS_COLLECTION_INTERVAL", time.Minute)
	if err != nil {
		return Config{}, err
	}
	metricsCollectionTimeout, err := durationFromEnv("METRICS_COLLECTION_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	metricsCleanupInterval, err := durationFromEnv("METRICS_CLEANUP_INTERVAL", time.Hour)
	if err != nil {
		return Config{}, err
	}
	metricsMaxClusters, err := intFromEnv("METRICS_MAX_CLUSTERS", 20, 1, 20)
	if err != nil {
		return Config{}, err
	}
	metricsMaxConcurrency, err := intFromEnv("METRICS_MAX_CONCURRENCY", 4, 1, 4)
	if err != nil {
		return Config{}, err
	}
	if metricsHistoryRetention < time.Hour || metricsHistoryRetention > 30*24*time.Hour {
		return Config{}, fmt.Errorf("METRICS_HISTORY_RETENTION must be between 1h and 720h")
	}
	if metricsCollectionInterval < 15*time.Second || metricsCollectionInterval > 24*time.Hour {
		return Config{}, fmt.Errorf("METRICS_COLLECTION_INTERVAL must be between 15s and 24h")
	}
	if metricsCollectionTimeout < time.Second || metricsCollectionTimeout > time.Minute {
		return Config{}, fmt.Errorf("METRICS_COLLECTION_TIMEOUT must be between 1s and 1m")
	}
	if metricsCleanupInterval < time.Minute || metricsCleanupInterval > 24*time.Hour {
		return Config{}, fmt.Errorf("METRICS_CLEANUP_INTERVAL must be between 1m and 24h")
	}
	if metricsMaxConcurrency > metricsMaxClusters {
		return Config{}, fmt.Errorf("METRICS_MAX_CONCURRENCY must not exceed METRICS_MAX_CLUSTERS")
	}
	correlationInterval, err := durationFromEnv("CORRELATION_INTERVAL", 5*time.Minute)
	if err != nil {
		return Config{}, err
	}
	if correlationInterval < 30*time.Second || correlationInterval > 24*time.Hour {
		return Config{}, fmt.Errorf("CORRELATION_INTERVAL must be between 30s and 24h")
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
	credentialKeyVersion := stringFromEnv("CREDENTIAL_KEY_VERSION", "v1")
	credentialDecryptionKeys, err := stringMapFromEnv("CREDENTIAL_DECRYPTION_KEYS", 8)
	if err != nil {
		return Config{}, err
	}
	if _, duplicate := credentialDecryptionKeys[credentialKeyVersion]; duplicate {
		return Config{}, fmt.Errorf("CREDENTIAL_DECRYPTION_KEYS must not contain the active CREDENTIAL_KEY_VERSION")
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

	alertEnabled, err := boolFromEnv("ALERT_ENABLED", true)
	if err != nil {
		return Config{}, err
	}
	alertPollInterval, err := durationFromEnv("ALERT_POLL_INTERVAL", 15*time.Second)
	if err != nil {
		return Config{}, err
	}
	alertClaimBatch, err := intFromEnv("ALERT_CLAIM_BATCH", 20, 1, 20)
	if err != nil {
		return Config{}, err
	}
	alertWorkerConcurrency, err := intFromEnv("ALERT_WORKER_CONCURRENCY", 4, 1, 4)
	if err != nil {
		return Config{}, err
	}
	alertEvaluationTimeout, err := durationFromEnv("ALERT_EVALUATION_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	alertClaimLease, err := durationFromEnv("ALERT_CLAIM_LEASE", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	alertMinEvalInterval, err := durationFromEnv("ALERT_MIN_EVALUATION_INTERVAL", 60*time.Second)
	if err != nil {
		return Config{}, err
	}
	alertMaxRulesPerCluster, err := intFromEnv("ALERT_MAX_RULES_PER_CLUSTER", 20, 1, 100)
	if err != nil {
		return Config{}, err
	}
	if alertPollInterval < time.Second {
		return Config{}, fmt.Errorf("ALERT_POLL_INTERVAL must be at least 1s")
	}
	if alertClaimLease < time.Second {
		return Config{}, fmt.Errorf("ALERT_CLAIM_LEASE must be at least 1s")
	}
	if alertMinEvalInterval < time.Second {
		return Config{}, fmt.Errorf("ALERT_MIN_EVALUATION_INTERVAL must be at least 1s")
	}

	capabilityCfg, err := loadCapabilityConfig(environment)
	if err != nil {
		return Config{}, err
	}
	alertRouteCfg, err := loadAlertRouteConfig(environment)
	if err != nil {
		return Config{}, err
	}

	oidcCfg, err := loadOIDCConfig(environment)
	if err != nil {
		return Config{}, err
	}

	signalCfg, err := loadSignalConfig(environment)
	if err != nil {
		return Config{}, err
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
		CredentialKeyVersion:       credentialKeyVersion,
		CredentialDecryptionKeys:   credentialDecryptionKeys,
		ClusterProbeTimeout:        clusterProbeTimeout,
		MetricsHistoryEnabled:      metricsHistoryEnabled,
		MetricsHistoryRetention:    metricsHistoryRetention,
		MetricsCollectionInterval:  metricsCollectionInterval,
		MetricsCollectionTimeout:   metricsCollectionTimeout,
		MetricsCleanupInterval:     metricsCleanupInterval,
		MetricsMaxClusters:         metricsMaxClusters,
		MetricsMaxConcurrency:      metricsMaxConcurrency,
		CorrelationInterval:        correlationInterval,
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
		AlertEnabled:               alertEnabled,
		AlertPollInterval:          alertPollInterval,
		AlertClaimBatch:            alertClaimBatch,
		AlertWorkerConcurrency:     alertWorkerConcurrency,
		AlertEvaluationTimeout:     alertEvaluationTimeout,
		AlertClaimLease:            alertClaimLease,
		AlertMinEvaluationInterval: alertMinEvalInterval,
		AlertMaxRulesPerCluster:    alertMaxRulesPerCluster,
		Capability:                 capabilityCfg,
		AlertRoute:                 alertRouteCfg,
		OIDC:                       oidcCfg,
		Signal:                     signalCfg,
	}, nil
}

func stringFromEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func stringMapFromEnv(key string, maximumEntries int) (map[string]string, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return map[string]string{}, nil
	}
	values := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object containing string values", key)
	}
	if len(values) > maximumEntries {
		return nil, fmt.Errorf("%s must contain at most %d entries", key, maximumEntries)
	}
	for version, encodedKey := range values {
		if version == "" || encodedKey == "" {
			return nil, fmt.Errorf("%s versions and keys must not be empty", key)
		}
	}
	return values, nil
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

func stringListFromEnv(key string, fallback []string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	if len(values) == 0 {
		return fallback
	}
	return values
}

func groupToRolesFromEnv(key string) (map[string][]string, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return map[string][]string{}, nil
	}
	var parsed map[string][]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object mapping group names to arrays of role codes", key)
	}
	result := make(map[string][]string, len(parsed))
	for group, roles := range parsed {
		group = strings.TrimSpace(group)
		if group == "" {
			return nil, fmt.Errorf("%s group names must not be empty", key)
		}
		seen := make(map[string]struct{}, len(roles))
		cleaned := make([]string, 0, len(roles))
		for _, role := range roles {
			role = strings.TrimSpace(role)
			if _, ok := allowedOIDCRoleCodes[role]; !ok {
				return nil, fmt.Errorf("%s references unknown role code %q", key, role)
			}
			if _, dup := seen[role]; dup {
				continue
			}
			seen[role] = struct{}{}
			cleaned = append(cleaned, role)
		}
		if len(cleaned) == 0 {
			return nil, fmt.Errorf("%s group %q must map to at least one role", key, group)
		}
		result[group] = cleaned
	}
	return result, nil
}

func containsAllStrings(values, required []string) bool {
	for _, requiredValue := range required {
		found := false
		for _, value := range values {
			if value == requiredValue {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func loadOIDCConfig(environment string) (OIDCConfig, error) {
	enabled, err := boolFromEnv("OIDC_ENABLED", false)
	if err != nil {
		return OIDCConfig{}, err
	}
	breakGlassEnabled, err := boolFromEnv("OIDC_BREAK_GLASS_ENABLED", enabled)
	if err != nil {
		return OIDCConfig{}, err
	}
	breakGlassMaxAccounts, err := intFromEnv("OIDC_BREAK_GLASS_MAX_ACCOUNTS", 1, 1, 2)
	if err != nil {
		return OIDCConfig{}, err
	}
	mfaRequired, err := boolFromEnv("OIDC_MFA_REQUIRED", enabled)
	if err != nil {
		return OIDCConfig{}, err
	}
	revokeOnDisable, err := boolFromEnv("OIDC_SESSION_REVOKE_ON_DISABLE", true)
	if err != nil {
		return OIDCConfig{}, err
	}
	maxAge, err := durationFromEnv("OIDC_SESSION_MAX_AGE", 8*time.Hour)
	if err != nil {
		return OIDCConfig{}, err
	}
	reauthentication, err := durationFromEnv("OIDC_SESSION_REAUTHENTICATION", time.Hour)
	if err != nil {
		return OIDCConfig{}, err
	}
	jwksCacheTTL, err := durationFromEnv("OIDC_JWKS_CACHE_TTL", time.Hour)
	if err != nil {
		return OIDCConfig{}, err
	}
	jwksRefreshTimeout, err := durationFromEnv("OIDC_JWKS_REFRESH_TIMEOUT", 10*time.Second)
	if err != nil {
		return OIDCConfig{}, err
	}
	groupToRoles, err := groupToRolesFromEnv("OIDC_GROUP_TO_ROLES")
	if err != nil {
		return OIDCConfig{}, err
	}
	cfg := OIDCConfig{
		Enabled:        enabled,
		Issuer:         stringFromEnv("OIDC_ISSUER", ""),
		ClientID:       stringFromEnv("OIDC_CLIENT_ID", ""),
		ClientSecret:   stringFromEnv("OIDC_CLIENT_SECRET", ""),
		RedirectURI:    stringFromEnv("OIDC_REDIRECT_URI", ""),
		RequiredScopes: stringListFromEnv("OIDC_REQUIRED_SCOPES", []string{"openid", "profile"}),
		ClaimMapping: OIDCClaimMapping{
			Subject:     stringFromEnv("OIDC_CLAIM_SUBJECT", "sub"),
			Username:    stringFromEnv("OIDC_CLAIM_USERNAME", ""),
			DisplayName: stringFromEnv("OIDC_CLAIM_DISPLAY_NAME", ""),
			Groups:      stringFromEnv("OIDC_CLAIM_GROUPS", ""),
		},
		AllowedSigningAlgorithms: stringListFromEnv("OIDC_ALLOWED_SIGNING_ALGORITHMS", nil),
		GroupToRoles:             groupToRoles,
		MFA: OIDCMFAConfig{
			Required:       mfaRequired,
			EvidenceClaim:  stringFromEnv("OIDC_MFA_EVIDENCE_CLAIM", ""),
			AcceptedValues: stringListFromEnv("OIDC_MFA_ACCEPTED_VALUES", nil),
		},
		Sessions: OIDCSessionConfig{
			MaxAge:           maxAge,
			Reauthentication: reauthentication,
			RevokeOnDisable:  revokeOnDisable,
		},
		BreakGlass: OIDCBreakGlassConfig{
			Enabled:     breakGlassEnabled,
			MaxAccounts: breakGlassMaxAccounts,
		},
		JWKS: OIDCJWKSConfig{
			CacheTTL:       jwksCacheTTL,
			RefreshTimeout: jwksRefreshTimeout,
		},
		AuthSessionSigningKey: []byte(stringFromEnv("OIDC_AUTH_SESSION_SIGNING_KEY", "")),
	}
	if err := cfg.validate(environment); err != nil {
		return OIDCConfig{}, err
	}
	return cfg, nil
}

func (c OIDCConfig) validate(environment string) error {
	if !c.Enabled {
		return nil
	}
	issuer, err := url.Parse(c.Issuer)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.User != nil {
		return fmt.Errorf("OIDC_ISSUER must be a canonical HTTPS URL when OIDC is enabled")
	}
	if strings.TrimSuffix(c.Issuer, "/") != c.Issuer {
		return fmt.Errorf("OIDC_ISSUER must not end with a trailing slash")
	}
	if c.ClientID == "" {
		return fmt.Errorf("OIDC_CLIENT_ID is required when OIDC is enabled")
	}
	redirect, err := url.Parse(c.RedirectURI)
	if err != nil || redirect.Scheme != "https" || redirect.Host == "" || redirect.User != nil || strings.Contains(c.RedirectURI, "#") {
		return fmt.Errorf("OIDC_REDIRECT_URI must be an absolute fragment-free HTTPS URL when OIDC is enabled")
	}
	if c.ClaimMapping.Subject != "sub" {
		return fmt.Errorf("OIDC_CLAIM_SUBJECT must be \"sub\"")
	}
	if c.ClaimMapping.Username == "" || c.ClaimMapping.DisplayName == "" || c.ClaimMapping.Groups == "" {
		return fmt.Errorf("OIDC_CLAIM_USERNAME, OIDC_CLAIM_DISPLAY_NAME and OIDC_CLAIM_GROUPS must be set when OIDC is enabled")
	}
	if !containsAllStrings(c.RequiredScopes, []string{"openid", "profile"}) {
		return fmt.Errorf("OIDC_REQUIRED_SCOPES must include openid and profile")
	}
	if len(c.AllowedSigningAlgorithms) == 0 {
		return fmt.Errorf("OIDC_ALLOWED_SIGNING_ALGORITHMS must be set when OIDC is enabled")
	}
	for _, algorithm := range c.AllowedSigningAlgorithms {
		if _, ok := allowedOIDCSigningAlgorithms[algorithm]; !ok {
			return fmt.Errorf("OIDC_ALLOWED_SIGNING_ALGORITHMS contains unsupported algorithm %q", algorithm)
		}
	}
	if len(c.GroupToRoles) == 0 {
		return fmt.Errorf("OIDC_GROUP_TO_ROLES must map at least one group to platform roles when OIDC is enabled")
	}
	if !c.MFA.Required {
		return fmt.Errorf("OIDC_MFA_REQUIRED must be true when OIDC is enabled")
	}
	if c.MFA.EvidenceClaim != "acr" && c.MFA.EvidenceClaim != "amr" {
		return fmt.Errorf("OIDC_MFA_EVIDENCE_CLAIM must be \"acr\" or \"amr\" when OIDC is enabled")
	}
	if len(c.MFA.AcceptedValues) == 0 {
		return fmt.Errorf("OIDC_MFA_ACCEPTED_VALUES must be set when OIDC is enabled")
	}
	if c.Sessions.MaxAge < 5*time.Minute || c.Sessions.MaxAge > 24*time.Hour {
		return fmt.Errorf("OIDC_SESSION_MAX_AGE must be between 5m and 24h")
	}
	if c.Sessions.Reauthentication < time.Minute || c.Sessions.Reauthentication > c.Sessions.MaxAge {
		return fmt.Errorf("OIDC_SESSION_REAUTHENTICATION must be between 1m and OIDC_SESSION_MAX_AGE")
	}
	if !c.Sessions.RevokeOnDisable {
		return fmt.Errorf("OIDC_SESSION_REVOKE_ON_DISABLE must be true when OIDC is enabled")
	}
	if !c.BreakGlass.Enabled {
		return fmt.Errorf("OIDC_BREAK_GLASS_ENABLED must be true when OIDC is enabled")
	}
	if c.BreakGlass.MaxAccounts < 1 || c.BreakGlass.MaxAccounts > 2 {
		return fmt.Errorf("OIDC_BREAK_GLASS_MAX_ACCOUNTS must be between 1 and 2")
	}
	if c.JWKS.CacheTTL < time.Minute || c.JWKS.CacheTTL > 24*time.Hour {
		return fmt.Errorf("OIDC_JWKS_CACHE_TTL must be between 1m and 24h")
	}
	if c.JWKS.RefreshTimeout < time.Second || c.JWKS.RefreshTimeout > time.Minute {
		return fmt.Errorf("OIDC_JWKS_REFRESH_TIMEOUT must be between 1s and 1m")
	}
	if environment == "production" && c.ClientSecret == "" {
		return fmt.Errorf("OIDC_CLIENT_SECRET is required in production")
	}
	if len(c.AuthSessionSigningKey) < 32 {
		return fmt.Errorf("OIDC_AUTH_SESSION_SIGNING_KEY must be at least 32 bytes when OIDC is enabled")
	}
	return nil
}

func loadCapabilityConfig(environment string) (CapabilityConfig, error) {
	enabled, err := boolFromEnv("CAPABILITY_ENABLED", false)
	if err != nil {
		return CapabilityConfig{}, err
	}
	prometheusTimeout, err := durationFromEnv("CAPABILITY_PROMETHEUS_TIMEOUT", 10*time.Second)
	if err != nil {
		return CapabilityConfig{}, err
	}
	lokiTimeout, err := durationFromEnv("CAPABILITY_LOKI_TIMEOUT", 15*time.Second)
	if err != nil {
		return CapabilityConfig{}, err
	}
	cfg := CapabilityConfig{
		Enabled:            enabled,
		PrometheusEndpoint: stringFromEnv("CAPABILITY_PROMETHEUS_ENDPOINT", ""),
		PrometheusTimeout:  prometheusTimeout,
		LokiEndpoint:       stringFromEnv("CAPABILITY_LOKI_ENDPOINT", ""),
		LokiTimeout:        lokiTimeout,
	}
	if err := cfg.validate(environment); err != nil {
		return CapabilityConfig{}, err
	}
	return cfg, nil
}

func (c CapabilityConfig) validate(environment string) error {
	if !c.Enabled {
		return nil
	}
	if c.PrometheusEndpoint == "" && c.LokiEndpoint == "" {
		return fmt.Errorf("CAPABILITY_PROMETHEUS_ENDPOINT or CAPABILITY_LOKI_ENDPOINT must be set when capability adapters are enabled")
	}
	for _, endpoint := range []string{c.PrometheusEndpoint, c.LokiEndpoint} {
		if endpoint == "" {
			continue
		}
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			return fmt.Errorf("CAPABILITY_PROMETHEUS_ENDPOINT and CAPABILITY_LOKI_ENDPOINT must be absolute HTTP(S) URLs when capability adapters are enabled")
		}
		if environment == "production" && parsed.Scheme != "https" {
			return fmt.Errorf("CAPABILITY_PROMETHEUS_ENDPOINT and CAPABILITY_LOKI_ENDPOINT must use HTTPS in production")
		}
	}
	return nil
}

func loadAlertRouteConfig(environment string) (AlertRouteConfig, error) {
	enabled, err := boolFromEnv("ALERT_ROUTE_ENABLED", false)
	if err != nil {
		return AlertRouteConfig{}, err
	}
	pollInterval, err := durationFromEnv("ALERT_ROUTE_POLL_INTERVAL", 5*time.Second)
	if err != nil {
		return AlertRouteConfig{}, err
	}
	requestTimeout, err := durationFromEnv("ALERT_ROUTE_REQUEST_TIMEOUT", 10*time.Second)
	if err != nil {
		return AlertRouteConfig{}, err
	}
	retryBase, err := durationFromEnv("ALERT_ROUTE_RETRY_BASE", 10*time.Second)
	if err != nil {
		return AlertRouteConfig{}, err
	}
	maxAttempts, err := intFromEnv("ALERT_ROUTE_MAX_ATTEMPTS", 5, 1, 20)
	if err != nil {
		return AlertRouteConfig{}, err
	}
	batchSize, err := intFromEnv("ALERT_ROUTE_BATCH_SIZE", 10, 1, 100)
	if err != nil {
		return AlertRouteConfig{}, err
	}
	cfg := AlertRouteConfig{
		Enabled:        enabled,
		PollInterval:   pollInterval,
		RequestTimeout: requestTimeout,
		RetryBase:      retryBase,
		MaxAttempts:    maxAttempts,
		BatchSize:      batchSize,
	}
	if err := cfg.validate(); err != nil {
		return AlertRouteConfig{}, err
	}
	return cfg, nil
}

func (c AlertRouteConfig) validate() error {
	if c.PollInterval <= 0 || c.RequestTimeout <= 0 || c.RetryBase <= 0 {
		return fmt.Errorf("alert route intervals and timeouts must be positive")
	}
	return nil
}

func loadSignalConfig(environment string) (SignalConfig, error) {
	enabled, err := boolFromEnv("SIGNAL_ENABLED", false)
	if err != nil {
		return SignalConfig{}, err
	}
	overviewWindow, err := durationFromEnv("SIGNAL_OVERVIEW_WINDOW", 24*time.Hour)
	if err != nil {
		return SignalConfig{}, err
	}
	cleanupInterval, err := durationFromEnv("SIGNAL_CLEANUP_INTERVAL", time.Hour)
	if err != nil {
		return SignalConfig{}, err
	}
	retentionBatch, err := intFromEnv("SIGNAL_RETENTION_BATCH", 500, 1, 10000)
	if err != nil {
		return SignalConfig{}, err
	}
	listLimit, err := intFromEnv("SIGNAL_LIST_LIMIT", 100, 1, 200)
	if err != nil {
		return SignalConfig{}, err
	}
	overviewTopN, err := intFromEnv("SIGNAL_OVERVIEW_TOP_N", 10, 1, 100)
	if err != nil {
		return SignalConfig{}, err
	}
	cfg := SignalConfig{
		Enabled:         enabled,
		RetentionBatch:  retentionBatch,
		ListLimit:       listLimit,
		OverviewTopN:    overviewTopN,
		OverviewWindow:  overviewWindow,
		CleanupInterval: cleanupInterval,
	}
	if err := cfg.validate(); err != nil {
		return SignalConfig{}, err
	}
	return cfg, nil
}

func (c SignalConfig) validate() error {
	if c.RetentionBatch <= 0 || c.ListLimit <= 0 || c.OverviewTopN <= 0 {
		return fmt.Errorf("signal retention batch, list limit and overview top n must be positive")
	}
	if c.OverviewWindow <= 0 || c.CleanupInterval <= 0 {
		return fmt.Errorf("signal overview window and cleanup interval must be positive")
	}
	return nil
}
