package auth

import (
	"testing"
	"time"
)

func TestTokenManager(t *testing.T) {
	manager := NewTokenManager("a-signing-key-that-is-long-enough-for-tests", 15*time.Minute)
	user := User{ID: 42, Username: "admin", AuthVersion: 3, Roles: []Role{{Code: SystemAdmin}}}

	raw, expiresAt, err := manager.IssueAccessToken(user)
	if err != nil {
		t.Fatalf("IssueAccessToken() error = %v", err)
	}
	if expiresAt.Before(time.Now()) {
		t.Fatal("IssueAccessToken() returned an expired token")
	}
	claims, err := manager.ParseAccessToken(raw)
	if err != nil {
		t.Fatalf("ParseAccessToken() error = %v", err)
	}
	if claims.Subject != "42" || claims.Username != "admin" || claims.AuthVersion != 3 || len(claims.Roles) != 1 {
		t.Fatalf("claims = %#v", claims)
	}
}

func TestRefreshTokenIsHashed(t *testing.T) {
	raw, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("NewRefreshToken() error = %v", err)
	}
	if raw == hash || HashRefreshToken(raw) != hash {
		t.Fatal("refresh token hash is invalid")
	}
}
