package oidc

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Discovery is the subset of the OIDC discovery document the runtime consumes.
// Field shapes mirror identityreadiness.Discovery so the offline gate and the
// runtime agree on what a captured/advertised contract looks like.
type Discovery struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	JWKSURI                          string   `json:"jwks_uri"`
	EndSessionEndpoint               string   `json:"end_session_endpoint"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	CodeChallengeMethodsSupported    []string `json:"code_challenge_methods_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ScopesSupported                  []string `json:"scopes_supported"`
}

// DiscoveryFetcher fetches and validates the OIDC discovery document for a
// configured provider. It enforces the same fail-closed contract as the
// offline readiness gate (ADR 0032) but over HTTPS at runtime.
type DiscoveryFetcher struct {
	client  *http.Client
	config  oidcProviderConfig
	now     func() time.Time
	cached  *Discovery
	expires time.Time
}

// oidcProviderConfig is the subset of config.OIDCConfig the runtime needs. It
// is a small local interface so the oidc package does not import config
// (which would risk a cycle once config wires the provider). Callers adapt
// config.OIDCConfig into this shape.
type oidcProviderConfig struct {
	issuer                   string
	clientID                 string
	requiredScopes           []string
	allowedSigningAlgorithms []string
	fetchTimeout             time.Duration
	cacheTTL                 time.Duration
}

// DiscoveryConfig adapts the platform configuration into the runtime provider
// configuration. Build it from config.OIDCConfig.
type DiscoveryConfig struct {
	Issuer                   string
	ClientID                 string
	RequiredScopes           []string
	AllowedSigningAlgorithms []string
	FetchTimeout             time.Duration
	CacheTTL                 time.Duration
}

// NewDiscoveryFetcher constructs a fetcher bound to the approved issuer and
// algorithms. FetchTimeout bounds each HTTP call; CacheTTL bounds how long a
// fetched discovery document is reused before a mandatory refresh.
func NewDiscoveryFetcher(cfg DiscoveryConfig) *DiscoveryFetcher {
	timeout := cfg.FetchTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ttl := cfg.CacheTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &DiscoveryFetcher{
		client: newBoundedHTTPClient(timeout),
		config: oidcProviderConfig{
			issuer:                   strings.TrimRight(cfg.Issuer, "/"),
			clientID:                 cfg.ClientID,
			requiredScopes:           cfg.RequiredScopes,
			allowedSigningAlgorithms: cfg.AllowedSigningAlgorithms,
			fetchTimeout:             timeout,
			cacheTTL:                 ttl,
		},
		now: time.Now,
	}
}

// wellKnownURL derives the discovery URL from the canonical issuer. The issuer
// must not already contain a path ending in .well-known; per OIDC spec the
// document is at issuer + "/.well-known/openid-configuration".
func (f *DiscoveryFetcher) wellKnownURL() string {
	return f.config.issuer + "/.well-known/openid-configuration"
}

// Fetch returns a validated discovery document, refreshing the cached copy when
// it has expired. The first call always hits the provider; subsequent calls
// within the cache TTL return the cached document without a network request.
func (f *DiscoveryFetcher) Fetch(ctx context.Context) (Discovery, error) {
	if f.cached != nil && f.now().Before(f.expires) {
		return *f.cached, nil
	}
	discovery, err := f.fetchFromProvider(ctx)
	if err != nil {
		// Serve a stale-but-unexpired copy is already handled above. If the
		// cache is expired we never serve stale discovery: a rotation or
		// contract change must be observed promptly.
		return Discovery{}, err
	}
	if err := f.Validate(discovery); err != nil {
		return Discovery{}, err
	}
	f.cached = &discovery
	f.expires = f.now().Add(f.config.cacheTTL)
	return discovery, nil
}

func (f *DiscoveryFetcher) fetchFromProvider(ctx context.Context) (Discovery, error) {
	if err := requireHTTPSURL(f.config.issuer); err != nil {
		return Discovery{}, err
	}
	var discovery Discovery
	if err := fetchJSON(ctx, f.client, f.wellKnownURL(), &discovery); err != nil {
		return Discovery{}, err
	}
	return discovery, nil
}

// Validate applies the fail-closed runtime contract to a fetched discovery
// document. It mirrors the offline readiness checks for issuer, endpoints,
// authorization flow, scopes and signing algorithms.
func (f *DiscoveryFetcher) Validate(discovery Discovery) error {
	if discovery.Issuer != f.config.issuer {
		return fmt.Errorf("oidc: discovery issuer %q does not match approved issuer %q", discovery.Issuer, f.config.issuer)
	}
	if err := requireHTTPSURL(discovery.AuthorizationEndpoint); err != nil {
		return fmt.Errorf("oidc: authorization endpoint invalid: %w", err)
	}
	if err := requireHTTPSURL(discovery.TokenEndpoint); err != nil {
		return fmt.Errorf("oidc: token endpoint invalid: %w", err)
	}
	if err := requireHTTPSURL(discovery.JWKSURI); err != nil {
		return fmt.Errorf("oidc: jwks_uri invalid: %w", err)
	}
	if err := requireHTTPSURL(discovery.EndSessionEndpoint); err != nil {
		return fmt.Errorf("oidc: end_session_endpoint invalid: %w", err)
	}
	if !slices.Contains(discovery.ResponseTypesSupported, "code") {
		return fmt.Errorf("oidc: discovery must advertise the authorization code response type")
	}
	if !slices.Contains(discovery.CodeChallengeMethodsSupported, "S256") {
		return fmt.Errorf("oidc: discovery must support PKCE S256")
	}
	for _, scope := range f.config.requiredScopes {
		if !slices.Contains(discovery.ScopesSupported, scope) {
			return fmt.Errorf("oidc: discovery does not advertise required scope %q", scope)
		}
	}
	for _, algorithm := range f.config.allowedSigningAlgorithms {
		if !slices.Contains(discovery.IDTokenSigningAlgValuesSupported, algorithm) {
			return fmt.Errorf("oidc: discovery does not advertise approved signing algorithm %q", algorithm)
		}
	}
	return nil
}

// Invalidate forces the next Fetch to hit the provider. It is used after a
// key-rotation event or for testing.
func (f *DiscoveryFetcher) Invalidate() {
	f.cached = nil
	f.expires = time.Time{}
}
