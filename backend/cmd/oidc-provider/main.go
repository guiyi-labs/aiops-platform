// Command oidc-provider runs a minimal local OIDC provider used by the M89
// identity-track drill to drive the platform's real Authorization Code + PKCE
// login over HTTPS without external network access.
//
// The provider mirrors the contract the platform enforces at runtime
// (ADR 0032, ADR 0052): an HTTPS issuer, discovery document, RS256 JWKS with
// key rotation, S256 PKCE, nonce echo and acr MFA evidence. It is a drill and
// development tool only, never a production identity provider.
//
// Endpoints:
//
//	/.well-known/openid-configuration  discovery document
//	/jwks                              current RSA signing key (kid "current")
//	/authorize                         issues an authorization code bound to the
//	                                   PKCE challenge, state and nonce; the drill
//	                                   passes the simulated user's identity
//	                                   attributes and optional fail mode as query
//	                                   parameters (user, username, display_name,
//	                                   groups, acr, fail)
//	/token                             exchanges the code for a signed ID token
//	                                   after verifying the S256 code verifier
//	/logout                            end-session endpoint (advertised only)
//	/healthz                           liveness for the drill wait loop
//
// Supported fail modes (query parameter fail=... on /authorize):
//
//	wrong_nonce  issue a token with a mismatched nonce
//	omit_nonce   issue a token without the nonce claim
//	wrong_issuer issue a token with an unexpected issuer
//	wrong_aud    issue a token with an unexpected audience
//	no_mfa       issue a token without acr/amr MFA evidence
//	low_mfa      issue a token with an unaccepted acr value
//	expired      issue a token already past its expiry
//	wrong_key    sign with the rotated key (not published in /jwks)
//	unsigned     return a token with a garbage signature
package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	currentKeyID = "current"
	rotatedKeyID = "rotated"
	codeTTL      = 5 * time.Minute
	tokenTTL     = 10 * time.Minute
)

type rsaKey struct {
	kid string
	key *rsa.PrivateKey
}

type authCode struct {
	challenge   string
	nonce       string
	state       string
	clientID    string
	redirectURI string
	subject     string
	username    string
	displayName string
	groups      []string
	acr         string
	fail        string
	expiresAt   time.Time
}

// localIdP is the drill identity provider. It is safe for concurrent use.
type localIdP struct {
	issuer      string
	clientID    string
	redirectURI string
	current     *rsaKey
	rotated     *rsaKey
	now         func() time.Time

	mu    sync.Mutex
	codes map[string]*authCode
}

func newLocalIdP(issuer, clientID, redirectURI string) *localIdP {
	return &localIdP{
		issuer:      issuer,
		clientID:    clientID,
		redirectURI: redirectURI,
		current:     mustGenerateKey(currentKeyID),
		rotated:     mustGenerateKey(rotatedKeyID),
		now:         time.Now,
		codes:       make(map[string]*authCode),
	}
}

func mustGenerateKey(kid string) *rsaKey {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("oidc-provider: generate %s key: %v", kid, err)
	}
	return &rsaKey{kid: kid, key: key}
}

func (i *localIdP) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", i.handleDiscovery)
	mux.HandleFunc("/jwks", i.handleJWKS)
	mux.HandleFunc("/authorize", i.handleAuthorize)
	mux.HandleFunc("/token", i.handleToken)
	mux.HandleFunc("/logout", i.handleLogout)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	return mux
}

func (i *localIdP) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"issuer":                                i.issuer,
		"authorization_endpoint":                i.issuer + "/authorize",
		"token_endpoint":                        i.issuer + "/token",
		"jwks_uri":                              i.issuer + "/jwks",
		"end_session_endpoint":                  i.issuer + "/logout",
		"response_types_supported":              []string{"code"},
		"code_challenge_methods_supported":      []string{"S256"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile"},
	})
}

func (i *localIdP) handleJWKS(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"keys": []map[string]string{jwkEntry(i.current)}})
}

