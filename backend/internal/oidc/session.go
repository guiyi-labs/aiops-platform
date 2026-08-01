package oidc

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// LocalUser is the resolved local account for an OIDC subject. The caller
// resolves it from the external_identities table; an incoming ID token never
// creates a row, and an email match never creates a link.
type LocalUser struct {
	ID          int64
	Username    string
	DisplayName string
	Status      string
	Roles       []string
}

// Session is the local session issued after a successful OIDC callback. It
// mirrors the shape of auth.Session so the existing revocation, grant and
// audit machinery applies uniformly. The refresh token is exposed because the
// HTTP handler must return it to the client through the same cookie channel as
// local login.
type Session struct {
	AccessToken          string
	TokenType            string
	AccessTokenExpiresIn int64
	RefreshToken         string
	User                 LocalUser
}

// IdentityResolver resolves an OIDC (issuer, subject) pair to a prelinked local
// user via the external_identities table. Automatic email linking is forbidden;
// the resolver only returns users an administrator explicitly prelinked.
type IdentityResolver interface {
	ResolveBySubject(ctx context.Context, issuer, subject string) (LocalUser, error)
}

// SessionIssuer issues a local session for a prelinked local user. The
// implementation delegates to auth.Service so refresh-token rotation,
// auth_version revocation, per-request status checks and audit semantics are
// preserved. OIDC login produces a local session through the same path as
// password login.
type SessionIssuer interface {
	IssueSession(ctx context.Context, userID int64, userAgent, ipAddress string) (Session, error)
}

// ErrSubjectNotPrelinked indicates no administrator prelinked the OIDC subject
// to a local user. The callback must fail closed; automatic linking is
// forbidden (ADR 0052).
var ErrSubjectNotPrelinked = errors.New("oidc: subject is not prelinked to a local user")

// ErrUserDisabled indicates the prelinked local user is disabled. OIDC login
// must fail closed even when the ID token is valid.
var ErrUserDisabled = errors.New("oidc: prelinked user is disabled")

// SessionManager orchestrates the OIDC callback into a local session: it
// verifies the ID token through Provider.HandleCallback, resolves the immutable
// subject to a prelinked local user, and issues a local session through the
// same path as password login. It is safe for concurrent use after the
// Provider is initialized.
type SessionManager struct {
	provider *Provider
	resolver IdentityResolver
	issuer   SessionIssuer
	reauth   time.Duration
	now      func() time.Time
}

// SessionManagerConfig adapts the platform configuration into the session
// manager parameters. Reauthentication bounds how long an OIDC-issued local
// session may be used before the user must reauthenticate with the provider.
type SessionManagerConfig struct {
	Reauthentication time.Duration
}

// NewSessionManager constructs a session manager bound to the verified
// provider, the identity resolver and the session issuer. The provider must be
// initialized before CompleteLogin is called.
func NewSessionManager(provider *Provider, resolver IdentityResolver, issuer SessionIssuer, cfg SessionManagerConfig) *SessionManager {
	reauth := cfg.Reauthentication
	if reauth <= 0 {
		reauth = time.Hour
	}
	return &SessionManager{
		provider: provider,
		resolver: resolver,
		issuer:   issuer,
		reauth:   reauth,
		now:      time.Now,
	}
}

// CompleteLogin completes the OIDC Authorization Code + PKCE flow and issues a
// local session for the prelinked user. It fails closed when:
//   - the ID token verification fails (Provider.HandleCallback),
//   - the subject is not prelinked to a local user,
//   - the prelinked user is disabled,
//   - the session issuer fails.
//
// The returned Session carries the local access and refresh tokens; the caller
// applies them through the same cookie channel as local login.
func (m *SessionManager) CompleteLogin(ctx context.Context, code, state, sessionToken, userAgent, ipAddress string) (Session, error) {
	result, err := m.provider.HandleCallback(ctx, code, state, sessionToken)
	if err != nil {
		return Session{}, err
	}
	user, err := m.resolver.ResolveBySubject(ctx, result.Issuer, result.Subject)
	if err != nil {
		return Session{}, fmt.Errorf("oidc: resolve prelinked subject: %w", err)
	}
	if user.Status != "active" {
		return Session{}, ErrUserDisabled
	}
	session, err := m.issuer.IssueSession(ctx, user.ID, userAgent, ipAddress)
	if err != nil {
		return Session{}, fmt.Errorf("oidc: issue local session: %w", err)
	}
	// The local account's effective roles are re-derived from the prelinked
	// local user on every protected request (auth.Authenticate), not from the
	// ID token alone. This keeps auth_version revocation and M35 grant checking
	// authoritative.
	return session, nil
}

// Reauthentication returns the configured reauthentication interval. The HTTP
// handler uses it to decide when to redirect an authenticated user back to the
// provider for fresh MFA evidence rather than silently refreshing the local
// access token.
func (m *SessionManager) Reauthentication() time.Duration { return m.reauth }

// Provider returns the bound OIDC provider so the HTTP handler can build the
// authorization URL and the RP-initiated logout URL. It is exported only for
// the server-side wiring; the provider is constructed and initialized once at
// startup.
func (m *SessionManager) Provider() *Provider { return m.provider }

// LogoutURL builds the provider end_session endpoint URL for RP-initiated
// logout. idTokenHint is the ID token from the user's current OIDC login (when
// available) so the provider can identify the session; postLogoutRedirectURI
// is the platform URL the provider should redirect to after logout. Both
// parameters are optional per OIDC spec, but the provider may require one or
// both.
func (p *Provider) LogoutURL(ctx context.Context, idTokenHint, postLogoutRedirectURI string) (string, error) {
	discovery, err := p.discovery.Fetch(ctx)
	if err != nil {
		return "", err
	}
	if discovery.EndSessionEndpoint == "" {
		return "", fmt.Errorf("oidc: provider does not advertise an end_session_endpoint")
	}
	params := url.Values{}
	if idTokenHint != "" {
		params.Set("id_token_hint", idTokenHint)
	}
	if postLogoutRedirectURI != "" {
		if err := requireHTTPSURL(postLogoutRedirectURI); err != nil {
			return "", fmt.Errorf("oidc: post_logout_redirect_uri must be an absolute HTTPS URL: %w", err)
		}
		params.Set("post_logout_redirect_uri", postLogoutRedirectURI)
	}
	if len(params) == 0 {
		return discovery.EndSessionEndpoint, nil
	}
	return discovery.EndSessionEndpoint + "?" + params.Encode(), nil
}

// StripBearerPrefix removes an optional "Bearer " prefix from raw so the caller
// can pass an Authorization header value directly to LogoutURL as the
// id_token_hint. It is a small helper kept here so handlers do not each
// re-implement the trim.
func StripBearerPrefix(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		return strings.TrimSpace(raw[len("bearer "):])
	}
	return raw
}
