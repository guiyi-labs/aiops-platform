package httpserver

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/oidc"
)

// oidcAuthSessionTTL bounds how long the auth-session cookie is valid. It must
// be short (the PKCE flow is interactive) but long enough for the user to
// authenticate with the provider.
const oidcAuthSessionTTL = 10 * time.Minute

// oidcHandler wires the OIDC provider and session manager into the HTTP API.
// It exposes three routes under /api/v1/auth/oidc:
//   - GET  /login    redirects to the provider authorization endpoint after
//     setting the short-lived auth-session cookie.
//   - GET  /callback completes the Authorization Code + PKCE flow, issues a
//     local session and sets the refresh-token cookie.
//   - POST /logout   performs RP-initiated logout by returning the provider
//     end_session endpoint URL.
//
// The handler is only registered when OIDC is enabled in configuration, so a
// nil SessionManager means the routes are absent and the server behaves as a
// local-only deployment. The handler embeds authHandler so it can reuse the
// shared refresh-token cookie channel and keep cookie attributes consistent
// with password login.
type oidcHandler struct {
	authHandler
	manager        *oidc.SessionManager
	authSessionTTL time.Duration
	postLogoutURI  string
}

func (h oidcHandler) login(c *gin.Context) {
	authorizationURL, sessionToken, err := h.manager.Provider().AuthorizationURL(c.Request.Context())
	if err != nil {
		writeError(c, http.StatusBadGateway, "OIDC_UNAVAILABLE", "unable to contact the identity provider")
		return
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oidc.AuthSessionCookieName,
		Value:    sessionToken,
		Path:     "/api/v1/auth/oidc",
		MaxAge:   int(h.authSessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	c.Redirect(http.StatusFound, authorizationURL)
}

func (h oidcHandler) callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	if code == "" || state == "" {
		writeError(c, http.StatusBadRequest, "OIDC_CALLBACK_INVALID", "code and state are required")
		return
	}
	cookie, err := c.Cookie(oidc.AuthSessionCookieName)
	if err != nil {
		writeError(c, http.StatusBadRequest, "OIDC_SESSION_EXPIRED", "the OIDC login session expired; try again")
		return
	}
	h.clearAuthSessionCookie(c)
	session, err := h.manager.CompleteLogin(c.Request.Context(), code, state, cookie, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, oidc.ErrUserDisabled), errors.Is(err, auth.ErrUserDisabled):
			writeError(c, http.StatusForbidden, "USER_DISABLED", "the user account is disabled")
		case errors.Is(err, oidc.ErrSubjectNotPrelinked):
			writeError(c, http.StatusForbidden, "OIDC_SUBJECT_NOT_PRELINKED", "the identity is not linked to a local account")
		default:
			writeError(c, http.StatusBadGateway, "OIDC_LOGIN_FAILED", "unable to complete OIDC login")
		}
		return
	}
	setAuditActor(c, session.User.ID, session.User.Username, session.User.DisplayName, session.User.Roles)
	h.setRefreshCookie(c, session.RefreshToken)
	c.JSON(http.StatusOK, oidcSessionResponse{
		AccessToken:          session.AccessToken,
		TokenType:            session.TokenType,
		AccessTokenExpiresIn: session.AccessTokenExpiresIn,
		User: auth.UserView{
			ID:          session.User.ID,
			Username:    session.User.Username,
			DisplayName: session.User.DisplayName,
			Roles:       session.User.Roles,
		},
	})
}

func (h oidcHandler) logout(c *gin.Context) {
	idTokenHint := strings.TrimSpace(c.GetHeader("X-OIDC-ID-Token-Hint"))
	logoutURL, err := h.manager.Provider().LogoutURL(c.Request.Context(), idTokenHint, h.postLogoutURI)
	if err != nil {
		writeError(c, http.StatusBadGateway, "OIDC_LOGOUT_UNAVAILABLE", "the identity provider does not advertise an end_session endpoint")
		return
	}
	c.JSON(http.StatusOK, gin.H{"logout_url": logoutURL})
}

func (h oidcHandler) clearAuthSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     oidc.AuthSessionCookieName,
		Value:    "",
		Path:     "/api/v1/auth/oidc",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

type oidcSessionResponse struct {
	AccessToken          string        `json:"access_token"`
	TokenType            string        `json:"token_type"`
	AccessTokenExpiresIn int64         `json:"expires_in"`
	User                 auth.UserView `json:"user"`
}
