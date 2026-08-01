package oidc

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ClaimMapping binds immutable OIDC claims to local account attributes.
type ClaimMapping struct {
	Subject     string
	Username    string
	DisplayName string
	Groups      string
}

// MFAConfig requires identity-provider-enforced MFA with accepted evidence.
type MFAConfig struct {
	Required       bool
	EvidenceClaim  string
	AcceptedValues []string
}

// SessionConfig bounds the local OIDC session lifetime.
type SessionConfig struct {
	MaxAge           time.Duration
	Reauthentication time.Duration
	RevokeOnDisable  bool
}

// ProviderConfig adapts config.OIDCConfig into the runtime provider parameters.
// SigningKey signs the short-lived auth-session cookie that carries state,
// nonce and the PKCE code verifier.
type ProviderConfig struct {
	Issuer                   string
	ClientID                 string
	ClientSecret             string
	RedirectURI              string
	RequiredScopes           []string
	AllowedSigningAlgorithms []string
	ClaimMapping             ClaimMapping
	GroupToRoles             map[string][]string
	MFA                      MFAConfig
	Sessions                 SessionConfig
	JWKSCacheTTL             time.Duration
	JWKSRefreshTimeout       time.Duration
	SigningKey               []byte
}

// CallbackResult is the verified outcome of an OIDC Authorization Code
// callback. The caller resolves Subject+Issuer to a prelinked local user
// (external_identities) and issues a local session.
type CallbackResult struct {
	Issuer      string
	Subject     string
	Username    string
	DisplayName string
	Groups      []string
	Roles       []string
	MFAEvidence string
}

// Provider orchestrates the OIDC Authorization Code + PKCE S256 flow against a
// single approved issuer. It is safe for concurrent use after Init.
type Provider struct {
	config    ProviderConfig
	discovery *DiscoveryFetcher
	jwks      *JWKSCache
	client    *http.Client
	signer    *authSessionSigner
	now       func() time.Time
}

// NewProvider constructs a provider bound to the approved configuration. Init
// must be called once before AuthorizationURL or HandleCallback to fetch
// discovery and bind the JWKS cache.
func NewProvider(cfg ProviderConfig) (*Provider, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	timeout := cfg.JWKSRefreshTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	discovery := NewDiscoveryFetcher(DiscoveryConfig{
		Issuer:                   cfg.Issuer,
		ClientID:                 cfg.ClientID,
		RequiredScopes:           cfg.RequiredScopes,
		AllowedSigningAlgorithms: cfg.AllowedSigningAlgorithms,
		FetchTimeout:             timeout,
		CacheTTL:                 cfg.JWKSCacheTTL,
	})
	return &Provider{
		config:    cfg,
		discovery: discovery,
		client:    newBoundedHTTPClient(timeout),
		signer:    newAuthSessionSigner(cfg.SigningKey),
		now:       time.Now,
	}, nil
}

func (c ProviderConfig) validate() error {
	if c.ClaimMapping.Subject != "sub" {
		return fmt.Errorf("oidc: claim mapping subject must be \"sub\"")
	}
	if c.ClaimMapping.Username == "" || c.ClaimMapping.DisplayName == "" || c.ClaimMapping.Groups == "" {
		return fmt.Errorf("oidc: username, display_name and groups claim mappings are required")
	}
	if len(c.AllowedSigningAlgorithms) == 0 {
		return fmt.Errorf("oidc: at least one signing algorithm is required")
	}
	if len(c.GroupToRoles) == 0 {
		return fmt.Errorf("oidc: at least one group-to-role mapping is required")
	}
	if !c.MFA.Required || (c.MFA.EvidenceClaim != "acr" && c.MFA.EvidenceClaim != "amr") || len(c.MFA.AcceptedValues) == 0 {
		return fmt.Errorf("oidc: MFA must be required with an acr/amr evidence claim and accepted values")
	}
	if len(c.SigningKey) < 32 {
		return fmt.Errorf("oidc: signing key must be at least 32 bytes")
	}
	return nil
}

