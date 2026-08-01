package oidc

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testSigningKey is a 2048-bit RSA private key generated once for the test
// binary. A fresh JWK + JWT pair is derived from it for each test that needs a
// signed ID token. Using one key keeps the tests fast while still exercising
// the full RSA verification path.
type testSigningKey struct {
	private *rsa.PrivateKey
	jwk     JWK
	kid     string
}

func newTestSigningKey(t *testing.T, kid string) *testSigningKey {
	t.Helper()
	jwk, priv := rsaJWK(t, kid, "RS256")
	return &testSigningKey{private: priv, jwk: jwk, kid: kid}
}

// syntheticIdP is a minimal OIDC provider used to exercise the Authorization
// Code + PKCE flow end-to-end without an external IdP. It serves discovery,
// JWKS and the token endpoint, and records the authorization requests it
// receives so tests can assert on PKCE/state/nonce parameters.
type syntheticIdP struct {
	server         *httptest.Server
	signingKey     *testSigningKey
	issuer         string
	tokenCalls     int32
	authzCalls     int32
	jwksCalls      int32
	lastRequest    url.Values
	currentIDToken string
	// tokenBehavior controls how the token endpoint responds. Default is
	// "issue"; tests set it to "error" or "missing-id-token" to exercise
	// failure modes without re-registering routes.
	tokenBehavior string
}

func newSyntheticIdP(t *testing.T) *syntheticIdP {
	t.Helper()
	key := newTestSigningKey(t, "test-key-1")
	idp := &syntheticIdP{signingKey: key, tokenBehavior: "issue"}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", idp.handleDiscovery)
	mux.HandleFunc("/jwks", idp.handleJWKS)
	mux.HandleFunc("/token", idp.handleToken)
	server := newTestTLSServer(t, mux)
	idp.server = server
	idp.issuer = server.URL
	return idp
}

func (i *syntheticIdP) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(Discovery{
		Issuer:                           i.issuer,
		AuthorizationEndpoint:            i.issuer + "/authorize",
		TokenEndpoint:                    i.issuer + "/token",
		JWKSURI:                          i.issuer + "/jwks",
		EndSessionEndpoint:               i.issuer + "/logout",
		ResponseTypesSupported:           []string{"code"},
		CodeChallengeMethodsSupported:    []string{"S256"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ScopesSupported:                  []string{"openid", "profile"},
	})
}

func (i *syntheticIdP) handleJWKS(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&i.jwksCalls, 1)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(JWKS{Keys: []JWK{i.signingKey.jwk}})
}

