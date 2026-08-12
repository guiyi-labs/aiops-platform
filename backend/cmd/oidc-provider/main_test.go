package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testProvider starts an in-process IdP on a throwaway TLS listener and
// returns the provider plus an HTTP client that trusts its certificate and
// does not follow redirects (the drill extracts the Location header).
func testProvider(t *testing.T) (*localIdP, *httptest.Server, *http.Client) {
	t.Helper()
	idp := newLocalIdP("", "aiops-platform", "https://app.example/api/v1/auth/oidc/callback")
	server := httptest.NewTLSServer(idp.handler())
	t.Cleanup(server.Close)
	idp.issuer = server.URL
	client := server.Client()
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return idp, server, client
}

const (
	testRedirectURI = "https://app.example/api/v1/auth/oidc/callback"
	testVerifier    = "drill-verifier-pkce-test-123456"
)

func authorizeURL(serverURL, state, nonce, extra string) string {
	values := url.Values{
		"response_type":         {"code"},
		"client_id":             {"aiops-platform"},
		"redirect_uri":          {testRedirectURI},
		"scope":                 {"openid profile"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {s256Challenge(testVerifier)},
		"code_challenge_method": {"S256"},
		"user":                  {"sub-123"},
		"username":              {"operator@drill.local"},
		"display_name":          {"Drill Operator"},
		"groups":                {"platform-operators"},
		"acr":                   {"phr"},
	}
	if extra != "" {
		values.Set("fail", extra)
	}
	return serverURL + "/authorize?" + values.Encode()
}

// authorize drives /authorize and returns the code from the 302 Location.
func authorize(t *testing.T, client *http.Client, serverURL, state, nonce, extra string) string {
	t.Helper()
	response, err := client.Get(authorizeURL(serverURL, state, nonce, extra))
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("authorize status = %d, want 302", response.StatusCode)
	}
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse Location %q: %v", response.Header.Get("Location"), err)
	}
	if location.Query().Get("state") != state {
		t.Fatalf("echoed state = %q, want %q", location.Query().Get("state"), state)
	}
	return location.Query().Get("code")
}

func tokenForm(code string) url.Values {
	return url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {"aiops-platform"},
		"code_verifier": {testVerifier},
	}
}

// exchange posts to /token and returns the raw ID token plus the parsed
// claims and header.
func exchange(t *testing.T, client *http.Client, serverURL, code string, form url.Values) (string, jwt.MapClaims, map[string]any) {
	t.Helper()
	response, err := client.PostForm(serverURL+"/token", form)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	defer response.Body.Close()
	var body struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("token status = %d, want 200", response.StatusCode)
	}
	claims := jwt.MapClaims{}
	parsed, _, err := new(jwt.Parser).ParseUnverified(body.IDToken, claims)
	if err != nil {
		t.Fatalf("parse id_token: %v", err)
	}
	return body.IDToken, claims, parsed.Header
}

func TestLocalIdPHappyPath(t *testing.T) {
	_, server, client := testProvider(t)

	discoveryResponse, err := client.Get(server.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	defer discoveryResponse.Body.Close()
	var discovery map[string]any
	if err := json.NewDecoder(discoveryResponse.Body).Decode(&discovery); err != nil {
		t.Fatalf("decode discovery: %v", err)
	}
	for endpoint, want := range map[string]any{
		"issuer":                                server.URL,
		"authorization_endpoint":                server.URL + "/authorize",
		"token_endpoint":                        server.URL + "/token",
		"jwks_uri":                              server.URL + "/jwks",
		"end_session_endpoint":                  server.URL + "/logout",
		"response_types_supported":              []any{"code"},
		"id_token_signing_alg_values_supported": []any{"RS256"},
	} {
		if got := discovery[endpoint]; fmtJSON(got) != fmtJSON(want) {
			t.Fatalf("discovery %s = %v, want %v", endpoint, got, want)
		}
	}

	code := authorize(t, client, server.URL, "state-1", "nonce-1", "")
	_, claims, header := exchange(t, client, server.URL, code, tokenForm(code))
	if header["alg"] != "RS256" || header["kid"] != currentKeyID {
		t.Fatalf("header = %v, want RS256 / %s", header, currentKeyID)
	}
	for claim, want := range map[string]any{
		"iss":                server.URL,
		"sub":                "sub-123",
		"aud":                "aiops-platform",
		"nonce":              "nonce-1",
		"acr":                "phr",
		"preferred_username": "operator@drill.local",
	} {
		if claims[claim] != want {
			t.Fatalf("claim %s = %v, want %v", claim, claims[claim], want)
		}
	}
	groups, ok := claims["groups"].([]any)
	if !ok || len(groups) != 1 || groups[0] != "platform-operators" {
		t.Fatalf("groups claim = %#v, want [platform-operators]", claims["groups"])
	}
	exp, ok := claims["exp"].(float64)
	if !ok || exp < float64(time.Now().Add(9*time.Minute).Unix()) {
		t.Fatalf("exp = %v, want ~10 minutes in the future", claims["exp"])
	}
}

func TestLocalIdPSingleUseAndPKCEEnforcement(t *testing.T) {
	_, server, client := testProvider(t)

	code := authorize(t, client, server.URL, "state-1", "nonce-1", "")
	exchange(t, client, server.URL, code, tokenForm(code))

	replayResponse, err := client.PostForm(server.URL+"/token", tokenForm(code))
	if err != nil {
		t.Fatalf("replay token: %v", err)
	}
	defer replayResponse.Body.Close()
	if replayResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("replay status = %d, want 400", replayResponse.StatusCode)
	}

	mismatchForm := tokenForm(authorize(t, client, server.URL, "state-2", "nonce-2", ""))
	mismatchForm.Set("code_verifier", "not-the-right-verifier")
	mismatchResponse, err := client.PostForm(server.URL+"/token", mismatchForm)
	if err != nil {
		t.Fatalf("mismatch token: %v", err)
	}
	defer mismatchResponse.Body.Close()
	if mismatchResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("PKCE mismatch status = %d, want 400", mismatchResponse.StatusCode)
	}

	// The /authorize endpoint must reject a missing PKCE challenge.
	badURL := server.URL + "/authorize?" + "response_type=code&client_id=aiops-platform&redirect_uri=" +
		url.QueryEscape(testRedirectURI) + "&scope=openid%20profile&state=state-3&nonce=nonce-3"
	badResponse, err := client.Get(badURL)
	if err != nil {
		t.Fatalf("missing challenge: %v", err)
	}
	defer badResponse.Body.Close()
	if badResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing challenge status = %d, want 400", badResponse.StatusCode)
	}
}