// Init fetches the discovery document and binds the JWKS cache to the
// discovered jwks_uri. It must be called once before serving OIDC requests.
func (p *Provider) Init(ctx context.Context) error {
	discovery, err := p.discovery.Fetch(ctx)
	if err != nil {
		return err
	}
	p.jwks = NewJWKSCache(JWKSCacheConfig{
		JWKSURI:                  discovery.JWKSURI,
		AllowedSigningAlgorithms: p.config.AllowedSigningAlgorithms,
		FetchTimeout:             p.config.JWKSRefreshTimeout,
		CacheTTL:                 p.config.JWKSCacheTTL,
	})
	return nil
}

// AuthorizationURL builds the provider authorization endpoint URL with PKCE
// S256, state and nonce, and returns a signed auth-session token that the
// caller must persist as a short-lived cookie. The user agent is redirected to
// the returned URL.
func (p *Provider) AuthorizationURL(ctx context.Context) (string, string, error) {
	discovery, err := p.discovery.Fetch(ctx)
	if err != nil {
		return "", "", err
	}
	verifier, challenge, err := GenerateCodeVerifier()
	if err != nil {
		return "", "", err
	}
	state, err := randomString(32)
	if err != nil {
		return "", "", err
	}
	nonce, err := randomString(32)
	if err != nil {
		return "", "", err
	}
	sessionToken, err := p.signer.issue(state, nonce, verifier)
	if err != nil {
		return "", "", err
	}
	scopes := p.config.RequiredScopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile"}
	}
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", p.config.ClientID)
	params.Set("redirect_uri", p.config.RedirectURI)
	params.Set("scope", strings.Join(scopes, " "))
	params.Set("state", state)
	params.Set("nonce", nonce)
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	return discovery.AuthorizationEndpoint + "?" + params.Encode(), sessionToken, nil
}

// HandleCallback exchanges the authorization code for an ID token, verifies the
// ID token (signature, issuer, audience, nonce, expiry, MFA evidence) and
// returns the normalized claims. It fails closed on any mismatch.
func (p *Provider) HandleCallback(ctx context.Context, code, state, sessionToken string) (CallbackResult, error) {
	if code == "" || state == "" || sessionToken == "" {
		return CallbackResult{}, fmt.Errorf("oidc: code, state and session token are required")
	}
	session, err := p.signer.verify(sessionToken)
	if err != nil {
		return CallbackResult{}, err
	}
	if session.State != state {
		return CallbackResult{}, fmt.Errorf("oidc: state mismatch")
	}
	discovery, err := p.discovery.Fetch(ctx)
	if err != nil {
		return CallbackResult{}, err
	}
	idToken, err := p.exchangeCode(ctx, discovery.TokenEndpoint, code, session.CodeVerifier)
	if err != nil {
		return CallbackResult{}, err
	}
	return p.verifyIDToken(ctx, idToken, session.Nonce)
}

// AuthSessionCookieName is the cookie name carrying the signed auth session.
const AuthSessionCookieName = "oidc_auth_session"