// handleToken exchanges the authorization code for an ID token. The synthetic
// IdP does not validate the code or PKCE verifier (the provider under test
// does that); it returns the ID token set on the struct so tests can control
// the exact claims exercised by each case. tokenBehavior switches between the
// success path and forced failure modes used by the rejection tests.
func (i *syntheticIdP) handleToken(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt32(&i.tokenCalls, 1)
	switch i.tokenBehavior {
	case "error":
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	case "missing-id-token":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"token_type": "Bearer"})
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	i.lastRequest = r.PostForm
	if i.currentIDToken == "" {
		http.Error(w, "test setup error: no ID token configured", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id_token": i.currentIDToken, "token_type": "Bearer", "expires_in": 3600})
}

// SetIDToken configures the ID token returned by the next token endpoint call.
// Tests call it before driving HandleCallback so they control the exact claims
// exercised by each case.
func (i *syntheticIdP) SetIDToken(token string) { i.currentIDToken = token }

// signIDToken builds an ID token signed with the synthetic IdP key. Claims are
// merged with sane defaults (matching the configured claim mapping and MFA
// policy) so individual tests only override what they need. The issuer is
// always the synthetic IdP's issuer so the provider's issuer check passes.
func (i *syntheticIdP) signIDToken(t *testing.T, overrides map[string]any) string {
	t.Helper()
	return i.signIDTokenWithKey(t, i.signingKey, overrides)
}

// signIDTokenWithKey signs an ID token with the given signing key. Rotation
// E2E tests use it to sign tokens with a retired key so the provider's
// fail-closed path can be exercised after the JWKS cache refresh drops it.
func (i *syntheticIdP) signIDTokenWithKey(t *testing.T, signingKey *testSigningKey, overrides map[string]any) string {
	t.Helper()
	claims := jwt.MapClaims{
		"iss":                i.issuer,
		"sub":                "sub-123",
		"aud":                "aiops-platform",
		"preferred_username": "operator@example.com",
		"name":               "Platform Operator",
		"groups":             []string{"oidc-admins"},
		"acr":                "mfa",
		"nonce":              "nonce-456",
		"exp":                time.Now().Add(15 * time.Minute).Unix(),
		"iat":                time.Now().Unix(),
	}
	for name, value := range overrides {
		claims[name] = value
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = signingKey.kid
	signed, err := token.SignedString(signingKey.private)
	if err != nil {
		t.Fatalf("sign id token: %v", err)
	}
	return signed
}

// RotateKey generates a fresh signing key with the given kid and retires the
// current key. The JWKS endpoint publishes only the new key afterwards,
// mirroring provider key retirement. The retired key is returned so E2E
// rotation tests can sign tokens with it to assert they fail closed after the
// cache refresh drops them.
func (i *syntheticIdP) RotateKey(t *testing.T, kid string) *testSigningKey {
	t.Helper()
	retired := i.signingKey
	i.signingKey = newTestSigningKey(t, kid)
	return retired
}

// testProviderConfig returns a ProviderConfig wired to the synthetic IdP. The
// caller may mutate the returned config before calling NewProvider.
func (i *syntheticIdP) providerConfig() ProviderConfig {
	return ProviderConfig{
		Issuer:                   i.issuer,
		ClientID:                 "aiops-platform",
		RedirectURI:              "https://platform.example.com/auth/oidc/callback",
		RequiredScopes:           []string{"openid", "profile"},
		AllowedSigningAlgorithms: []string{"RS256"},
		ClaimMapping: ClaimMapping{
			Subject:     "sub",
			Username:    "preferred_username",
			DisplayName: "name",
			Groups:      "groups",
		},
		GroupToRoles: map[string][]string{
			"oidc-admins":  {"system_admin"},
			"oidc-viewers": {"viewer"},
		},
		MFA: MFAConfig{
			Required:       true,
			EvidenceClaim:  "acr",
			AcceptedValues: []string{"mfa", "otp"},
		},
		Sessions: SessionConfig{
			MaxAge:           8 * time.Hour,
			Reauthentication: time.Hour,
			RevokeOnDisable:  true,
		},
		JWKSCacheTTL:       time.Hour,
		JWKSRefreshTimeout: 5 * time.Second,
		SigningKey:         []byte("test-provider-signing-key-32-bytes!!!"),
	}
}

// newProviderWithSyntheticIdP constructs a Provider bound to the synthetic IdP
// and primes discovery + JWKS. The returned provider is ready to serve
// authorization and callback requests.
func newProviderWithSyntheticIdP(t *testing.T, idp *syntheticIdP) *Provider {
	t.Helper()
	cfg := idp.providerConfig()
	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider error = %v", err)
	}
	testClient := testHTTPClient(t, idp.server)
	// Inject the test HTTP client into every outbound path so the provider
	// trusts the self-signed certificate of the synthetic IdP. This mirrors
	// how discovery_test.go and jwks_test.go wire their caches.
	provider.discovery.client = testClient
	provider.client = testClient
	if err := provider.Init(context.Background()); err != nil {
		t.Fatalf("Init error = %v", err)
	}
	// Re-bind the JWKS cache to the test HTTP client so token verification can
	// reach the synthetic JWKS endpoint.
	provider.jwks.client = testClient
	return provider
}

func TestProviderConfigValidationRejectsInvalidConfigurations(t *testing.T) {
	base := ProviderConfig{
		Issuer:                   "https://idp.example.com",
		ClientID:                 "aiops-platform",
		RedirectURI:              "https://platform.example.com/auth/oidc/callback",
		RequiredScopes:           []string{"openid", "profile"},
		AllowedSigningAlgorithms: []string{"RS256"},
		ClaimMapping: ClaimMapping{
			Subject:     "sub",
			Username:    "preferred_username",
			DisplayName: "name",
			Groups:      "groups",
		},
		GroupToRoles: map[string][]string{"oidc-admins": {"system_admin"}},
		MFA: MFAConfig{
			Required:       true,
			EvidenceClaim:  "acr",
			AcceptedValues: []string{"mfa"},
		},
		SigningKey: []byte("test-provider-signing-key-32-bytes!!!"),
	}
	cases := map[string]func(*ProviderConfig){
		"non-sub subject":        func(c *ProviderConfig) { c.ClaimMapping.Subject = "user_id" },
		"missing username claim": func(c *ProviderConfig) { c.ClaimMapping.Username = "" },
		"missing display name":   func(c *ProviderConfig) { c.ClaimMapping.DisplayName = "" },
		"missing groups claim":   func(c *ProviderConfig) { c.ClaimMapping.Groups = "" },
		"no algorithms":          func(c *ProviderConfig) { c.AllowedSigningAlgorithms = nil },
		"no group mapping":       func(c *ProviderConfig) { c.GroupToRoles = nil },
		"mfa not required":       func(c *ProviderConfig) { c.MFA.Required = false },
		"bad evidence claim":     func(c *ProviderConfig) { c.MFA.EvidenceClaim = "sid" },
		"no accepted values":     func(c *ProviderConfig) { c.MFA.AcceptedValues = nil },
		"short signing key":      func(c *ProviderConfig) { c.SigningKey = []byte("short") },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := base
			mutate(&cfg)
			if _, err := NewProvider(cfg); err == nil {
				t.Fatalf("NewProvider error = nil, want validation error for %q", name)
			}
		})
	}
}

