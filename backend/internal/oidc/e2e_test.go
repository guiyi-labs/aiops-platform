package oidc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestSyntheticIdPEndToEndLifecycle is the M36E synthetic IdP end-to-end gate.
// It proves the full OIDC lifecycle works together against a synthetic IdP
// without a real organization IdP: login, authorization (claim/group-to-role
// mapping + MFA evidence), signing-key rotation, identity disable, provider
// logout and break-glass drill audit.
//
// The subtests run in order and share fixtures so the lifecycle narrative
// accumulates state exactly as a production deployment would: the JWKS cache
// carries rotated keys forward, a disabled user stays disabled until restored,
// and the break-glass drill tracker retains its last-drill timestamp.
func TestSyntheticIdPEndToEndLifecycle(t *testing.T) {
	idp := newSyntheticIdP(t)
	provider := newProviderWithSyntheticIdP(t, idp)

	activeUser := LocalUser{
		ID:          42,
		Username:    "e2e-operator",
		DisplayName: "E2E Operator",
		Status:      "active",
		Roles:       []string{"system_admin"},
	}
	resolver := &stubIdentityResolver{user: activeUser}
	issuer := &stubSessionIssuer{session: Session{
		AccessToken:          "e2e-access-token",
		RefreshToken:         "e2e-refresh-token",
		TokenType:            "Bearer",
		AccessTokenExpiresIn: 900,
		User:                 activeUser,
	}}
	auditor := &stubBreakGlassAuditor{}
	tracker := NewBreakGlassDrillTracker(BreakGlassDrillConfig{
		RequiredInterval: 24 * time.Hour,
		MaxAccounts:      2,
	}, auditor)
	manager := NewSessionManager(provider, resolver, issuer, SessionManagerConfig{
		Reauthentication: time.Hour,
	})

	// driveLogin drives a full Authorization Code + PKCE flow through the
	// synthetic IdP and returns the issued local session. It fails the test
	// when any step errors.
	driveLogin := func(t *testing.T) Session {
		t.Helper()
		_, sessionToken, err := provider.AuthorizationURL(context.Background())
		if err != nil {
			t.Fatalf("AuthorizationURL error = %v", err)
		}
		session, err := provider.signer.verify(sessionToken)
		if err != nil {
			t.Fatalf("verify session token: %v", err)
		}
		idp.SetIDToken(idp.signIDToken(t, map[string]any{"nonce": session.Nonce}))
		result, err := manager.CompleteLogin(context.Background(), "e2e-code", session.State, sessionToken, "e2e-agent", "10.0.0.1")
		if err != nil {
			t.Fatalf("CompleteLogin error = %v", err)
		}
		return result
	}

	// driveCallback drives the provider-level callback (HandleCallback) and
	// returns the verified CallbackResult. It is used by the Authorization and
	// Rotation subtests to inspect claims, roles, MFA evidence and signing-key
	// behaviour without going through the session manager.
	driveCallback := func(t *testing.T) CallbackResult {
		t.Helper()
		_, sessionToken, err := provider.AuthorizationURL(context.Background())
		if err != nil {
			t.Fatalf("AuthorizationURL error = %v", err)
		}
		session, err := provider.signer.verify(sessionToken)
		if err != nil {
			t.Fatalf("verify session token: %v", err)
		}
		idp.SetIDToken(idp.signIDToken(t, map[string]any{"nonce": session.Nonce}))
		result, err := provider.HandleCallback(context.Background(), "e2e-code", session.State, sessionToken)
		if err != nil {
			t.Fatalf("HandleCallback error = %v", err)
		}
		return result
	}

	t.Run("Login", func(t *testing.T) {
		resolverCallsBefore := resolver.called
		issuerCallsBefore := issuer.called

		session := driveLogin(t)

		if session.AccessToken != "e2e-access-token" {
			t.Fatalf("AccessToken = %q, want e2e-access-token", session.AccessToken)
		}
		if session.RefreshToken != "e2e-refresh-token" {
			t.Fatalf("RefreshToken = %q, want e2e-refresh-token", session.RefreshToken)
		}
		if session.User.ID != 42 || session.User.Username != "e2e-operator" {
			t.Fatalf("session user = %#v, want id=42 username=e2e-operator", session.User)
		}
		if session.User.Status != "active" {
			t.Fatalf("session user status = %q, want active", session.User.Status)
		}
		if resolver.called != resolverCallsBefore+1 {
			t.Fatalf("resolver calls = %d, want %d", resolver.called, resolverCallsBefore+1)
		}
		if resolver.lastIssuer != idp.issuer || resolver.lastSubject != "sub-123" {
			t.Fatalf("resolver called with issuer=%q subject=%q, want %q / sub-123", resolver.lastIssuer, resolver.lastSubject, idp.issuer)
		}
		if issuer.called != issuerCallsBefore+1 {
			t.Fatalf("issuer calls = %d, want %d", issuer.called, issuerCallsBefore+1)
		}
		if issuer.lastUserID != 42 || issuer.lastUserAgent != "e2e-agent" || issuer.lastIPAddress != "10.0.0.1" {
			t.Fatalf("issuer called with user=%d agent=%q ip=%q, want 42 / e2e-agent / 10.0.0.1", issuer.lastUserID, issuer.lastUserAgent, issuer.lastIPAddress)
		}
	})

	t.Run("Authorization", func(t *testing.T) {
		// The callback result carries the verified subject, username, display
		// name, group-mapped roles and MFA evidence.
		result := driveCallback(t)
		if result.Subject != "sub-123" {
			t.Fatalf("Subject = %q, want sub-123", result.Subject)
		}
		if result.Username != "operator@example.com" {
			t.Fatalf("Username = %q, want operator@example.com", result.Username)
		}
		if result.DisplayName != "Platform Operator" {
			t.Fatalf("DisplayName = %q, want Platform Operator", result.DisplayName)
		}
		if len(result.Groups) != 1 || result.Groups[0] != "oidc-admins" {
			t.Fatalf("Groups = %#v, want [oidc-admins]", result.Groups)
		}
		if len(result.Roles) != 1 || result.Roles[0] != "system_admin" {
			t.Fatalf("Roles = %#v, want [system_admin]", result.Roles)
		}
		if result.MFAEvidence != "mfa" {
			t.Fatalf("MFAEvidence = %q, want mfa", result.MFAEvidence)
		}

		// MFA is enforced: a token whose acr evidence is empty fails closed.
		_, sessionToken, err := provider.AuthorizationURL(context.Background())
		if err != nil {
			t.Fatalf("AuthorizationURL error = %v", err)
		}
		session, err := provider.signer.verify(sessionToken)
		if err != nil {
			t.Fatalf("verify session token: %v", err)
		}
		idp.SetIDToken(idp.signIDToken(t, map[string]any{"nonce": session.Nonce, "acr": ""}))
		_, err = provider.HandleCallback(context.Background(), "e2e-code", session.State, sessionToken)
		if err == nil {
			t.Fatal("HandleCallback error = nil, want MFA evidence failure")
		}
		if !strings.Contains(err.Error(), "MFA") {
			t.Fatalf("error = %v, want MFA evidence failure", err)
		}
	})

	t.Run("Rotation", func(t *testing.T) {
		// Rotate the IdP signing key. The JWKS endpoint now publishes only the
		// new key; tokens signed with the retired key must fail closed after
		// the cache refresh drops it.
		retiredKey := idp.RotateKey(t, "rotated-key-1")

		// A new login signed with the rotated key succeeds, proving the JWKS
		// cache refreshed and picked up the new key.
		session := driveLogin(t)
		if session.User.ID != 42 {
			t.Fatalf("post-rotation session user = %d, want 42", session.User.ID)
		}

		// A callback where the IdP returns a token signed with the retired key
		// must fail closed: the cache refresh dropped the retired kid.
		_, sessionToken, err := provider.AuthorizationURL(context.Background())
		if err != nil {
			t.Fatalf("AuthorizationURL error = %v", err)
		}
		authSession, err := provider.signer.verify(sessionToken)
		if err != nil {
			t.Fatalf("verify session token: %v", err)
		}
		idp.SetIDToken(idp.signIDTokenWithKey(t, retiredKey, map[string]any{"nonce": authSession.Nonce}))
		_, err = provider.HandleCallback(context.Background(), "e2e-code", authSession.State, sessionToken)
		if err == nil {
			t.Fatal("HandleCallback error = nil, want retired-key verification failure")
		}
		if !errors.Is(err, ErrUnknownKey) {
			t.Fatalf("error = %v, want ErrUnknownKey", err)
		}
	})

	t.Run("Disable", func(t *testing.T) {
		// The prelinked user is disabled. OIDC login must fail closed even when
		// the ID token is valid.
		resolver.user.Status = "disabled"
		issuerCallsBefore := issuer.called

		_, sessionToken, err := provider.AuthorizationURL(context.Background())
		if err != nil {
			t.Fatalf("AuthorizationURL error = %v", err)
		}
		session, err := provider.signer.verify(sessionToken)
		if err != nil {
			t.Fatalf("verify session token: %v", err)
		}
		idp.SetIDToken(idp.signIDToken(t, map[string]any{"nonce": session.Nonce}))
		_, err = manager.CompleteLogin(context.Background(), "e2e-code", session.State, sessionToken, "e2e-agent", "10.0.0.1")
		if err == nil || !errors.Is(err, ErrUserDisabled) {
			t.Fatalf("error = %v, want ErrUserDisabled", err)
		}
		if issuer.called != issuerCallsBefore {
			t.Fatalf("issuer called for disabled user: calls = %d, want %d", issuer.called, issuerCallsBefore)
		}
		// Restore the user so the lifecycle narrative is not left in a broken state.
		resolver.user.Status = "active"
	})

	t.Run("Logout", func(t *testing.T) {
		// RP-initiated logout builds the provider end_session endpoint URL with
		// the id_token_hint and HTTPS post_logout_redirect_uri.
		logoutURL, err := provider.LogoutURL(context.Background(), "e2e-id-token-hint", "https://platform.example.com/post-logout")
		if err != nil {
			t.Fatalf("LogoutURL error = %v", err)
		}
		if !strings.HasPrefix(logoutURL, idp.issuer+"/logout") {
			t.Fatalf("logoutURL = %q, want prefix %q", logoutURL, idp.issuer+"/logout")
		}
		if !strings.Contains(logoutURL, "id_token_hint=e2e-id-token-hint") {
			t.Fatalf("logoutURL missing id_token_hint: %q", logoutURL)
		}
		if !strings.Contains(logoutURL, "post_logout_redirect_uri=https") {
			t.Fatalf("logoutURL missing post_logout_redirect_uri: %q", logoutURL)
		}
	})

	t.Run("BreakGlass", func(t *testing.T) {
		// The break-glass fallback is initially stale (no drill recorded).
		if tracker.IsCurrent() {
			t.Fatal("IsCurrent = true before any drill")
		}
		// A break-glass drill is recorded: the auditor receives the event and
		// the tracker reports the fallback as current.
		eventsBefore := auditor.count()
		event := BreakGlassEvent{
			UserID:    99,
			Username:  "breakglass",
			UserAgent: "e2e-agent",
			IPAddress: "10.0.0.99",
			Reason:    "monthly E2E drill",
		}
		if err := tracker.Record(context.Background(), event); err != nil {
			t.Fatalf("Record error = %v", err)
		}
		if auditor.count() != eventsBefore+1 {
			t.Fatalf("auditor recorded %d events, want %d", auditor.count(), eventsBefore+1)
		}
		if !tracker.IsCurrent() {
			t.Fatal("IsCurrent = false after recording a drill")
		}
		// The recorded event carries the high-priority audit fields.
		auditor.mu.Lock()
		if len(auditor.events) == 0 {
			auditor.mu.Unlock()
			t.Fatal("auditor recorded no events")
		}
		recorded := auditor.events[len(auditor.events)-1]
		auditor.mu.Unlock()
		if recorded.UserID != 99 || recorded.Username != "breakglass" {
			t.Fatalf("recorded event = %#v, want UserID=99 Username=breakglass", recorded)
		}
		if recorded.Reason != "monthly E2E drill" {
			t.Fatalf("recorded Reason = %q, want monthly E2E drill", recorded.Reason)
		}
		// A break-glass event without a reason is rejected so every use is
		// attributed to a drill or an outage.
		if err := tracker.Record(context.Background(), BreakGlassEvent{UserID: 99, Username: "breakglass", Reason: ""}); err == nil {
			t.Fatal("Record error = nil, want reason-required validation error")
		}
	})
}

