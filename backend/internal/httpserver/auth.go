package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/requestctx"
)

const refreshCookieName = "aiops_refresh_token"

type authHandler struct {
	service       *auth.Service
	secureCookies bool
	refreshTTL    time.Duration
}

type loginRequest struct {
	Username string `json:"username" binding:"required,min=3,max=64"`
	Password string `json:"password" binding:"required,min=8,max=128"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required,min=8,max=128"`
	NewPassword     string `json:"new_password" binding:"required,min=12,max=128"`
}

func (h authHandler) login(c *gin.Context) {
	var request loginRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "username and password are required")
		return
	}
	session, err := h.service.Login(c.Request.Context(), request.Username, request.Password, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInvalidCredentials):
			writeError(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "username or password is incorrect")
		case errors.Is(err, auth.ErrUserDisabled):
			writeError(c, http.StatusForbidden, "USER_DISABLED", "the user account is disabled")
		default:
			writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to create session")
		}
		return
	}
	setAuditActor(c, session.User.ID, session.User.Username, session.User.DisplayName, session.User.Roles)
	h.setRefreshCookie(c, session.RefreshToken())
	c.JSON(http.StatusOK, session)
}

func (h authHandler) refresh(c *gin.Context) {
	raw, err := c.Cookie(refreshCookieName)
	if err != nil {
		writeError(c, http.StatusUnauthorized, "REFRESH_TOKEN_REQUIRED", "refresh session is missing")
		return
	}
	session, err := h.service.Refresh(c.Request.Context(), raw, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		h.clearRefreshCookie(c)
		if errors.Is(err, auth.ErrUserDisabled) {
			writeError(c, http.StatusForbidden, "USER_DISABLED", "the user account is disabled")
		} else {
			writeError(c, http.StatusUnauthorized, "REFRESH_TOKEN_INVALID", "refresh session is invalid or expired")
		}
		return
	}
	setAuditActor(c, session.User.ID, session.User.Username, session.User.DisplayName, session.User.Roles)
	h.setRefreshCookie(c, session.RefreshToken())
	c.JSON(http.StatusOK, session)
}

func (h authHandler) logout(c *gin.Context) {
	raw, _ := c.Cookie(refreshCookieName)
	if err := h.service.Logout(c.Request.Context(), raw); err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to revoke session")
		return
	}
	h.clearRefreshCookie(c)
	c.Status(http.StatusNoContent)
}

func (h authHandler) me(c *gin.Context) {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	c.JSON(http.StatusOK, auth.UserView{
		ID:          metadata.ActorID,
		Username:    metadata.ActorName,
		DisplayName: metadata.ActorDisplayName,
		Roles:       metadata.Roles,
	})
}

func (h authHandler) changePassword(c *gin.Context) {
	var request changePasswordRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "current_password and a 12-128 character new_password are required")
		return
	}
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	err := h.service.ChangePassword(c.Request.Context(), metadata.ActorID, request.CurrentPassword, request.NewPassword)
	switch {
	case err == nil:
		h.clearRefreshCookie(c)
		c.Status(http.StatusNoContent)
	case errors.Is(err, auth.ErrCurrentPasswordInvalid):
		writeError(c, http.StatusUnauthorized, "CURRENT_PASSWORD_INVALID", "current password is incorrect")
	case errors.Is(err, auth.ErrPasswordUnchanged):
		writeError(c, http.StatusConflict, "PASSWORD_UNCHANGED", "new password must differ from current password")
	case errors.Is(err, auth.ErrInvalidUser):
		writeError(c, http.StatusBadRequest, "INVALID_NEW_PASSWORD", "new password must be between 12 and 128 characters")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to change password")
	}
}

func (h authHandler) sessions(c *gin.Context) {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	raw, _ := c.Cookie(refreshCookieName)
	items, err := h.service.ListSessions(c.Request.Context(), metadata.ActorID, raw)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to list sessions")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items), "remaining": 0})
}

func (h authHandler) revokeSession(c *gin.Context) {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	sessionID, err := strconv.ParseInt(c.Param("session_id"), 10, 64)
	if err != nil || sessionID < 1 {
		writeError(c, http.StatusBadRequest, "INVALID_SESSION_ID", "session_id must be a positive integer")
		return
	}
	setAuditTarget(c, "Session", "", strconv.FormatInt(sessionID, 10))
	raw, _ := c.Cookie(refreshCookieName)
	err = h.service.RevokeSession(c.Request.Context(), metadata.ActorID, sessionID, raw)
	switch {
	case err == nil:
		c.Status(http.StatusNoContent)
	case errors.Is(err, auth.ErrSessionNotFound):
		writeError(c, http.StatusNotFound, "SESSION_NOT_FOUND", "session does not exist or is no longer active")
	case errors.Is(err, auth.ErrCurrentSessionProtected):
		writeError(c, http.StatusConflict, "CURRENT_SESSION_PROTECTED", "use logout to revoke the current session")
	case errors.Is(err, auth.ErrCurrentSessionRequired):
		writeError(c, http.StatusConflict, "CURRENT_SESSION_REQUIRED", "current refresh session is required")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to revoke session")
	}
}

func (h authHandler) revokeOtherSessions(c *gin.Context) {
	metadata, _ := requestctx.MetadataFrom(c.Request.Context())
	setAuditTarget(c, "Session", "", "other-sessions")
	raw, _ := c.Cookie(refreshCookieName)
	count, err := h.service.RevokeOtherSessions(c.Request.Context(), metadata.ActorID, raw)
	if errors.Is(err, auth.ErrCurrentSessionRequired) {
		writeError(c, http.StatusConflict, "CURRENT_SESSION_REQUIRED", "current refresh session is required")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "unable to revoke other sessions")
		return
	}
	c.JSON(http.StatusOK, gin.H{"revoked": count})
}

func (h authHandler) setRefreshCookie(c *gin.Context, value string) {
	http.SetCookie(c.Writer, &http.Cookie{ // #nosec G124 -- Secure follows config (local http demo); HttpOnly+SameSite always on
		Name:     refreshCookieName,
		Value:    value,
		Path:     "/api/v1/auth",
		MaxAge:   int(h.refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func (h authHandler) clearRefreshCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{ // #nosec G124 -- Secure follows config (local http demo); HttpOnly+SameSite always on
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func withAuthentication(service *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			writeError(c, http.StatusUnauthorized, "ACCESS_TOKEN_REQUIRED", "a bearer access token is required")
			return
		}
		claims, err := service.Authenticate(c.Request.Context(), parts[1])
		if err != nil {
			if errors.Is(err, auth.ErrUserDisabled) {
				writeError(c, http.StatusForbidden, "USER_DISABLED", "the user account is disabled")
			} else {
				writeError(c, http.StatusUnauthorized, "ACCESS_TOKEN_INVALID", "access token is invalid or expired")
			}
			return
		}
		actorID, _ := strconv.ParseInt(claims.Subject, 10, 64)
		metadata, _ := requestctx.MetadataFrom(c.Request.Context())
		metadata.ActorID = actorID
		metadata.ActorName = claims.Username
		metadata.ActorDisplayName = claims.DisplayName
		metadata.Roles = append([]string(nil), claims.Roles...)
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Next()
	}
}

func requireRoles(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}
	return func(c *gin.Context) {
		metadata, _ := requestctx.MetadataFrom(c.Request.Context())
		for _, role := range metadata.Roles {
			if _, ok := allowed[role]; ok {
				c.Next()
				return
			}
		}
		writeError(c, http.StatusForbidden, "PERMISSION_DENIED", "the current user does not have permission")
	}
}