func jwkEntry(key *rsaKey) map[string]string {
	return map[string]string{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": key.kid,
		"n":   base64.RawURLEncoding.EncodeToString(key.key.PublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.key.PublicKey.E)).Bytes()),
	}
}

func (i *localIdP) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if query.Get("response_type") != "code" {
		writeError(w, http.StatusBadRequest, "invalid_request", "response_type must be code")
		return
	}
	if query.Get("client_id") != i.clientID {
		writeError(w, http.StatusBadRequest, "unauthorized_client", "unknown client_id")
		return
	}
	if query.Get("redirect_uri") != i.redirectURI {
		writeError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is not registered")
		return
	}
	if !containsScope(query.Get("scope"), "openid") {
		writeError(w, http.StatusBadRequest, "invalid_scope", "openid scope is required")
		return
	}
	state := query.Get("state")
	nonce := query.Get("nonce")
	challenge := query.Get("code_challenge")
	if state == "" || nonce == "" || challenge == "" || query.Get("code_challenge_method") != "S256" {
		writeError(w, http.StatusBadRequest, "invalid_request", "state, nonce, code_challenge and code_challenge_method=S256 are required")
		return
	}

	code := randomToken(32)
	now := i.now()
	i.mu.Lock()
	i.codes[code] = &authCode{
		challenge:   challenge,
		nonce:       nonce,
		state:       state,
		clientID:    i.clientID,
		redirectURI: i.redirectURI,
		subject:     firstNonEmpty(query.Get("user"), "drill-user"),
		username:    firstNonEmpty(query.Get("username"), "drill-user"),
		displayName: firstNonEmpty(query.Get("display_name"), "Drill User"),
		groups:      splitCSV(query.Get("groups"), []string{"platform-operators"}),
		acr:         firstNonEmpty(query.Get("acr"), "phr"),
		fail:        query.Get("fail"),
		expiresAt:   now.Add(codeTTL),
	}
	i.mu.Unlock()

	location := i.redirectURI + "?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(state)
	http.Redirect(w, r, location, http.StatusFound)
}

func (i *localIdP) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "malformed form body")
		return
	}
	if r.PostForm.Get("grant_type") != "authorization_code" {
		writeError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code")
		return
	}
	codeValue := r.PostForm.Get("code")
	verifier := r.PostForm.Get("code_verifier")
	if codeValue == "" || verifier == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "code and code_verifier are required")
		return
	}
	if r.PostForm.Get("client_id") != i.clientID {
		writeError(w, http.StatusBadRequest, "invalid_client", "client_id mismatch")
		return
	}
	if r.PostForm.Get("redirect_uri") != i.redirectURI {
		writeError(w, http.StatusBadRequest, "invalid_request", "redirect_uri mismatch")
		return
	}

	i.mu.Lock()
	code, ok := i.codes[codeValue]
	if ok {
		delete(i.codes, codeValue)
	}
	i.mu.Unlock()
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_grant", "code is unknown, expired or already used")
		return
	}
	if i.now().After(code.expiresAt) {
		writeError(w, http.StatusBadRequest, "invalid_grant", "code expired")
		return
	}
	if s256Challenge(verifier) != code.challenge {
		writeError(w, http.StatusBadRequest, "invalid_grant", "PKCE code_verifier does not match code_challenge")
		return
	}

	idToken, err := i.issueIDToken(code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server_error", "unable to sign id_token")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id_token":     idToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
		"access_token": "drill-access-" + randomToken(16),
	})
}