// TestSyntheticIdPEndToEndBreakGlassStaleness proves the break-glass drill
// tracker reports stale when the drill interval has elapsed. It uses a custom
// clock so the E2E gate does not depend on real-time waiting.
func TestSyntheticIdPEndToEndBreakGlassStaleness(t *testing.T) {
	auditor := &stubBreakGlassAuditor{}
	now := time.Now()
	tracker := &BreakGlassDrillTracker{
		cfg:     BreakGlassDrillConfig{RequiredInterval: time.Hour, MaxAccounts: 2},
		auditor: auditor,
		now:     func() time.Time { return now },
	}
	// Record a drill at the current time.
	if err := tracker.Record(context.Background(), BreakGlassEvent{
		UserID: 7, Username: "bg", Reason: "drill",
	}); err != nil {
		t.Fatalf("Record error = %v", err)
	}
	if !tracker.IsCurrent() {
		t.Fatal("IsCurrent = false immediately after a drill")
	}
	// Advance time past the required interval: the fallback is now stale.
	now = now.Add(2 * time.Hour)
	if tracker.IsCurrent() {
		t.Fatal("IsCurrent = true after the drill interval elapsed")
	}
	if tracker.LastDrillAt().IsZero() {
		t.Fatal("LastDrillAt = zero, want the recorded drill timestamp")
	}
}
