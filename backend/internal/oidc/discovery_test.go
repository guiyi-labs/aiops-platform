package oidc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func discoveryHandler(issuer string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Discovery{
			Issuer:                           issuer,
			AuthorizationEndpoint:            issuer + "/authorize",
			TokenEndpoint:                    issuer + "/token",
			JWKSURI:                          issuer + "/jwks",
			EndSessionEndpoint:               issuer + "/logout",
			ResponseTypesSupported:           []string{"code"},
			CodeChallengeMethodsSupported:    []string{"S256"},
			IDTokenSigningAlgValuesSupported: []string{"RS256", "ES256"},
			ScopesSupported:                  []string{"openid", "profile"},
		})
	})
	return mux
}

func newTestDiscoveryFetcher(t *testing.T, server *httptest.Server, issuer string) *DiscoveryFetcher {
	t.Helper()
	return &DiscoveryFetcher{
		client: testHTTPClient(t, server),
		config: oidcProviderConfig{
			issuer:                   issuer,
			clientID:                 "aiops-platform",
			requiredScopes:           []string{"openid", "profile"},
			allowedSigningAlgorithms: []string{"RS256", "ES256"},
			fetchTimeout:             5 * time.Second,
			cacheTTL:                 time.Hour,
		},
		now: time.Now,
	}
}

func TestDiscoveryFetchAndValidate(t *testing.T) {
	server := newTestTLSServer(t, discoveryHandler("placeholder"))
	issuer := server.URL
	// Reconfigure handler with the real issuer (known after server starts).
	server.Config.Handler = discoveryHandler(issuer)
	fetcher := newTestDiscoveryFetcher(t, server, issuer)

	discovery, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch error = %v", err)
	}
	if discovery.Issuer != issuer {
		t.Fatalf("Issuer = %q, want %q", discovery.Issuer, issuer)
	}
	if discovery.JWKSURI != issuer+"/jwks" {
		t.Fatalf("JWKSURI = %q", discovery.JWKSURI)
	}
}

func TestDiscoveryFetchCachesWithinTTL(t *testing.T) {
	calls := 0
	handler := http.NewServeMux()
	handler.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Discovery{
			Issuer:                           "placeholder",
			AuthorizationEndpoint:            "https://idp.example.com/authorize",
			TokenEndpoint:                    "https://idp.example.com/token",
			JWKSURI:                          "https://idp.example.com/jwks",
			EndSessionEndpoint:               "https://idp.example.com/logout",
			ResponseTypesSupported:           []string{"code"},
			CodeChallengeMethodsSupported:    []string{"S256"},
			IDTokenSigningAlgValuesSupported: []string{"RS256"},
			ScopesSupported:                  []string{"openid", "profile"},
		})
	})
	server := newTestTLSServer(t, handler)
	// Use the server URL as issuer so discovery issuer matches config issuer.
	fetcher := newTestDiscoveryFetcher(t, server, server.URL)
	// Patch the handler to return the server URL as issuer.
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Discovery{
			Issuer:                           server.URL,
			AuthorizationEndpoint:            server.URL + "/authorize",
			TokenEndpoint:                    server.URL + "/token",
			JWKSURI:                          server.URL + "/jwks",
			EndSessionEndpoint:               server.URL + "/logout",
			ResponseTypesSupported:           []string{"code"},
			CodeChallengeMethodsSupported:    []string{"S256"},
			IDTokenSigningAlgValuesSupported: []string{"RS256", "ES256"},
			ScopesSupported:                  []string{"openid", "profile"},
		})
	})

	if _, err := fetcher.Fetch(context.Background()); err != nil {
		t.Fatalf("first Fetch error = %v", err)
	}
	if _, err := fetcher.Fetch(context.Background()); err != nil {
		t.Fatalf("second Fetch error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("provider called %d times, want 1 (cached within TTL)", calls)
	}
}

func TestDiscoveryValidateRejectsContractViolations(t *testing.T) {
	fetcher := &DiscoveryFetcher{
		config: oidcProviderConfig{
			issuer:                   "https://idp.example.com",
			requiredScopes:           []string{"openid", "profile"},
			allowedSigningAlgorithms: []string{"RS256"},
		},
		now: time.Now,
	}
	base := Discovery{
		Issuer:                           "https://idp.example.com",
		AuthorizationEndpoint:            "https://idp.example.com/authorize",
		TokenEndpoint:                    "https://idp.example.com/token",
		JWKSURI:                          "https://idp.example.com/jwks",
		EndSessionEndpoint:               "https://idp.example.com/logout",
		ResponseTypesSupported:           []string{"code"},
		CodeChallengeMethodsSupported:    []string{"S256"},
		IDTokenSigningAlgValuesSupported: []string{"RS256"},
		ScopesSupported:                  []string{"openid", "profile"},
	}
	cases := map[string]func(d *Discovery){
		"issuer mismatch":     func(d *Discovery) { d.Issuer = "https://other.example.com" },
		"http authz endpoint": func(d *Discovery) { d.AuthorizationEndpoint = "http://idp.example.com/authorize" },
		"http token endpoint": func(d *Discovery) { d.TokenEndpoint = "http://idp.example.com/token" },
		"http jwks uri":       func(d *Discovery) { d.JWKSURI = "http://idp.example.com/jwks" },
		"http end session":    func(d *Discovery) { d.EndSessionEndpoint = "http://idp.example.com/logout" },
		"no code response":    func(d *Discovery) { d.ResponseTypesSupported = []string{"token"} },
		"no pkce s256":        func(d *Discovery) { d.CodeChallengeMethodsSupported = []string{"plain"} },
		"missing scope":       func(d *Discovery) { d.ScopesSupported = []string{"openid"} },
		"missing algorithm":   func(d *Discovery) { d.IDTokenSigningAlgValuesSupported = []string{"ES256"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := base
			mutate(&d)
			if err := fetcher.Validate(d); err == nil {
				t.Fatalf("Validate error = nil, want contract violation for %q", name)
			}
		})
	}
}

func TestDiscoveryFetchRejectsHTTPSIssuer(t *testing.T) {
	fetcher := &DiscoveryFetcher{
		config: oidcProviderConfig{issuer: "http://insecure.example.com"},
		now:    time.Now,
	}
	if _, err := fetcher.Fetch(context.Background()); err == nil {
		t.Fatal("Fetch error = nil, want HTTPS issuer error")
	}
}

func TestDiscoveryFetchRejectsRedirect(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	})
	server := newTestTLSServer(t, handler)
	fetcher := newTestDiscoveryFetcher(t, server, server.URL)
	if _, err := fetcher.Fetch(context.Background()); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("Fetch error = %v, want redirect rejection", err)
	}
}