func (i *localIdP) issueIDToken(code *authCode) (string, error) {
	now := i.now()
	claims := jwt.MapClaims{
		"iss":                i.issuer,
		"sub":                code.subject,
		"aud":                i.clientID,
		"iat":                now.Unix(),
		"exp":                now.Add(tokenTTL).Unix(),
		"nonce":              code.nonce,
		"acr":                code.acr,
		"preferred_username": code.username,
		"name":               code.displayName,
		"groups":             code.groups,
		"email":              code.username + "@drill.local",
	}

	signingKey := i.current
	switch code.fail {
	case "wrong_nonce":
		claims["nonce"] = "attacker-controlled-nonce"
	case "omit_nonce":
		delete(claims, "nonce")
	case "wrong_issuer":
		claims["iss"] = "https://evil.invalid"
	case "wrong_aud":
		claims["aud"] = "some-other-client"
	case "no_mfa":
		delete(claims, "acr")
	case "low_mfa":
		claims["acr"] = "loa1"
	case "expired":
		claims["exp"] = now.Add(-time.Hour).Unix()
	case "wrong_key":
		signingKey = i.rotated
	case "unsigned":
		return unsignedToken(claims), nil
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = signingKey.kid
	return token.SignedString(signingKey.key)
}

// unsignedToken builds a syntactically JWT-shaped token with a garbage
// signature. The platform rejects it during signature verification.
func unsignedToken(claims jwt.MapClaims) string {
	header, _ := json.Marshal(jwt.MapClaims{"alg": "none", "typ": "JWT", "kid": currentKeyID})
	payload, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding
	return enc.EncodeToString(header) + "." + enc.EncodeToString(payload) + "." + enc.EncodeToString([]byte("garbage-signature"))
}

func (i *localIdP) handleLogout(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"end_session_endpoint": i.issuer + "/logout",
		"post_logout_redirect": i.redirectURI,
	})
}

// ---- helpers ----

func s256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func containsScope(scope, required string) bool {
	for _, part := range strings.Fields(scope) {
		if part == required {
			return true
		}
	}
	return false
}

func splitCSV(raw string, fallback []string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string(nil), fallback...)
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func randomToken(bytes int) string {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		log.Fatalf("oidc-provider: random token: %v", err)
	}
	return hex.EncodeToString(buf)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, description string) {
	writeJSON(w, status, map[string]string{"error": code, "error_description": description})
}

// generateCert writes a self-signed certificate covering localhost and
// 127.0.0.1 so the drill platform process can trust the provider via
// SSL_CERT_FILE. The returned files are plain PEM.
func generateCert(certOut, keyOut string) error {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "aiops-drill-oidc-provider"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(certOut, certPEM, 0o600); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}
	if err := os.WriteFile(keyOut, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	return nil
}

func main() {
	issuer := flag.String("issuer", "", "HTTPS issuer URL advertised in discovery (required)")
	listen := flag.String("listen", "127.0.0.1:9443", "listen address")
	clientID := flag.String("client-id", "aiops-platform", "registered client_id")
	redirectURI := flag.String("redirect-uri", "https://localhost:8090/api/v1/auth/oidc/callback", "registered redirect_uri")
	certPath := flag.String("cert", "", "TLS certificate PEM; auto-generated when empty")
	keyPath := flag.String("key", "", "TLS key PEM; auto-generated when empty")
	certOut := flag.String("cert-out", "", "write the TLS certificate PEM here")
	keyOut := flag.String("key-out", "", "write the TLS key PEM here")
	flag.Parse()

	if *issuer == "" {
		log.Fatal("oidc-provider: -issuer is required")
	}
	if *certPath == "" || *keyPath == "" {
		if *certOut == "" {
			log.Fatal("oidc-provider: -cert-out is required when auto-generating the certificate")
		}
		if *keyOut == "" {
			log.Fatal("oidc-provider: -key-out is required when auto-generating the certificate")
		}
		if err := generateCert(*certOut, *keyOut); err != nil {
			log.Fatalf("oidc-provider: generate certificate: %v", err)
		}
		*certPath, *keyPath = *certOut, *keyOut
	}

	idp := newLocalIdP(strings.TrimSuffix(*issuer, "/"), *clientID, *redirectURI)
	server := &http.Server{
		Addr:              *listen,
		Handler:           idp.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}

	go func() {
		log.Printf("oidc-provider: listening on https://%s (issuer %s)", *listen, *issuer)
		if err := server.ListenAndServeTLS(*certPath, *keyPath); err != nil && err != http.ErrServerClosed {
			log.Fatalf("oidc-provider: serve: %v", err)
		}
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	log.Println("oidc-provider: shut down")
}
