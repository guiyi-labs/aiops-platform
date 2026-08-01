package oidc

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// authSessionTTL bounds how long an authorization request may be outstanding.
// A callback arriving after this window is rejected so state/nonce/PKCE values
// cannot be replayed.
const authSessionTTL = 10 * time.Minute

// GenerateCodeVerifier generates a high-entropy PKCE code verifier and its
// S256 code challenge per RFC 7636. The verifier is 43 base64url characters
// (32 random bytes); the challenge is base64url(sha256(verifier)).
func GenerateCodeVerifier() (verifier, challenge string, err error) {
	buffer := make([]byte, 32)
	if _, err = rand.Read(buffer); err != nil {
		return "", "", fmt.Errorf("oidc: generate code verifier: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(buffer)
	digest := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(digest[:])
	return verifier, challenge, nil
}

// randomString returns a base64url-encoded random string of byteLen bytes.
func randomString(byteLen int) (string, error) {
	buffer := make([]byte, byteLen)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("oidc: generate random string: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// authSession binds an authorization request to its expected callback. It is
// carried in a short-lived HMAC-signed cookie so the server can validate state,
// nonce and PKCE without server-side session storage.
type authSession struct {
	State        string `json:"state"`
	Nonce        string `json:"nonce"`
	CodeVerifier string `json:"code_verifier"`
	ExpiresAt    int64  `json:"expires_at"`
}

// authSessionSigner signs and verifies authSession tokens using HMAC-SHA256.
type authSessionSigner struct {
	key []byte
	now func() time.Time
}

func newAuthSessionSigner(key []byte) *authSessionSigner {
	return &authSessionSigner{key: key, now: time.Now}
}

func (s *authSessionSigner) issue(state, nonce, codeVerifier string) (string, error) {
	session := authSession{
		State:        state,
		Nonce:        nonce,
		CodeVerifier: codeVerifier,
		ExpiresAt:    s.now().Add(authSessionTTL).Unix(),
	}
	payload, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("oidc: marshal auth session: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return encoded + "." + signature, nil
}

func (s *authSessionSigner) verify(token string) (authSession, error) {
	var session authSession
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return session, fmt.Errorf("oidc: malformed auth session token")
	}
	encoded := parts[0]
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return session, fmt.Errorf("oidc: malformed auth session signature")
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(encoded))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return session, fmt.Errorf("oidc: invalid auth session signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return session, fmt.Errorf("oidc: malformed auth session payload")
	}
	if err := json.Unmarshal(payload, &session); err != nil {
		return session, fmt.Errorf("oidc: decode auth session: %w", err)
	}
	if time.Unix(session.ExpiresAt, 0).Before(s.now()) {
		return session, fmt.Errorf("oidc: auth session expired")
	}
	return session, nil
}
