// Package oidc implements the production OIDC authentication provider
// integration behind a small compiled interface. M36A introduces the
// immutable subject-prelinking model and the configuration contract; later
// phases add discovery/JWKS caching, the Authorization Code + PKCE flow,
// session/logout management and a synthetic IdP end-to-end gate.
//
// Invariants enforced by this package and the associated configuration
// (ADR 0032, ADR 0052):
//   - The provider subject is the only stable link to a local user. Automatic
//     email linking is forbidden.
//   - One provider subject maps to at most one local user.
//   - MFA evidence is identity-provider enforced and must appear in every
//     accepted ID token.
//   - The provider client secret never enters the browser, audit trail, logs
//     or policy file.
package oidc

import "time"

// ExternalIdentity is the immutable administrator-owned prelink between an OIDC
// provider subject and a local user. Rows are created explicitly by an
// administrator before the bound user can authenticate through OIDC; they are
// never created automatically from an incoming ID token.
//
// The (Issuer, Subject) pair is unique: a provider subject maps to at most one
// local user. Deleting a row (or disabling the local user) revokes the link.
type ExternalIdentity struct {
	ID        int64     `gorm:"primaryKey"`
	UserID    int64     `gorm:"not null;index"`
	Issuer    string    `gorm:"size:512;not null"`
	Subject   string    `gorm:"size:512;not null"`
	CreatedAt time.Time `gorm:"not null"`
}

// TableName pins the storage table so GORM table inference never drifts from
// the paired up/down migration.
func (ExternalIdentity) TableName() string { return "external_identities" }