func TestAuthorizationURLContainsRequiredParameters(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)

	authzURL, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	if sessionToken == "" {
		t.Fatal("session token is empty")
	}
	parsed, err := url.Parse(authzURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	if parsed.Scheme != "https" {
		t.Fatalf("scheme = %q, want https", parsed.Scheme)
	}
	if parsed.Path != "/authorize" {
		t.Fatalf("path = %q, want /authorize", parsed.Path)
	}
	query := parsed.Query()
	required := map[string]string{
		"response_type":         "code",
		"client_id":             "aiops-platform",
		"redirect_uri":          "https://platform.example.com/auth/oidc/callback",
		"scope":                 "openid profile",
		"code_challenge_method": "S256",
	}
	for key, value := range required {
		if got := query.Get(key); got != value {
			t.Fatalf("query %q = %q, want %q", key, got, value)
		}
	}
	for _, key := range []string{"state", "nonce", "code_challenge"} {
		if query.Get(key) == "" {
			t.Fatalf("query %q is empty", key)
		}
	}
	// The session token must round-trip through the signer so the callback can
	// recover the state, nonce and code verifier.
	session, err := provider.signer.verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	if session.State != query.Get("state") {
		t.Fatalf("session state = %q, want %q", session.State, query.Get("state"))
	}
	if session.Nonce != query.Get("nonce") {
		t.Fatalf("session nonce = %q, want %q", session.Nonce, query.Get("nonce"))
	}
}

