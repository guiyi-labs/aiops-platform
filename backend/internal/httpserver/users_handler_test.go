package httpserver

import (
	"context"
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

type userRepositoryStub struct {
	active            []auth.User
	management        []auth.User
	created           auth.User
	createdErr        error
	updated           auth.User
	updateErr         error
	resetUser         auth.User
	resetErr          error
	listActiveErr     error
	listUsersErr      error
	user              auth.User
	findErr           error
	updateLoginErr    error
	rotateErr         error
	revokeErr         error
	changeErr         error
	sessions          []auth.RefreshToken
	sessionsErr       error
	revokeForUserErr  error
	revokeOthersCount int64
	revokeOthersErr   error
}

func (s *userRepositoryStub) CountUsers(context.Context) (int64, error) {
	return int64(len(s.management)), nil
}
func (s *userRepositoryStub) CreateBootstrapAdmin(context.Context, auth.User) error { return nil }
func (s *userRepositoryStub) FindUserByUsername(context.Context, string) (auth.User, error) {
	return s.user, s.findErr
}
func (s *userRepositoryStub) FindUserByID(context.Context, int64) (auth.User, error) {
	return s.user, s.findErr
}
func (s *userRepositoryStub) ListActiveUsers(context.Context) ([]auth.User, error) {
	return s.active, s.listActiveErr
}
func (s *userRepositoryStub) ListUsers(_ context.Context, limit int) ([]auth.User, int64, error) {
	if s.listUsersErr != nil {
		return nil, 0, s.listUsersErr
	}
	if limit < 1 || limit > len(s.management) {
		return s.management, int64(len(s.management)), nil
	}
	return s.management[:limit], int64(len(s.management)), nil
}
func (s *userRepositoryStub) CreateUser(_ context.Context, user auth.User, roles []string) (auth.User, error) {
	if s.createdErr != nil {
		return auth.User{}, s.createdErr
	}
	s.created = user
	s.created.ID = 11
	for _, code := range roles {
		s.created.Roles = append(s.created.Roles, auth.Role{Code: code})
	}
	return s.created, nil
}
func (s *userRepositoryStub) UpdateUser(context.Context, int64, auth.UserUpdate) (auth.User, error) {
	return s.updated, s.updateErr
}
func (s *userRepositoryStub) ResetPassword(context.Context, int64, string, time.Time) (auth.User, error) {
	return s.resetUser, s.resetErr
}
func (s *userRepositoryStub) ChangePassword(context.Context, int64, string, string, time.Time) error {
	return s.changeErr
}
func (s *userRepositoryStub) UpdateLastLogin(context.Context, int64, time.Time) error {
	return s.updateLoginErr
}
func (s *userRepositoryStub) CreateRefreshToken(context.Context, auth.RefreshToken) error { return nil }
func (s *userRepositoryStub) RotateRefreshToken(context.Context, string, auth.RefreshToken, time.Time) (auth.User, error) {
	if s.rotateErr != nil {
		return auth.User{}, s.rotateErr
	}
	return s.user, nil
}
func (s *userRepositoryStub) RevokeRefreshToken(context.Context, string, time.Time) error {
	return s.revokeErr
}
func (s *userRepositoryStub) ListRefreshTokens(context.Context, int64, time.Time) ([]auth.RefreshToken, error) {
	return s.sessions, s.sessionsErr
}
func (s *userRepositoryStub) RevokeRefreshTokenForUser(context.Context, int64, int64, string, time.Time) error {
	return s.revokeForUserErr
}
func (s *userRepositoryStub) RevokeOtherRefreshTokens(context.Context, int64, string, time.Time) (int64, error) {
	return s.revokeOthersCount, s.revokeOthersErr
}

func newUsersRouter(stub *userRepositoryStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	service := auth.NewService(stub, auth.NewPasswordHasher(), auth.NewTokenManager("test-secret-key-32-bytes!!", 15*time.Minute), 24*time.Hour)
	h := userHandler{service: service}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), requestctx.Metadata{
			ActorID: 1, ActorDisplayName: "Admin", Roles: []string{auth.SystemAdmin}, RequestID: "users-test",
		}))
		c.Next()
	})
	router.GET("/api/v1/users/assignable", h.assignable)
	router.GET("/api/v1/users", h.list)
	router.POST("/api/v1/users", h.create)
	router.PATCH("/api/v1/users/:user_id", h.update)
	router.POST("/api/v1/users/:user_id/password-reset", h.resetPassword)
	return router
}

func managedUser(id int64, username string) auth.User {
	return auth.User{ID: id, Username: username, DisplayName: "Playwright Admin", Status: auth.StatusActive, Roles: []auth.Role{{Code: auth.SystemAdmin}}}
}

