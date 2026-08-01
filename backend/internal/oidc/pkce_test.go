package oidc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestGenerateCodeVerifierProducesValidS256Pair(t *testing.T) {
	verifier, challenge, err := GenerateCodeVerifier()
	if err != nil {
		t.Fatalf("GenerateCodeVerifier error = %v", err)
	}
	if len(verifier) < 43 {
		t.Fatalf("verifier length = %d, want >= 43 (RFC 7636)", len(verifier))
	}
	if len(verifier) > 128 {
		t.Fatalf("verifier length = %d, want <= 128 (RFC 7636)", len(verifier))
	}
	for _, c := range verifier {
		if !isUnreserved(c) {
			t.Fatalf("verifier contains non-unreserved character %q", c)
		}
	}
	digest := sha256.Sum256([]byte(verifier))
	expected := base64.RawURLEncoding.EncodeToString(digest[:])
	if challenge != expected {
		t.Fatalf("challenge = %q, want %q", challenge, expected)
	}
}

func TestGenerateCodeVerifierProducesUniqueValues(t *testing.T) {
	seen := make(map[string]struct{}, 8)
	for i := 0; i < 8; i++ {
		verifier, _, err := GenerateCodeVerifier()
		if err != nil {
			t.Fatalf("GenerateCodeVerifier %d error = %v", i, err)
		}
		if _, dup := seen[verifier]; dup {
			t.Fatalf("verifier %d collided with a previous value", i)
		}
		seen[verifier] = struct{}{}
	}
}

func TestRandomStringProducesUniqueValues(t *testing.T) {
	seen := make(map[string]struct{}, 4)
	for i := 0; i < 4; i++ {
		value, err := randomString(32)
		if err != nil {
			t.Fatalf("randomString %d error = %v", i, err)
		}
		if len(value) == 0 {
			t.Fatalf("randomString %d returned empty value", i)
		}
		if _, dup := seen[value]; dup {
			t.Fatalf("randomString %d collided with a previous value", i)
		}
		seen[value] = struct{}{}
	}
}

func TestAuthSessionSignerIssueVerifyRoundTrip(t *testing.T) {
	key := []byte("test-signing-key-with-at-least-32-bytes!!")
	now := time.Now()
	signer := &authSessionSigner{key: key, now: func() time.Time { return now }}

	token, err := signer.issue("state-123", "nonce-456", "verifier-789")
	if err != nil {
		t.Fatalf("issue error = %v", err)
	}
	if token == "" {
		t.Fatal("issued token is empty")
	}
	if strings.Count(token, ".") != 1 {
		t.Fatalf("token must contain exactly one '.', got %q", token)
	}

	session, err := signer.verify(token)
	if err != nil {
		t.Fatalf("verify error = %v", err)
	}
	if session.State != "state-123" || session.Nonce != "nonce-456" || session.CodeVerifier != "verifier-789" {
		t.Fatalf("verified session = %#v", session)
	}
}

func TestAuthSessionSignerRejectsMalformedTokens(t *testing.T) {
	signer := newAuthSessionSigner([]byte("test-signing-key-with-at-least-32-bytes!!"))
	cases := map[string]string{
		"empty":             "",
		"no separator":      "abc",
		"too many parts":    "a.b.c",
		"bad signature b64": "payload.!!!not-base64!!!",
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := signer.verify(token); err == nil {
				t.Fatalf("verify error = nil, want malformed token error for %q", name)
			}
		})
	}
}

func TestAuthSessionSignerRejectsWrongSignature(t *testing.T) {
	issuer := newAuthSessionSigner([]byte("signing-key-a-with-at-least-32-bytes!!!"))
	verifier := newAuthSessionSigner([]byte("signing-key-b-with-at-least-32-bytes!!!"))
	token, err := issuer.issue("state", "nonce", "verifier")
	if err != nil {
		t.Fatalf("issue error = %v", err)
	}
	if _, err := verifier.verify(token); err == nil {
		t.Fatal("verify error = nil, want signature mismatch error")
	}
}

func TestAuthSessionSignerRejectsExpiredSession(t *testing.T) {
	now := time.Now()
	signer := &authSessionSigner{
		key: []byte("test-signing-key-with-at-least-32-bytes!!"),
		now: func() time.Time { return now },
	}
	token, err := signer.issue("state", "nonce", "verifier")
	if err != nil {
		t.Fatalf("issue error = %v", err)
	}
	// Advance past the authSessionTTL window.
	signer.now = func() time.Time { return now.Add(authSessionTTL + time.Second) }
	if _, err := signer.verify(token); err == nil {
		t.Fatal("verify error = nil, want expired session error")
	}
}

func TestAuthSessionSignerRejectsCorruptedPayload(t *testing.T) {
	key := []byte("test-signing-key-with-at-least-32-bytes!!")
	signer := newAuthSessionSigner(key)
	// Build a token with a valid HMAC over a non-JSON payload so the signature
	// verifies but the payload decode fails.
	encoded := base64.RawURLEncoding.EncodeToString([]byte("not-json"))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(encoded))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	token := encoded + "." + signature
	if _, err := signer.verify(token); err == nil {
		t.Fatal("verify error = nil, want payload decode error")
	}
}

// isUnreserved reports whether c is an RFC 7636 unreserved PKCE character.
func isUnreserved(c rune) bool {
	switch {
	case 'A' <= c && c <= 'Z':
		return true
	case 'a' <= c && c <= 'z':
		return true
	case '0' <= c && c <= '9':
		return true
	case c == '-' || c == '.' || c == '_' || c == '~':
		return true
	}
	return false
}
