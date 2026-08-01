package oidc

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// stubIdentityResolver is a test IdentityResolver that returns a configured
// local user for a matching (issuer, subject) pair.
type stubIdentityResolver struct {
	user        LocalUser
	err         error
	called      int32
	mu          sync.Mutex
	lastIssuer  string
	lastSubject string
}

func (r *stubIdentityResolver) ResolveBySubject(ctx context.Context, issuer, subject string) (LocalUser, error) {
	r.mu.Lock()
	r.called++
	r.lastIssuer = issuer
	r.lastSubject = subject
	r.mu.Unlock()
	if r.err != nil {
		return LocalUser{}, r.err
	}
	return r.user, nil
}

// stubSessionIssuer is a test SessionIssuer that returns a configured session.
type stubSessionIssuer struct {
	session       Session
	err           error
	called        int32
	mu            sync.Mutex
	lastUserID    int64
	lastUserAgent string
	lastIPAddress string
}

func (i *stubSessionIssuer) IssueSession(ctx context.Context, userID int64, userAgent, ipAddress string) (Session, error) {
	i.mu.Lock()
	i.called++
	i.lastUserID = userID
	i.lastUserAgent = userAgent
	i.lastIPAddress = ipAddress
	i.mu.Unlock()
	if i.err != nil {
		return Session{}, i.err
	}
	return i.session, nil
}

func newSessionManagerForTest(t *testing.T, idp *syntheticIdP, resolver IdentityResolver, issuer SessionIssuer) *SessionManager {
	t.Helper()
	provider := newProviderWithSyntheticIdP(t, idp)
	return NewSessionManager(provider, resolver, issuer, SessionManagerConfig{Reauthentication: time.Hour})
}

func TestCompleteLoginIssuesLocalSession(t *testing.T) {
	idp := newSyntheticIdP(t)
	resolver := &stubIdentityResolver{user: LocalUser{ID: 7, Username: "operator", DisplayName: "Platform Operator", Status: "active", Roles: []string{"system_admin"}}}
	issuer := &stubSessionIssuer{session: Session{AccessToken: "access-123", RefreshToken: "refresh-456", TokenType: "Bearer", AccessTokenExpiresIn: 900, User: resolver.user}}
	manager := newSessionManagerForTest(t, idp, resolver, issuer)

	_, sessionToken, err := manager.provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := manager.signer().verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	idp.SetIDToken(idp.signIDToken(t, map[string]any{"nonce": session.Nonce}))

	result, err := manager.CompleteLogin(context.Background(), "code", session.State, sessionToken, "test-agent", "127.0.0.1")
	if err != nil {
		t.Fatalf("CompleteLogin error = %v", err)
	}
	if result.AccessToken != "access-123" || result.RefreshToken != "refresh-456" {
		t.Fatalf("session = %#v", result)
	}
	if result.User.ID != 7 || result.User.Username != "operator" {
		t.Fatalf("session user = %#v", result.User)
	}
	if resolver.called == 0 || resolver.lastIssuer != idp.issuer || resolver.lastSubject != "sub-123" {
		t.Fatalf("resolver not called with issuer/subject: calls=%d issuer=%q subject=%q", resolver.called, resolver.lastIssuer, resolver.lastSubject)
	}
	if issuer.called == 0 || issuer.lastUserID != 7 || issuer.lastUserAgent != "test-agent" || issuer.lastIPAddress != "127.0.0.1" {
		t.Fatalf("issuer not called with user id/agent/ip: calls=%d user=%d agent=%q ip=%q", issuer.called, issuer.lastUserID, issuer.lastUserAgent, issuer.lastIPAddress)
	}
}

