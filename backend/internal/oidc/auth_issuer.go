package oidc

import (
	"context"
	"fmt"

	"k8s-aiops.local/backend/internal/auth"
)

// AuthSessionIssuer adapts auth.Service to the oidc.SessionIssuer interface.
// It delegates to auth.Service.IssueSessionForUser so OIDC-issued local
// sessions flow through the same refresh-token rotation, auth_version
// revocation and audit semantics as password login. The adapter also projects
// the auth.Session onto the OIDC-local Session shape so the session manager
// stays independent of the auth package's concrete types.
type AuthSessionIssuer struct {
	service *auth.Service
}

// NewAuthSessionIssuer constructs an issuer backed by the auth service. The
// service must be the same instance wired into the HTTP server so auth_version
// revocation and role re-derivation stay authoritative.
func NewAuthSessionIssuer(service *auth.Service) *AuthSessionIssuer {
	return &AuthSessionIssuer{service: service}
}

// IssueSession delegates to auth.Service.IssueSessionForUser and projects the
// result onto the oidc-local Session. A disabled or missing user fails closed
// with ErrUserDisabled so the session manager surfaces a single, consistent
// error to the callback handler.
func (a *AuthSessionIssuer) IssueSession(ctx context.Context, userID int64, userAgent, ipAddress string) (Session, error) {
	session, err := a.service.IssueSessionForUser(ctx, userID, userAgent, ipAddress)
	if err != nil {
		if err == auth.ErrUserDisabled {
			return Session{}, ErrUserDisabled
		}
		return Session{}, fmt.Errorf("oidc: issue local session: %w", err)
	}
	return Session{
		AccessToken:          session.AccessToken,
		TokenType:            session.TokenType,
		AccessTokenExpiresIn: session.AccessTokenExpiresIn,
		RefreshToken:         session.RefreshToken(),
		User: LocalUser{
			ID:          session.User.ID,
			Username:    session.User.Username,
			DisplayName: session.User.DisplayName,
			Status:      auth.StatusActive,
			Roles:       append([]string(nil), session.User.Roles...),
		},
	}, nil
}