func TestUsersAssignableAndList(t *testing.T) {
	stub := &userRepositoryStub{active: []auth.User{managedUser(1, "admin"), managedUser(2, "ops")}, management: []auth.User{managedUser(1, "admin"), managedUser(2, "ops")}}
	router := newUsersRouter(stub)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/assignable", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"username":"ops"`) || !contains(rec.Body.String(), `"total":2`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users?limit=1", nil))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"total":2`) || !contains(rec.Body.String(), `"remaining":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid limit
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users?limit=0", nil))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_QUERY") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// repo failure surfaces 500
	stub = &userRepositoryStub{listActiveErr: errors.New("db down")}
	rec = httptest.NewRecorder()
	newUsersRouter(stub).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/assignable", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUsersCreate(t *testing.T) {
	body := `{"username":"ops.engineer","password":"super-secret-123","display_name":"Ops Engineer","roles":["operations_admin"]}`
	router := newUsersRouter(&userRepositoryStub{})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body)))
	if rec.Code != http.StatusCreated || !contains(rec.Body.String(), `"username":"ops.engineer"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// missing required field
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"username":"ops"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// service-level validation failure
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(`{"username":"bad name!","password":"short","display_name":"x","roles":["nope"]}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_USER") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// duplicate username
	router = newUsersRouter(&userRepositoryStub{createdErr: auth.ErrUsernameExists})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body)))
	if rec.Code != http.StatusConflict || !contains(rec.Body.String(), "USERNAME_EXISTS") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// generic failure
	router = newUsersRouter(&userRepositoryStub{createdErr: errors.New("db down")})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users", strings.NewReader(body)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUsersUpdate(t *testing.T) {
	router := newUsersRouter(&userRepositoryStub{updated: managedUser(2, "ops")})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/users/2", strings.NewReader(`{"display_name":"Ops Updated"}`)))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"username":"ops"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid id
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/users/0", strings.NewReader(`{"display_name":"x"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_USER_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// empty body
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/users/2", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_USER") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "invalid user", err: auth.ErrInvalidUser, status: http.StatusBadRequest, code: "INVALID_USER"},
		{name: "not found", err: auth.ErrUserNotFound, status: http.StatusNotFound, code: "USER_NOT_FOUND"},
		{name: "self protection", err: auth.ErrSelfProtection, status: http.StatusConflict, code: "SELF_PROTECTION"},
		{name: "last admin", err: auth.ErrLastSystemAdmin, status: http.StatusConflict, code: "LAST_SYSTEM_ADMIN"},
		{name: "generic", err: errors.New("db down"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, tt := range cases {
		router := newUsersRouter(&userRepositoryStub{updateErr: tt.err})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPatch, "/api/v1/users/2", strings.NewReader(`{"status":"disabled"}`)))
		if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
			t.Fatalf("%s: status=%d body=%s want status=%d code=%s", tt.name, rec.Code, rec.Body.String(), tt.status, tt.code)
		}
	}
}

func TestUsersResetPassword(t *testing.T) {
	router := newUsersRouter(&userRepositoryStub{resetUser: managedUser(2, "ops")})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users/2/password-reset", strings.NewReader(`{"password":"new-password-123"}`)))
	if rec.Code != http.StatusOK || !contains(rec.Body.String(), `"username":"ops"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// invalid id
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users/abc/password-reset", strings.NewReader(`{"password":"new-password-123"}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_USER_ID") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	// missing password
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users/2/password-reset", strings.NewReader(`{}`)))
	if rec.Code != http.StatusBadRequest || !contains(rec.Body.String(), "INVALID_REQUEST") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "weak password", err: auth.ErrInvalidUser, status: http.StatusBadRequest, code: "INVALID_PASSWORD"},
		{name: "not found", err: auth.ErrUserNotFound, status: http.StatusNotFound, code: "USER_NOT_FOUND"},
		{name: "self reset", err: auth.ErrSelfPasswordReset, status: http.StatusConflict, code: "SELF_PASSWORD_RESET"},
		{name: "generic", err: errors.New("db down"), status: http.StatusInternalServerError, code: "INTERNAL_ERROR"},
	}
	for _, tt := range cases {
		router := newUsersRouter(&userRepositoryStub{resetErr: tt.err})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/users/2/password-reset", strings.NewReader(`{"password":"new-password-123"}`)))
		if rec.Code != tt.status || !contains(rec.Body.String(), tt.code) {
			t.Fatalf("%s: status=%d body=%s want status=%d code=%s", tt.name, rec.Code, rec.Body.String(), tt.status, tt.code)
		}
	}
}