func TestCompleteLoginFailsWhenIDTokenInvalid(t *testing.T) {
	idp := newSyntheticIdP(t)
	resolver := &stubIdentityResolver{user: LocalUser{ID: 7, Status: "active"}}
	issuer := &stubSessionIssuer{session: Session{AccessToken: "access"}}
	manager := newSessionManagerForTest(t, idp, resolver, issuer)

	_, sessionToken, err := manager.provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := manager.signer().verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	// Issue an ID token with the wrong nonce so verification fails closed.
	idp.SetIDToken(idp.signIDToken(t, map[string]any{"nonce": "different-nonce"}))

	_, err = manager.CompleteLogin(context.Background(), "code", session.State, sessionToken, "agent", "ip")
	if err == nil {
		t.Fatal("CompleteLogin error = nil, want ID token verification failure")
	}
	if resolver.called != 0 || issuer.called != 0 {
		t.Fatalf("resolver/issuer called %d/%d times, want 0 (fail before resolve)", resolver.called, issuer.called)
	}
}

func TestCompleteLoginFailsWhenSubjectNotPrelinked(t *testing.T) {
	idp := newSyntheticIdP(t)
	resolver := &stubIdentityResolver{err: ErrSubjectNotPrelinked}
	issuer := &stubSessionIssuer{session: Session{AccessToken: "access"}}
	manager := newSessionManagerForTest(t, idp, resolver, issuer)

	_, sessionToken, err := manager.provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := manager.signer().verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	idp.SetIDToken(idp.signIDToken(t, map[string]any{"nonce": session.Nonce}))

	_, err = manager.CompleteLogin(context.Background(), "code", session.State, sessionToken, "agent", "ip")
	if err == nil || !errors.Is(err, ErrSubjectNotPrelinked) {
		t.Fatalf("CompleteLogin error = %v, want ErrSubjectNotPrelinked", err)
	}
	if issuer.called != 0 {
		t.Fatalf("issuer called %d times, want 0 (no session for unlinked subject)", issuer.called)
	}
}

func TestCompleteLoginFailsWhenUserDisabled(t *testing.T) {
	idp := newSyntheticIdP(t)
	resolver := &stubIdentityResolver{user: LocalUser{ID: 7, Status: "disabled"}}
	issuer := &stubSessionIssuer{session: Session{AccessToken: "access"}}
	manager := newSessionManagerForTest(t, idp, resolver, issuer)

	_, sessionToken, err := manager.provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := manager.signer().verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	idp.SetIDToken(idp.signIDToken(t, map[string]any{"nonce": session.Nonce}))

	_, err = manager.CompleteLogin(context.Background(), "code", session.State, sessionToken, "agent", "ip")
	if err == nil || !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("CompleteLogin error = %v, want ErrUserDisabled", err)
	}
	if issuer.called != 0 {
		t.Fatalf("issuer called %d times, want 0 (no session for disabled user)", issuer.called)
	}
}

func TestCompleteLoginPropagatesSessionIssuerError(t *testing.T) {
	idp := newSyntheticIdP(t)
	resolver := &stubIdentityResolver{user: LocalUser{ID: 7, Status: "active"}}
	issuerErr := errors.New("session issuer unavailable")
	issuer := &stubSessionIssuer{err: issuerErr}
	manager := newSessionManagerForTest(t, idp, resolver, issuer)

	_, sessionToken, err := manager.provider.AuthorizationURL(context.Background())
	if err != nil {
		t.Fatalf("AuthorizationURL error = %v", err)
	}
	session, err := manager.signer().verify(sessionToken)
	if err != nil {
		t.Fatalf("verify session token: %v", err)
	}
	idp.SetIDToken(idp.signIDToken(t, map[string]any{"nonce": session.Nonce}))

	_, err = manager.CompleteLogin(context.Background(), "code", session.State, sessionToken, "agent", "ip")
	if err == nil || !errors.Is(err, issuerErr) {
		t.Fatalf("CompleteLogin error = %v, want issuer error", err)
	}
}