func (p *Provider) exchangeCode(ctx context.Context, tokenEndpoint, code, codeVerifier string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", p.config.RedirectURI)
	form.Set("client_id", p.config.ClientID)
	form.Set("code_verifier", codeVerifier)
	if p.config.ClientSecret != "" {
		form.Set("client_secret", p.config.ClientSecret)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("oidc: token exchange failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("oidc: token endpoint returned status %d", response.StatusCode)
	}
	var tokenResponse struct {
		IDToken string `json:"id_token"`
	}
	if err := decodeLimitedJSON(response, &tokenResponse); err != nil {
		return "", err
	}
	if tokenResponse.IDToken == "" {
		return "", fmt.Errorf("oidc: token response missing id_token")
	}
	return tokenResponse.IDToken, nil
}

// verifyIDToken validates the ID token signature, issuer, audience, nonce,
// expiry and MFA evidence, then extracts the configured claims and maps groups
// to platform roles.
func (p *Provider) verifyIDToken(ctx context.Context, idToken, expectedNonce string) (CallbackResult, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(idToken, &claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("oidc: id token header missing kid")
		}
		key, _, err := p.jwks.KeyByID(ctx, kid)
		return key, err
	},
		jwt.WithValidMethods(p.config.AllowedSigningAlgorithms),
		jwt.WithIssuer(p.config.Issuer),
		jwt.WithAudience(p.config.ClientID),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return CallbackResult{}, fmt.Errorf("oidc: id token invalid: %w", err)
	}

	nonce, _ := claims["nonce"].(string)
	if nonce == "" || nonce != expectedNonce {
		return CallbackResult{}, fmt.Errorf("oidc: nonce mismatch")
	}

	if err := p.verifyMFA(claims); err != nil {
		return CallbackResult{}, err
	}

	subject, _ := claims["sub"].(string)
	if subject == "" {
		return CallbackResult{}, fmt.Errorf("oidc: id token missing sub claim")
	}
	username := stringClaim(claims, p.config.ClaimMapping.Username)
	displayName := stringClaim(claims, p.config.ClaimMapping.DisplayName)
	if username == "" || displayName == "" {
		return CallbackResult{}, fmt.Errorf("oidc: id token missing required claims")
	}
	groups := extractGroups(claims, p.config.ClaimMapping.Groups)
	roles := p.mapGroupsToRoles(groups)
	if len(roles) == 0 {
		return CallbackResult{}, fmt.Errorf("oidc: id token groups do not map to any platform role")
	}
	evidence := p.mfaEvidence(claims)
	return CallbackResult{
		Issuer:      p.config.Issuer,
		Subject:     subject,
		Username:    username,
		DisplayName: displayName,
		Groups:      groups,
		Roles:       roles,
		MFAEvidence: evidence,
	}, nil
}

func (p *Provider) verifyMFA(claims jwt.MapClaims) error {
	if !p.config.MFA.Required {
		return nil
	}
	evidence := p.mfaEvidence(claims)
	if evidence == "" {
		return fmt.Errorf("oidc: id token missing required MFA evidence")
	}
	if !slices.Contains(p.config.MFA.AcceptedValues, evidence) {
		return fmt.Errorf("oidc: id token MFA evidence %q is not accepted", evidence)
	}
	return nil
}

func (p *Provider) mfaEvidence(claims jwt.MapClaims) string {
	switch p.config.MFA.EvidenceClaim {
	case "acr":
		value, _ := claims["acr"].(string)
		return value
	case "amr":
		values, _ := claims["amr"].([]any)
		for _, value := range values {
			if s, ok := value.(string); ok && slices.Contains(p.config.MFA.AcceptedValues, s) {
				return s
			}
		}
	}
	return ""
}

func (p *Provider) mapGroupsToRoles(groups []string) []string {
	seen := make(map[string]struct{})
	roles := make([]string, 0)
	for _, group := range groups {
		mapped, ok := p.config.GroupToRoles[group]
		if !ok {
			continue
		}
		for _, role := range mapped {
			if _, dup := seen[role]; dup {
				continue
			}
			seen[role] = struct{}{}
			roles = append(roles, role)
		}
	}
	slices.Sort(roles)
	return roles
}

func stringClaim(claims jwt.MapClaims, name string) string {
	switch value := claims[name].(type) {
	case string:
		return value
	case float64:
		return fmt.Sprintf("%v", value)
	default:
		return ""
	}
}

func extractGroups(claims jwt.MapClaims, name string) []string {
	switch value := claims[name].(type) {
	case string:
		if value == "" {
			return nil
		}
		return []string{value}
	case []any:
		groups := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok && s != "" {
				groups = append(groups, s)
			}
		}
		return groups
	default:
		return nil
	}
}

// keyByID is exported for tests that need to assert the JWKS cache is wired.
func (p *Provider) keyByID(ctx context.Context, kid string) (crypto.PublicKey, string, error) {
	return p.jwks.KeyByID(ctx, kid)
}

// ErrProviderNotInitialized is returned when Init has not been called.
var ErrProviderNotInitialized = errors.New("oidc: provider not initialized")
