package oidc

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"k8s-aiops.local/backend/internal/auth"
)

// ErrIdentityNotPrelinked wraps the not-found case so the session manager can
// distinguish "no row" from a real database error. It is the storage-side
// counterpart of ErrSubjectNotPrelinked.
var ErrIdentityNotPrelinked = errors.New("oidc: identity is not prelinked to a local user")

// GormIdentityResolver resolves an OIDC (issuer, subject) pair to a prelinked
// local user by joining external_identities with users. Automatic email
// linking is forbidden: only an explicit administrator-created row resolves.
type GormIdentityResolver struct {
	db *gorm.DB
}

// NewGormIdentityResolver constructs a resolver bound to the GORM database
// handle. The handle must have access to the external_identities and users
// tables (migration 000026 and the auth schema).
func NewGormIdentityResolver(db *gorm.DB) *GormIdentityResolver {
	return &GormIdentityResolver{db: db}
}

// ResolveBySubject looks up the prelinked local user for (issuer, subject).
// It fails closed with ErrSubjectNotPrelinked when no row exists, when the
// joined user is missing, or when the database errors. The returned LocalUser
// carries the local account's status and role codes so the session manager
// can enforce the active-status check and so per-request role re-derivation
// (via auth.Authenticate) stays authoritative.
func (r *GormIdentityResolver) ResolveBySubject(ctx context.Context, issuer, subject string) (LocalUser, error) {
	if issuer == "" || subject == "" {
		return LocalUser{}, ErrSubjectNotPrelinked
	}
	var identity ExternalIdentity
	err := r.db.WithContext(ctx).
		Where("issuer = ? AND subject = ?", issuer, subject).
		First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return LocalUser{}, ErrSubjectNotPrelinked
	}
	if err != nil {
		return LocalUser{}, fmt.Errorf("oidc: query external identity: %w", err)
	}
	var user auth.User
	err = r.db.WithContext(ctx).
		Preload("Roles").
		First(&user, identity.UserID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return LocalUser{}, ErrSubjectNotPrelinked
	}
	if err != nil {
		return LocalUser{}, fmt.Errorf("oidc: load prelinked user: %w", err)
	}
	return LocalUser{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Status:      user.Status,
		Roles:       user.RoleCodes(),
	}, nil
}