func TestSessionManagerReauthenticationInterval(t *testing.T) {
	manager := NewSessionManager(&Provider{}, &stubIdentityResolver{}, &stubSessionIssuer{}, SessionManagerConfig{Reauthentication: 30 * time.Minute})
	if got := manager.Reauthentication(); got != 30*time.Minute {
		t.Fatalf("Reauthentication = %v, want 30m", got)
	}
	// Zero reauthentication defaults to one hour.
	defaultManager := NewSessionManager(&Provider{}, &stubIdentityResolver{}, &stubSessionIssuer{}, SessionManagerConfig{})
	if got := defaultManager.Reauthentication(); got != time.Hour {
		t.Fatalf("default Reauthentication = %v, want 1h", got)
	}
}

// signer exposes the provider's auth-session signer for test setup.
func (m *SessionManager) signer() *authSessionSigner { return m.provider.signer }

func TestProviderLogoutURLBuildsEndSessionEndpoint(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)

	logoutURL, err := provider.LogoutURL(context.Background(), "id-token-hint", "https://platform.example.com/post-logout")
	if err != nil {
		t.Fatalf("LogoutURL error = %v", err)
	}
	if !strings.HasPrefix(logoutURL, idp.issuer+"/logout") {
		t.Fatalf("logoutURL = %q, want prefix %q", logoutURL, idp.issuer+"/logout")
	}
	if !strings.Contains(logoutURL, "id_token_hint=id-token-hint") {
		t.Fatalf("logoutURL missing id_token_hint: %q", logoutURL)
	}
	if !strings.Contains(logoutURL, "post_logout_redirect_uri=https") {
		t.Fatalf("logoutURL missing post_logout_redirect_uri: %q", logoutURL)
	}
}

func TestProviderLogoutURLOmitsEmptyParameters(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)

	logoutURL, err := provider.LogoutURL(context.Background(), "", "")
	if err != nil {
		t.Fatalf("LogoutURL error = %v", err)
	}
	if strings.Contains(logoutURL, "?") {
		t.Fatalf("logoutURL should have no query when both params empty: %q", logoutURL)
	}
}

func TestProviderLogoutURLRejectsHTTPPostLogoutRedirect(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)

	if _, err := provider.LogoutURL(context.Background(), "", "http://insecure.example.com/post-logout"); err == nil {
		t.Fatal("LogoutURL error = nil, want HTTPS post_logout_redirect_uri error")
	}
}

func TestStripBearerPrefix(t *testing.T) {
	cases := map[string]string{
		"Bearer token":     "token",
		"bearer token":     "token",
		"  Bearer  token ": "token",
		"token":            "token",
		"":                 "",
	}
	for input, want := range cases {
		if got := StripBearerPrefix(input); got != want {
			t.Fatalf("StripBearerPrefix(%q) = %q, want %q", input, got, want)
		}
	}
}

// stubBreakGlassAuditor records break-glass events for assertion.
type stubBreakGlassAuditor struct {
	mu     sync.Mutex
	events []BreakGlassEvent
	err    error
}

func (a *stubBreakGlassAuditor) RecordBreakGlassLogin(ctx context.Context, event BreakGlassEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	a.events = append(a.events, event)
	return nil
}

func (a *stubBreakGlassAuditor) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.events)
}

func TestBreakGlassDrillTrackerRecordsEvents(t *testing.T) {
	auditor := &stubBreakGlassAuditor{}
	tracker := NewBreakGlassDrillTracker(BreakGlassDrillConfig{RequiredInterval: 24 * time.Hour, MaxAccounts: 2}, auditor)

	if tracker.IsCurrent() {
		t.Fatal("IsCurrent = true before any drill")
	}

	err := tracker.Record(context.Background(), BreakGlassEvent{
		UserID: 1, Username: "breakglass", UserAgent: "agent", IPAddress: "127.0.0.1", Reason: "monthly drill",
	})
	if err != nil {
		t.Fatalf("Record error = %v", err)
	}
	if !tracker.IsCurrent() {
		t.Fatal("IsCurrent = false after a drill within the interval")
	}
	if auditor.count() != 1 {
		t.Fatalf("auditor recorded %d events, want 1", auditor.count())
	}
}