func TestHandleCallbackHappyPath(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)

	// Drive an authorization request to obtain a real session token (state,
	// nonce and PKCE verifier) so the callback can be exercised end-to-end.
	_, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := provider.signer.verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	idp.SetIDToken(idp.signIDToken(t, map[string]any{
		"nonce": session.Nonce,
	}))

	result, err := provider.HandleCallback(context.Background(), "auth-code", session.State, sessionToken)
	if err != nil {
		t.Fatalf("HandleCallback error = %v", err)
	}
	if result.Issuer != idp.issuer {
		t.Fatalf("Issuer = %q, want %q", result.Issuer, idp.issuer)
	}
	if result.Subject != "sub-123" {
		t.Fatalf("Subject = %q, want sub-123", result.Subject)
	}
	if result.Username != "operator@example.com" {
		t.Fatalf("Username = %q", result.Username)
	}
	if result.DisplayName != "Platform Operator" {
		t.Fatalf("DisplayName = %q", result.DisplayName)
	}
	if len(result.Groups) != 1 || result.Groups[0] != "oidc-admins" {
		t.Fatalf("Groups = %#v", result.Groups)
	}
	if len(result.Roles) != 1 || result.Roles[0] != "system_admin" {
		t.Fatalf("Roles = %#v", result.Roles)
	}
	if result.MFAEvidence != "mfa" {
		t.Fatalf("MFAEvidence = %q, want mfa", result.MFAEvidence)
	}
	if got := atomic.LoadInt32(&idp.tokenCalls); got != 1 {
		t.Fatalf("token endpoint called %d times, want 1", got)
	}
	// The token endpoint must receive the PKCE verifier and the authorization
	// code so the provider can complete the code exchange.
	if idp.lastRequest.Get("code") != "auth-code" {
		t.Fatalf("token request code = %q, want auth-code", idp.lastRequest.Get("code"))
	}
	if idp.lastRequest.Get("code_verifier") != session.CodeVerifier {
		t.Fatalf("token request code_verifier mismatch")
	}
	if idp.lastRequest.Get("client_id") != "aiops-platform" {
		t.Fatalf("token request client_id = %q", idp.lastRequest.Get("client_id"))
	}
}

func TestHandleCallbackRejectsMissingInputs(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)
	cases := map[string]struct {
		code, state, session string
	}{
		"missing code":          {"", "state", "session"},
		"missing state":         {"code", "", "session"},
		"missing session token": {"code", "state", ""},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := provider.HandleCallback(context.Background(), input.code, input.state, input.session); err == nil {
				t.Fatalf("HandleCallback error = nil, want input validation error for %q", name)
			}
		})
	}
}

func TestHandleCallbackRejectsStateMismatch(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)
	_, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	_, err = provider.HandleCallback(context.Background(), "code", "wrong-state", sessionToken)
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("HandleCallback error = %v, want state mismatch", err)
	}
}

func TestHandleCallbackRejectsInvalidSessionToken(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)
	_, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := provider.signer.verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	if _, err := provider.HandleCallback(context.Background(), "code", session.State, "not-a-valid-session-token"); err == nil {
		t.Fatal("HandleCallback error = nil, want session token error")
	}
}

func TestHandleCallbackRejectsTokenEndpointError(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)
	idp.tokenBehavior = "error"
	_, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := provider.signer.verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	_, err = provider.HandleCallback(context.Background(), "code", session.State, sessionToken)
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("HandleCallback error = %v, want status 500", err)
	}
}

func TestHandleCallbackRejectsMissingIDToken(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)
	idp.tokenBehavior = "missing-id-token"
	_, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := provider.signer.verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	_, err = provider.HandleCallback(context.Background(), "code", session.State, sessionToken)
	if err == nil || !strings.Contains(err.Error(), "missing id_token") {
		t.Fatalf("HandleCallback error = %v, want missing id_token", err)
	}
}

func TestVerifyIDTokenRejectsContractViolations(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)
	_, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := provider.signer.verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}

	cases := map[string]map[string]any{
		"wrong issuer":        {"iss": "https://other.example.com"},
		"wrong audience":      {"aud": "other-client"},
		"wrong nonce":         {"nonce": "different-nonce"},
		"expired token":       {"exp": time.Now().Add(-time.Minute).Unix()},
		"missing mfa":         {"acr": ""},
		"unaccepted mfa":      {"acr": "pwd"},
		"missing sub":         {"sub": ""},
		"missing username":    {"preferred_username": ""},
		"missing displayname": {"name": ""},
		"no role mapping":     {"groups": []string{"unknown-group"}},
	}
	for name, overrides := range cases {
		t.Run(name, func(t *testing.T) {
			idp.SetIDToken(idp.signIDToken(t, overrides))
			_, err := provider.HandleCallback(context.Background(), "code", session.State, sessionToken)
			if err == nil {
				t.Fatalf("HandleCallback error = nil, want failure for %q", name)
			}
		})
	}
}

