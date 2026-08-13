package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/auth"
	"k8s-aiops.local/backend/internal/requestctx"
)

func newAuthRouter(stub *userRepositoryStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	service := auth.NewService(stub, auth.NewPasswordHasher(), auth.NewTokenManager("test-secret-key-32-bytes!!", 15*time.Minute), 24*time.Hour)
	h := authHandler{service: service, secureCookies: false, refreshTTL: 24 * time.Hour}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorName: "admin", ActorDisplayName: "Admin", Roles: []string{auth.SystemAdmin}, RequestID: "auth-test",
		}))
		c.Next()
	})
	router.POST("/api/v1/auth/login", h.login)
	router.POST("/api/v1/auth/refresh", h.refresh)
	router.POST("/api/v1/auth/logout", h.logout)
	router.GET("/api/v1/auth/me", h.me)
	router.POST("/api/v1/auth/change-password", h.changePassword)
	router.GET("/api/v1/auth/sessions", h.sessions)
	router.DELETE("/api/v1/auth/sessions/:session_id", h.revokeSession)
	router.POST("/api/v1/auth/sessions/revoke-others", h.revokeOtherSessions)
	return router
}

var testLoginHash = func() string {
	hash, _ := auth.NewPasswordHasher().Hash("password123")
	return hash
}()

func activeUser(id int64) auth.User {
	return auth.User{ID: id, Username: "admin", DisplayName: "Admin", PasswordHash: testLoginHash, Status: auth.StatusActive, AuthVersion: 0, Roles: []auth.Role{{Code: auth.SystemAdmin}}}
}

// ---- login ----

func TestLoginSuccess(t *testing.T) {
	stub := &userRepositoryStub{user: activeUser(1), management: []auth.User{activeUser(1)}}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"username":"admin","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rec.Code, rec.Body.String())
	}
	if !contains(rec.Body.String(), `"access_token"`) {
		t.Fatalf("expected access_token in body: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), refreshCookieName) {
		t.Fatalf("expected refresh cookie set: %s", rec.Header().Get("Set-Cookie"))
	}
}