func TestBreakGlassDrillTrackerRequiresReason(t *testing.T) {
	auditor := &stubBreakGlassAuditor{}
	tracker := NewBreakGlassDrillTracker(BreakGlassDrillConfig{}, auditor)

	cases := map[string]BreakGlassEvent{
		"missing reason":   {UserID: 1, Username: "bg", Reason: ""},
		"missing user id":  {UserID: 0, Username: "bg", Reason: "drill"},
		"missing username": {UserID: 1, Username: "", Reason: "drill"},
	}
	for name, event := range cases {
		t.Run(name, func(t *testing.T) {
			if err := tracker.Record(context.Background(), event); err == nil {
				t.Fatalf("Record error = nil, want validation error for %q", name)
			}
		})
	}
	if auditor.count() != 0 {
		t.Fatalf("auditor recorded %d events, want 0 (all rejected)", auditor.count())
	}
}

func TestBreakGlassDrillTrackerPropagatesAuditorError(t *testing.T) {
	auditorErr := errors.New("audit trail unavailable")
	auditor := &stubBreakGlassAuditor{err: auditorErr}
	tracker := NewBreakGlassDrillTracker(BreakGlassDrillConfig{}, auditor)

	err := tracker.Record(context.Background(), BreakGlassEvent{UserID: 1, Username: "bg", Reason: "drill"})
	if err == nil || !errors.Is(err, auditorErr) {
		t.Fatalf("Record error = %v, want auditor error", err)
	}
	// The tracker must not mark itself current when the audit write failed.
	if tracker.IsCurrent() {
		t.Fatal("IsCurrent = true after auditor failure")
	}
}

func TestBreakGlassDrillTrackerReportsStaleAfterInterval(t *testing.T) {
	auditor := &stubBreakGlassAuditor{}
	tracker := &BreakGlassDrillTracker{
		cfg:     BreakGlassDrillConfig{RequiredInterval: time.Hour, MaxAccounts: 2},
		auditor: auditor,
		now:     func() time.Time { return time.Now() },
	}
	// Simulate a drill recorded in the past.
	past := time.Now().Add(-2 * time.Hour)
	tracker.mu.Lock()
	tracker.last = past
	tracker.mu.Unlock()
	if tracker.IsCurrent() {
		t.Fatal("IsCurrent = true after the drill interval has elapsed")
	}
	if tracker.LastDrillAt() != past {
		t.Fatalf("LastDrillAt = %v, want %v", tracker.LastDrillAt(), past)
	}
}

func TestBreakGlassDrillTrackerDefaultsConfig(t *testing.T) {
	tracker := NewBreakGlassDrillTracker(BreakGlassDrillConfig{}, nil)
	if tracker.RequiredInterval() != 7*24*time.Hour {
		t.Fatalf("default RequiredInterval = %v, want 7d", tracker.RequiredInterval())
	}
	if tracker.MaxAccounts() != 2 {
		t.Fatalf("default MaxAccounts = %d, want 2", tracker.MaxAccounts())
	}
	// MaxAccounts is capped at 2 even when over-configured.
	big := NewBreakGlassDrillTracker(BreakGlassDrillConfig{MaxAccounts: 5}, nil)
	if big.MaxAccounts() != 2 {
		t.Fatalf("capped MaxAccounts = %d, want 2", big.MaxAccounts())
	}
}

func TestBreakGlassDrillTrackerWorksWithoutAuditor(t *testing.T) {
	tracker := NewBreakGlassDrillTracker(BreakGlassDrillConfig{RequiredInterval: time.Hour}, nil)
	if err := tracker.Record(context.Background(), BreakGlassEvent{UserID: 1, Username: "bg", Reason: "drill"}); err != nil {
		t.Fatalf("Record error = %v", err)
	}
	if !tracker.IsCurrent() {
		t.Fatal("IsCurrent = false after recording without an auditor")
	}
}
