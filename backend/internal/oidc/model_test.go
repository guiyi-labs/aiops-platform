package oidc

import "testing"

func TestExternalIdentityTableName(t *testing.T) {
	if got := (ExternalIdentity{}).TableName(); got != "external_identities" {
		t.Fatalf("TableName = %q, want external_identities", got)
	}
}

func TestExternalIdentityFields(t *testing.T) {
	identity := ExternalIdentity{ID: 7, UserID: 3, Issuer: "https://idp.example.com", Subject: "sub-123"}
	if identity.Issuer == "" || identity.Subject == "" || identity.UserID == 0 {
		t.Fatalf("unexpected zero field on ExternalIdentity: %#v", identity)
	}
}