func TestLoginInvalidBind(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"username":"ab"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("expected 400 INVALID_REQUEST got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	stub := &userRepositoryStub{findErr: auth.ErrInvalidCredentials}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"username":"admin","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || !contains(rec.Body.String(), "INVALID_CREDENTIALS") {
		t.Fatalf("expected 401 INVALID_CREDENTIALS got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginUserDisabled(t *testing.T) {
	u := activeUser(1)
	u.Status = auth.StatusDisabled
	stub := &userRepositoryStub{user: u}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"username":"admin","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || !contains(rec.Body.String(), "USER_DISABLED") {
		t.Fatalf("expected 403 USER_DISABLED got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginUpdateLastLoginError(t *testing.T) {
	stub := &userRepositoryStub{user: activeUser(1), updateLoginErr: errors.New("db down")}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"username":"admin","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || !contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500 INTERNAL_ERROR got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---- refresh ----

func TestRefreshNoCookie(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || !contains(rec.Body.String(), "REFRESH_TOKEN_REQUIRED") {
		t.Fatalf("expected 401 REFRESH_TOKEN_REQUIRED got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRefreshSuccess(t *testing.T) {
	stub := &userRepositoryStub{user: activeUser(1)}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "some-valid-token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d: %s", rec.Code, rec.Body.String())
	}
	if !contains(rec.Body.String(), "access_token") {
		t.Fatalf("expected access_token: %s", rec.Body.String())
	}
}

func TestRefreshUserDisabled(t *testing.T) {
	stub := &userRepositoryStub{rotateErr: auth.ErrUserDisabled}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || !contains(rec.Body.String(), "USER_DISABLED") {
		t.Fatalf("expected 403 USER_DISABLED got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRefreshInvalid(t *testing.T) {
	stub := &userRepositoryStub{rotateErr: errors.New("bad token")}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "bad"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || !contains(rec.Body.String(), "REFRESH_TOKEN_INVALID") {
		t.Fatalf("expected 401 REFRESH_TOKEN_INVALID got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---- logout ----

func TestLogoutSuccess(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLogoutError(t *testing.T) {
	stub := &userRepositoryStub{revokeErr: errors.New("revoke fail")}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || !contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500 INTERNAL_ERROR got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---- me ----

func TestMe(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !contains(rec.Body.String(), "admin") {
		t.Fatalf("expected 200 with username got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---- changePassword ----

func TestChangePasswordInvalidBind(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"current_password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("expected 400 INVALID_REQUEST got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChangePasswordSuccess(t *testing.T) {
	stub := &userRepositoryStub{user: activeUser(1)}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"current_password":"password123","new_password":"newpassword1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChangePasswordInvalidCurrent(t *testing.T) {
	u := activeUser(1)
	stub := &userRepositoryStub{user: u}
	// Override the stub's ChangePassword to return the right error
	stub.changeErr = auth.ErrCurrentPasswordInvalid
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"current_password":"wrongpassword","new_password":"newpassword1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || !contains(rec.Body.String(), "CURRENT_PASSWORD_INVALID") {
		t.Fatalf("expected 401 CURRENT_PASSWORD_INVALID got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChangePasswordUnchanged(t *testing.T) {
	u := activeUser(1)
	stub := &userRepositoryStub{user: u}
	stub.changeErr = auth.ErrPasswordUnchanged
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"current_password":"password123","new_password":"newpassword1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict || !contains(rec.Body.String(), "PASSWORD_UNCHANGED") {
		t.Fatalf("expected 409 PASSWORD_UNCHANGED got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChangePasswordShortNewPassword(t *testing.T) {
	stub := &userRepositoryStub{user: activeUser(1)}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"current_password":"password123","new_password":"short"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("expected 400 INVALID_REQUEST (gin binding catches short password) got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChangePasswordDefaultError(t *testing.T) {
	stub := &userRepositoryStub{user: activeUser(1)}
	stub.changeErr = errors.New("db error")
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	body := `{"current_password":"password123","new_password":"newpassword1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || !contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500 INTERNAL_ERROR got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---- sessions ----

func TestSessionsSuccess(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	stub := &userRepositoryStub{
		sessions: []auth.RefreshToken{
			{ID: 1, UserAgent: "test", IPAddress: "127.0.0.1", TokenHash: "h1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		},
	}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sessions", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("expected 200 with total=1 got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSessionsError(t *testing.T) {
	stub := &userRepositoryStub{sessionsErr: errors.New("db fail")}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/sessions", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || !contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500 INTERNAL_ERROR got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---- revokeSession ----

func TestRevokeSessionInvalidID(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions/abc", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_SESSION_ID") {
		t.Fatalf("expected 400 INVALID_SESSION_ID got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeSessionZeroID(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions/0", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_SESSION_ID") {
		t.Fatalf("expected 400 INVALID_SESSION_ID got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeSessionSuccess(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions/42", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeSessionNotFound(t *testing.T) {
	stub := &userRepositoryStub{revokeForUserErr: auth.ErrSessionNotFound}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions/42", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound || !contains(rec.Body.String(), "SESSION_NOT_FOUND") {
		t.Fatalf("expected 404 SESSION_NOT_FOUND got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeSessionProtected(t *testing.T) {
	stub := &userRepositoryStub{revokeForUserErr: auth.ErrCurrentSessionProtected}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions/42", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict || !contains(rec.Body.String(), "CURRENT_SESSION_PROTECTED") {
		t.Fatalf("expected 409 CURRENT_SESSION_PROTECTED got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeSessionNoCookie(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions/42", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict || !contains(rec.Body.String(), "CURRENT_SESSION_REQUIRED") {
		t.Fatalf("expected 409 CURRENT_SESSION_REQUIRED got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeSessionDefaultError(t *testing.T) {
	stub := &userRepositoryStub{revokeForUserErr: errors.New("internal")}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/auth/sessions/42", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || !contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500 INTERNAL_ERROR got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---- revokeOtherSessions ----

func TestRevokeOtherSessionsSuccess(t *testing.T) {
	stub := &userRepositoryStub{revokeOthersCount: 3}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/revoke-others", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"revoked":3`) {
		t.Fatalf("expected 200 with revoked:3 got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeOtherSessionsNoCookie(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/revoke-others", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict || !contains(rec.Body.String(), "CURRENT_SESSION_REQUIRED") {
		t.Fatalf("expected 409 CURRENT_SESSION_REQUIRED got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRevokeOtherSessionsError(t *testing.T) {
	stub := &userRepositoryStub{revokeOthersErr: errors.New("fail")}
	router := newAuthRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions/revoke-others", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "token"})
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || !contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("expected 500 INTERNAL_ERROR got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---- withAuthentication ----

func newAuthMiddlewareRouter(stub *userRepositoryStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	service := auth.NewService(stub, auth.NewPasswordHasher(), auth.NewTokenManager("test-secret-key-32-bytes!!", 15*time.Minute), 24*time.Hour)
	router := gin.New()
	router.GET("/protected", withAuthentication(service), func(c *gin.Context) {
		metadata, _ := requestctx.MetadataFrom(c.Request.Context())
		c.JSON(200, gin.H{"actor": metadata.ActorName})
	})
	return router
}

func TestWithAuthNoHeader(t *testing.T) {
	stub := &userRepositoryStub{}
	router := newAuthMiddlewareRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || !contains(rec.Body.String(), "ACCESS_TOKEN_REQUIRED") {
		t.Fatalf("expected 401 ACCESS_TOKEN_REQUIRED got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWithAuthInvalidToken(t *testing.T) {
	stub := &userRepositoryStub{user: activeUser(1)}
	router := newAuthMiddlewareRouter(stub)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-string")
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || !contains(rec.Body.String(), "ACCESS_TOKEN_INVALID") {
		t.Fatalf("expected 401 ACCESS_TOKEN_INVALID got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWithAuthUserNotFound(t *testing.T) {
	stub := &userRepositoryStub{findErr: auth.ErrUserNotFound}
	router := newAuthMiddlewareRouter(stub)

	tm := auth.NewTokenManager("test-secret-key-32-bytes!!", 15*time.Minute)
	token, _, _ := tm.IssueAccessToken(auth.User{ID: 999, Username: "ghost", Status: auth.StatusActive, AuthVersion: 0})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || !contains(rec.Body.String(), "ACCESS_TOKEN_INVALID") {
		t.Fatalf("expected 401 ACCESS_TOKEN_INVALID got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWithAuthUserDisabled(t *testing.T) {
	u := activeUser(1)
	u.Status = auth.StatusDisabled
	stub := &userRepositoryStub{user: u}
	router := newAuthMiddlewareRouter(stub)

	tm := auth.NewTokenManager("test-secret-key-32-bytes!!", 15*time.Minute)
	token, _, _ := tm.IssueAccessToken(u)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || !contains(rec.Body.String(), "USER_DISABLED") {
		t.Fatalf("expected 403 USER_DISABLED got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWithAuthAuthVersionMismatch(t *testing.T) {
	u := activeUser(1)
	u.AuthVersion = 0
	stub := &userRepositoryStub{user: u}
	router := newAuthMiddlewareRouter(stub)

	tm := auth.NewTokenManager("test-secret-key-32-bytes!!", 15*time.Minute)
	// Issue token for a user at version 0
	token, _, _ := tm.IssueAccessToken(auth.User{ID: 1, Username: "admin", DisplayName: "Admin", Status: auth.StatusActive, AuthVersion: 0, Roles: []auth.Role{{Code: auth.SystemAdmin}}})

	// Simulate auth_version bump on the user
	u.AuthVersion = 1
	stub.user = u

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized || !contains(rec.Body.String(), "ACCESS_TOKEN_INVALID") {
		t.Fatalf("expected 401 ACCESS_TOKEN_INVALID got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestWithAuthSuccess(t *testing.T) {
	u := activeUser(1)
	stub := &userRepositoryStub{user: u}
	router := newAuthMiddlewareRouter(stub)

	tm := auth.NewTokenManager("test-secret-key-32-bytes!!", 15*time.Minute)
	token, _, _ := tm.IssueAccessToken(u)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !contains(rec.Body.String(), "admin") {
		t.Fatalf("expected 200 with actor got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---- requireRoles ----

func TestRequireRolesDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, Roles: []string{auth.Viewer}, RequestID: "roles-test",
		}))
		c.Next()
	})
	router.GET("/admin-only", requireRoles(auth.SystemAdmin), func(c *gin.Context) {
		c.String(200, "ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden || !contains(rec.Body.String(), "PERMISSION_DENIED") {
		t.Fatalf("expected 403 PERMISSION_DENIED got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequireRolesAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, Roles: []string{auth.SystemAdmin}, RequestID: "roles-test",
		}))
		c.Next()
	})
	router.GET("/admin-only", requireRoles(auth.SystemAdmin), func(c *gin.Context) {
		c.String(200, "ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin-only", nil)
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("expected 200 ok got %d: %s", rec.Code, rec.Body.String())
	}
}