func TestLocalIdPFailModes(t *testing.T) {
	cases := []struct {
		name   string
		extra  string
		assert func(t *testing.T, claims jwt.MapClaims, header map[string]any)
	}{
		{
			name:  "wrong_nonce",
			extra: "wrong_nonce",
			assert: func(t *testing.T, claims jwt.MapClaims, _ map[string]any) {
				if claims["nonce"] != "attacker-controlled-nonce" {
					t.Fatalf("nonce = %v, want attacker-controlled-nonce", claims["nonce"])
				}
			},
		},
		{
			name:  "omit_nonce",
			extra: "omit_nonce",
			assert: func(t *testing.T, claims jwt.MapClaims, _ map[string]any) {
				if _, ok := claims["nonce"]; ok {
					t.Fatalf("nonce present, want omitted")
				}
			},
		},
		{
			name:  "wrong_issuer",
			extra: "wrong_issuer",
			assert: func(t *testing.T, claims jwt.MapClaims, _ map[string]any) {
				if claims["iss"] != "https://evil.invalid" {
					t.Fatalf("iss = %v, want https://evil.invalid", claims["iss"])
				}
			},
		},
		{
			name:  "wrong_audience",
			extra: "wrong_aud",
			assert: func(t *testing.T, claims jwt.MapClaims, _ map[string]any) {
				if claims["aud"] != "some-other-client" {
					t.Fatalf("aud = %v, want some-other-client", claims["aud"])
				}
			},
		},
		{
			name:  "no_mfa",
			extra: "no_mfa",
			assert: func(t *testing.T, claims jwt.MapClaims, _ map[string]any) {
				if _, ok := claims["acr"]; ok {
					t.Fatalf("acr present, want omitted")
				}
			},
		},
		{
			name:  "low_mfa",
			extra: "low_mfa",
			assert: func(t *testing.T, claims jwt.MapClaims, _ map[string]any) {
				if claims["acr"] != "loa1" {
					t.Fatalf("acr = %v, want loa1", claims["acr"])
				}
			},
		},
		{
			name:  "expired",
			extra: "expired",
			assert: func(t *testing.T, claims jwt.MapClaims, _ map[string]any) {
				exp, ok := claims["exp"].(float64)
				if !ok || exp >= float64(time.Now().Unix()) {
					t.Fatalf("exp = %v, want past expiry", claims["exp"])
				}
			},
		},
		{
			name:  "wrong_key",
			extra: "wrong_key",
			assert: func(t *testing.T, _ jwt.MapClaims, header map[string]any) {
				if header["kid"] != rotatedKeyID {
					t.Fatalf("kid = %v, want %s", header["kid"], rotatedKeyID)
				}
			},
		},
		{
			name:  "unsigned",
			extra: "unsigned",
			assert: func(t *testing.T, claims jwt.MapClaims, header map[string]any) {
				if header["alg"] != "none" {
					t.Fatalf("alg = %v, want none", header["alg"])
				}
				if _, ok := claims["sub"]; !ok {
					t.Fatalf("unsigned token missing sub claim")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, server, client := testProvider(t)
			code := authorize(t, client, server.URL, "state-1", "nonce-1", tc.extra)
			_, claims, header := exchange(t, client, server.URL, code, tokenForm(code))
			tc.assert(t, claims, header)
		})
	}
}

func fmtJSON(value any) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