func TestVerifyIDTokenRejectsDisallowedAlgorithm(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)
	_, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := provider.signer.verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	// Sign a token with HS256 (symmetric) which the provider must reject.
	claims := jwt.MapClaims{
		"iss":                idp.issuer,
		"sub":                "sub-123",
		"aud":                "aiops-platform",
		"preferred_username": "operator@example.com",
		"name":               "Platform Operator",
		"groups":             []string{"oidc-admins"},
		"acr":                "mfa",
		"nonce":              session.Nonce,
		"exp":                time.Now().Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = "test-key-1"
	signed, err := token.SignedString([]byte("shared-secret"))
	if err != nil {
		t.Fatalf("sign hs256 token: %v", err)
	}
	idp.SetIDToken(signed)
	_, err = provider.HandleCallback(context.Background(), "code", session.State, sessionToken)
	if err == nil {
		t.Fatal("HandleCallback error = nil, want disallowed algorithm error")
	}
}

func TestVerifyIDTokenRejectsUnknownKeyID(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)
	_, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := provider.signer.verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	// Sign with the test key but advertise an unknown kid. The provider must
	// fail closed because the JWKS does not publish that kid.
	claims := jwt.MapClaims{
		"iss":                idp.issuer,
		"sub":                "sub-123",
		"aud":                "aiops-platform",
		"preferred_username": "operator@example.com",
		"name":               "Platform Operator",
		"groups":             []string{"oidc-admins"},
		"acr":                "mfa",
		"nonce":              session.Nonce,
		"exp":                time.Now().Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "unknown-kid"
	signed, err := token.SignedString(idp.signingKey.private)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	idp.SetIDToken(signed)
	_, err = provider.HandleCallback(context.Background(), "code", session.State, sessionToken)
	if err == nil {
		t.Fatal("HandleCallback error = nil, want unknown key error")
	}
}

func TestVerifyIDTokenAcceptsAMREvidence(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)
	// Switch the provider to amr-based MFA evidence.
	provider.config.MFA.EvidenceClaim = "amr"
	provider.config.MFA.AcceptedValues = []string{"otp"}

	_, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := provider.signer.verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	idp.SetIDToken(idp.signIDToken(t, map[string]any{
		"nonce": session.Nonce,
		"acr":   "",
		"amr":   []any{"otp"},
	}))
	result, err := provider.HandleCallback(context.Background(), "code", session.State, sessionToken)
	if err != nil {
		t.Fatalf("HandleCallback error = %v", err)
	}
	if result.MFAEvidence != "otp" {
		t.Fatalf("MFAEvidence = %q, want otp", result.MFAEvidence)
	}
}

func TestMapGroupsToRolesDeduplicatesAndSorts(t *testing.T) {
	provider := &Provider{config: ProviderConfig{
		GroupToRoles: map[string][]string{
			"admins":  {"system_admin", "operations_admin"},
			"ops":     {"operations_admin"},
			"viewers": {"viewer"},
		},
	}}
	roles := provider.mapGroupsToRoles([]string{"viewers", "admins", "ops", "unknown"})
	want := []string{"operations_admin", "system_admin", "viewer"}
	if len(roles) != len(want) {
		t.Fatalf("roles = %#v, want %#v", roles, want)
	}
	for i, r := range roles {
		if r != want[i] {
			t.Fatalf("roles[%d] = %q, want %q", i, r, want[i])
		}
	}
}

func TestMapGroupsToRolesEmptyWhenNoGroupMatches(t *testing.T) {
	provider := &Provider{config: ProviderConfig{
		GroupToRoles: map[string][]string{"admins": {"system_admin"}},
	}}
	if roles := provider.mapGroupsToRoles([]string{"unknown"}); len(roles) != 0 {
		t.Fatalf("roles = %#v, want empty", roles)
	}
}

func TestExtractGroupsHandlesStringAndArray(t *testing.T) {
	claims := jwt.MapClaims{"groups": "single-group"}
	if groups := extractGroups(claims, "groups"); len(groups) != 1 || groups[0] != "single-group" {
		t.Fatalf("string groups = %#v", groups)
	}
	claims["groups"] = []any{"g1", "g2", ""}
	if groups := extractGroups(claims, "groups"); len(groups) != 2 || groups[0] != "g1" || groups[1] != "g2" {
		t.Fatalf("array groups = %#v", groups)
	}
	claims["groups"] = 42
	if groups := extractGroups(claims, "groups"); len(groups) != 0 {
		t.Fatalf("non-string/array groups = %#v, want empty", groups)
	}
	claims["groups"] = ""
	if groups := extractGroups(claims, "groups"); len(groups) != 0 {
		t.Fatalf("empty string groups = %#v, want empty", groups)
	}
}

func TestStringClaimHandlesStringAndNumber(t *testing.T) {
	claims := jwt.MapClaims{"name": "Alice"}
	if got := stringClaim(claims, "name"); got != "Alice" {
		t.Fatalf("stringClaim string = %q", got)
	}
	claims["name"] = float64(42)
	if got := stringClaim(claims, "name"); got != "42" {
		t.Fatalf("stringClaim number = %q, want 42", got)
	}
	claims["name"] = nil
	if got := stringClaim(claims, "name"); got != "" {
		t.Fatalf("stringClaim nil = %q, want empty", got)
	}
}

func TestProviderInitFetchesDiscoveryAndBindsJWKS(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)
	if provider.jwks == nil {
		t.Fatal("JWKS cache not bound after Init")
	}
	if got := atomic.LoadInt32(&idp.jwksCalls); got != 0 {
		t.Fatalf("jwks called %d times during Init, want 0 (lazy fetch)", got)
	}
	// A KeyByID call triggers the first JWKS fetch.
	if _, _, err := provider.keyByID(context.Background(), "test-key-1"); err != nil {
		t.Fatalf("KeyByID error = %v", err)
	}
	if got := atomic.LoadInt32(&idp.jwksCalls); got != 1 {
		t.Fatalf("jwks called %d times, want 1", got)
	}
}

func TestProviderRejectsClientSecretInBrowserFlow(t *testing.T) {
	// The provider never sends the client secret in the authorization URL; it
	// is only used at the token endpoint. This test guards against regressions
	// where the secret might leak through the front-channel.
	idp := newSyntheticIdP(t)
	cfg := idp.providerConfig()
	cfg.ClientSecret = "production-confidential-client-secret"
	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider error = %v", err)
	}
	provider.discovery.client = testHTTPClient(t, idp.server)
	if err := provider.Init(context.Background()); err != nil {
		t.Fatalf("Init error = %v", err)
	}
	provider.jwks.client = testHTTPClient(t, idp.server)

	authzURL, _, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	if strings.Contains(authzURL, "client_secret") {
		t.Fatalf("authorization URL must not contain client_secret: %s", authzURL)
	}
	if strings.Contains(authzURL, "production-confidential-client-secret") {
		t.Fatalf("authorization URL leaks client secret: %s", authzURL)
	}
}

func TestProviderSendsClientSecretAtTokenEndpoint(t *testing.T) {
	idp := newSyntheticIdP(t)
	cfg := idp.providerConfig()
	cfg.ClientSecret = "production-confidential-client-secret"
	provider, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider error = %v", err)
	}
	testClient := testHTTPClient(t, idp.server)
	provider.discovery.client = testClient
	provider.client = testClient
	if err := provider.Init(context.Background()); err != nil {
		t.Fatalf("Init error = %v", err)
	}
	provider.jwks.client = testClient

	_, sessionToken, err := provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := provider.signer.verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	idp.SetIDToken(idp.signIDToken(t, map[string]any{"nonce": session.Nonce}))
	if _, err := provider.HandleCallback(context.Background(), "code", session.State, sessionToken); err != nil {
		t.Fatalf("HandleCallback error = %v", err)
	}
	if idp.lastRequest.Get("client_secret") != "production-confidential-client-secret" {
		t.Fatalf("token request client_secret = %q, want the configured secret", idp.lastRequest.Get("client_secret"))
	}
}
