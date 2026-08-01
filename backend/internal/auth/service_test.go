package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type repositoryStub struct {
	user            User
	users           []User
	refresh         RefreshToken
	refreshOwner    User
	rotateErr       error
	managementUsers []User
	createdUser     User
	createdRoles    []string
	updatedUser     User
	updated         UserUpdate
	updateErr       error
	resetUser       User
	resetHash       string
	resetErr        error
	changedExpected string
	changedHash     string
	changeErr       error
	sessions        []RefreshToken
	revokedSession  int64
	revokedCurrent  string
	revokedOthers   string
	findErr         error
}

func (r *repositoryStub) CountUsers(context.Context) (int64, error)        { return 1, nil }
func (r *repositoryStub) CreateBootstrapAdmin(context.Context, User) error { return nil }
func (r *repositoryStub) FindUserByUsername(context.Context, string) (User, error) {
	if r.user.ID == 0 {
		return User{}, ErrUserNotFound
	}
	return r.user, nil
}
func (r *repositoryStub) FindUserByID(context.Context, int64) (User, error) {
	if r.findErr != nil {
		return User{}, r.findErr
	}
	return r.user, nil
}
func (r *repositoryStub) ListActiveUsers(context.Context) ([]User, error) {
	if r.users != nil {
		return r.users, nil
	}
	return []User{r.user}, nil
}
func (r *repositoryStub) ListUsers(context.Context, int) ([]User, int64, error) {
	return r.managementUsers, int64(len(r.managementUsers)), nil
}
func (r *repositoryStub) CreateUser(_ context.Context, user User, roles []string) (User, error) {
	r.createdUser = user
	r.createdRoles = append([]string(nil), roles...)
	user.ID = 11
	for _, code := range roles {
		user.Roles = append(user.Roles, Role{Code: code})
	}
	return user, nil
}
func (r *repositoryStub) UpdateUser(_ context.Context, _ int64, update UserUpdate) (User, error) {
	r.updated = update
	if r.updateErr != nil {
		return User{}, r.updateErr
	}
	return r.updatedUser, nil
}
func (r *repositoryStub) ResetPassword(_ context.Context, _ int64, passwordHash string, _ time.Time) (User, error) {
	r.resetHash = passwordHash
	return r.resetUser, r.resetErr
}
func (r *repositoryStub) ChangePassword(_ context.Context, _ int64, expectedHash, passwordHash string, _ time.Time) error {
	r.changedExpected, r.changedHash = expectedHash, passwordHash
	return r.changeErr
}
func (r *repositoryStub) UpdateLastLogin(context.Context, int64, time.Time) error { return nil }
func (r *repositoryStub) CreateRefreshToken(_ context.Context, token RefreshToken) error {
	r.refresh = token
	return nil
}
func (r *repositoryStub) RotateRefreshToken(context.Context, string, RefreshToken, time.Time) (User, error) {
	return r.refreshOwner, r.rotateErr
}
func (r *repositoryStub) RevokeRefreshToken(context.Context, string, time.Time) error { return nil }
func (r *repositoryStub) ListRefreshTokens(context.Context, int64, time.Time) ([]RefreshToken, error) {
	return r.sessions, nil
}
func (r *repositoryStub) RevokeRefreshTokenForUser(_ context.Context, _ int64, sessionID int64, currentHash string, _ time.Time) error {
	r.revokedSession, r.revokedCurrent = sessionID, currentHash
	return nil
}
func (r *repositoryStub) RevokeOtherRefreshTokens(_ context.Context, _ int64, currentHash string, _ time.Time) (int64, error) {
	r.revokedOthers = currentHash
	return 2, nil
}

func TestServiceLoginAndAuthenticate(t *testing.T) {
	hasher := NewPasswordHasher()
	hash, err := hasher.Hash("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	repository := &repositoryStub{user: User{
		ID: 7, Username: "operator", DisplayName: "Platform Operator", PasswordHash: hash,
		Status: StatusActive, Roles: []Role{{Code: OperationsAdmin}},
	}}
	service := NewService(repository, hasher, NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)

	session, err := service.Login(context.Background(), "operator", "correct-password", "test-agent", "127.0.0.1")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if session.User.DisplayName != "Platform Operator" || session.RefreshToken() == "" {
		t.Fatalf("Login() session = %#v", session)
	}
	claims, err := service.Authenticate(context.Background(), session.AccessToken)
	if err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if claims.DisplayName != "Platform Operator" || len(claims.Roles) != 1 || claims.Roles[0] != OperationsAdmin {
		t.Fatalf("Authenticate() claims = %#v", claims)
	}
	if repository.refresh.TokenHash == session.RefreshToken() {
		t.Fatal("refresh token was persisted in plaintext")
	}
}

func TestAuthenticateUsesCurrentUserStatusAndRoles(t *testing.T) {
	manager := NewTokenManager("test-signing-key-that-is-long-enough", time.Minute)
	token, _, err := manager.IssueAccessToken(User{ID: 7, Username: "operator", DisplayName: "Old Name", AuthVersion: 2, Roles: []Role{{Code: OperationsAdmin}}})
	if err != nil {
		t.Fatal(err)
	}
	repository := &repositoryStub{user: User{ID: 7, Username: "operator", DisplayName: "Current Name", AuthVersion: 2, Status: StatusActive, Roles: []Role{{Code: Viewer}}}}
	service := NewService(repository, NewPasswordHasher(), manager, time.Hour)
	claims, err := service.Authenticate(context.Background(), token)
	if err != nil || claims.DisplayName != "Current Name" || len(claims.Roles) != 1 || claims.Roles[0] != Viewer {
		t.Fatalf("Authenticate() claims=%#v err=%v", claims, err)
	}
	repository.user.Status = StatusDisabled
	if _, err := service.Authenticate(context.Background(), token); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("Authenticate() disabled error=%v", err)
	}
}

func TestAuthenticateRejectsStaleCredentialVersion(t *testing.T) {
	manager := NewTokenManager("test-signing-key-that-is-long-enough", time.Minute)
	token, _, err := manager.IssueAccessToken(User{ID: 7, Username: "operator", AuthVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(&repositoryStub{user: User{ID: 7, Username: "operator", Status: StatusActive, AuthVersion: 2}}, NewPasswordHasher(), manager, time.Hour)
	if _, err := service.Authenticate(context.Background(), token); !errors.Is(err, ErrInvalidAccessToken) {
		t.Fatalf("Authenticate() error=%v, want ErrInvalidAccessToken", err)
	}
}

func TestServiceRejectsInvalidPassword(t *testing.T) {
	hasher := NewPasswordHasher()
	hash, _ := hasher.Hash("correct-password")
	service := NewService(&repositoryStub{user: User{ID: 1, PasswordHash: hash, Status: StatusActive}}, hasher,
		NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)

	_, err := service.Login(context.Background(), "admin", "wrong-password", "", "")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
}

func TestServiceRejectsRefreshForDisabledUser(t *testing.T) {
	service := NewService(&repositoryStub{rotateErr: ErrUserDisabled}, NewPasswordHasher(),
		NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)

	_, err := service.Refresh(context.Background(), "refresh-token", "", "")
	if !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("Refresh() error = %v, want ErrUserDisabled", err)
	}
}

func TestServiceFiltersAssignableUsers(t *testing.T) {
	service := NewService(&repositoryStub{users: []User{
		{ID: 1, Username: "sys", DisplayName: "System", Status: StatusActive, Roles: []Role{{Code: SystemAdmin}}},
		{ID: 2, Username: "ops", DisplayName: "Operations", Status: StatusActive, Roles: []Role{{Code: OperationsAdmin}}},
		{ID: 3, Username: "viewer", DisplayName: "Viewer", Status: StatusActive, Roles: []Role{{Code: Viewer}}},
		{ID: 4, Username: "disabled", DisplayName: "Disabled", Status: "disabled", Roles: []Role{{Code: OperationsAdmin}}},
	}}, NewPasswordHasher(), NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)

	users, err := service.AssignableUsers(context.Background())
	if err != nil {
		t.Fatalf("AssignableUsers() error = %v", err)
	}
	if len(users) != 2 || users[0].Username != "sys" || users[1].Username != "ops" {
		t.Fatalf("AssignableUsers() = %#v", users)
	}
}

func TestServiceCreatesManagedUserWithHashedPasswordAndSortedRoles(t *testing.T) {
	repository := &repositoryStub{}
	hasher := NewPasswordHasher()
	service := NewService(repository, hasher, NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)
	user, err := service.CreateUser(context.Background(), CreateUserInput{Username: "ops.user", Password: "a-secure-password", DisplayName: " Operations User ", Roles: []string{Viewer, OperationsAdmin, Viewer}})
	if err != nil {
		t.Fatalf("CreateUser() error=%v", err)
	}
	if user.ID != 11 || user.DisplayName != "Operations User" || len(repository.createdRoles) != 2 || repository.createdRoles[0] != OperationsAdmin || repository.createdRoles[1] != Viewer {
		t.Fatalf("user=%#v repository=%#v", user, repository)
	}
	if repository.createdUser.PasswordHash == "a-secure-password" || !hasher.Compare(repository.createdUser.PasswordHash, "a-secure-password") {
		t.Fatal("password was not securely hashed")
	}
}

func TestServiceRejectsInvalidManagedUserAndSelfProtection(t *testing.T) {
	repository := &repositoryStub{}
	service := NewService(repository, NewPasswordHasher(), NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)
	if _, err := service.CreateUser(context.Background(), CreateUserInput{Username: "Uppercase", Password: "short", DisplayName: "User", Roles: []string{Viewer}}); !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("CreateUser() error=%v", err)
	}
	disabled := StatusDisabled
	if _, err := service.UpdateUser(context.Background(), 7, 7, UpdateUserInput{Status: &disabled}); !errors.Is(err, ErrSelfProtection) {
		t.Fatalf("UpdateUser() error=%v", err)
	}
	roles := []string{Viewer}
	if _, err := service.UpdateUser(context.Background(), 7, 7, UpdateUserInput{Roles: &roles}); !errors.Is(err, ErrSelfProtection) {
		t.Fatalf("UpdateUser() role error=%v", err)
	}
}

func TestServicePropagatesLastAdministratorProtection(t *testing.T) {
	repository := &repositoryStub{updateErr: ErrLastSystemAdmin}
	service := NewService(repository, NewPasswordHasher(), NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)
	disabled := StatusDisabled
	if _, err := service.UpdateUser(context.Background(), 9, 1, UpdateUserInput{Status: &disabled}); !errors.Is(err, ErrLastSystemAdmin) {
		t.Fatalf("UpdateUser() error=%v", err)
	}
}

func TestServiceResetsPasswordWithHashAndSelfProtection(t *testing.T) {
	repository := &repositoryStub{resetUser: User{ID: 9, Username: "operator", DisplayName: "Operator", Status: StatusActive, AuthVersion: 2, Roles: []Role{{Code: Viewer}}}}
	hasher := NewPasswordHasher()
	service := NewService(repository, hasher, NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)
	user, err := service.ResetPassword(context.Background(), 9, 1, "replacement-password")
	if err != nil || user.ID != 9 {
		t.Fatalf("ResetPassword() user=%#v err=%v", user, err)
	}
	if repository.resetHash == "replacement-password" || !hasher.Compare(repository.resetHash, "replacement-password") {
		t.Fatal("reset password was not securely hashed")
	}
	if _, err := service.ResetPassword(context.Background(), 1, 1, "replacement-password"); !errors.Is(err, ErrSelfPasswordReset) {
		t.Fatalf("ResetPassword() self error=%v", err)
	}
	if _, err := service.ResetPassword(context.Background(), 9, 1, "short"); !errors.Is(err, ErrInvalidUser) {
		t.Fatalf("ResetPassword() short error=%v", err)
	}
}

func TestServiceChangesOwnPasswordAndRejectsInvalidCurrentValue(t *testing.T) {
	hasher := NewPasswordHasher()
	currentHash, _ := hasher.Hash("current-password")
	repository := &repositoryStub{user: User{ID: 7, Username: "operator", PasswordHash: currentHash, Status: StatusActive}}
	service := NewService(repository, hasher, NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)
	if err := service.ChangePassword(context.Background(), 7, "wrong-password", "replacement-password"); !errors.Is(err, ErrCurrentPasswordInvalid) {
		t.Fatalf("ChangePassword() current error=%v", err)
	}
	if err := service.ChangePassword(context.Background(), 7, "current-password", "current-password"); !errors.Is(err, ErrPasswordUnchanged) {
		t.Fatalf("ChangePassword() unchanged error=%v", err)
	}
	if err := service.ChangePassword(context.Background(), 7, "current-password", "replacement-password"); err != nil {
		t.Fatalf("ChangePassword() error=%v", err)
	}
	if repository.changedExpected != currentHash || repository.changedHash == "replacement-password" || !hasher.Compare(repository.changedHash, "replacement-password") {
		t.Fatal("password change did not use expected hash and new bcrypt hash")
	}
}

func TestServiceListsAndRevokesOtherSessionsWithoutExposingHashes(t *testing.T) {
	rawCurrent := "current-refresh-token"
	repository := &repositoryStub{sessions: []RefreshToken{
		{ID: 1, TokenHash: HashRefreshToken(rawCurrent), UserAgent: "current-agent", IPAddress: "127.0.0.1"},
		{ID: 2, TokenHash: HashRefreshToken("other-token"), UserAgent: "other-agent", IPAddress: "10.0.0.2"},
	}}
	service := NewService(repository, NewPasswordHasher(), NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)
	items, err := service.ListSessions(context.Background(), 7, rawCurrent)
	if err != nil || len(items) != 2 || !items[0].Current || items[1].Current {
		t.Fatalf("ListSessions() items=%#v err=%v", items, err)
	}
	if err := service.RevokeSession(context.Background(), 7, 2, rawCurrent); err != nil || repository.revokedSession != 2 || repository.revokedCurrent != HashRefreshToken(rawCurrent) {
		t.Fatalf("RevokeSession() state=%#v err=%v", repository, err)
	}
	count, err := service.RevokeOtherSessions(context.Background(), 7, rawCurrent)
	if err != nil || count != 2 || repository.revokedOthers != HashRefreshToken(rawCurrent) {
		t.Fatalf("RevokeOtherSessions() count=%d state=%#v err=%v", count, repository, err)
	}
	if err := service.RevokeSession(context.Background(), 7, 2, ""); !errors.Is(err, ErrCurrentSessionRequired) {
		t.Fatalf("RevokeSession() missing current error=%v", err)
	}
}

func TestIssueSessionForUserIssuesSessionForActiveUser(t *testing.T) {
	repository := &repositoryStub{user: User{
		ID: 42, Username: "oidc-operator", DisplayName: "OIDC Operator",
		Status: StatusActive, Roles: []Role{{Code: SystemAdmin}},
	}}
	service := NewService(repository, NewPasswordHasher(), NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)

	session, err := service.IssueSessionForUser(context.Background(), 42, "oidc-agent", "10.0.0.1")
	if err != nil {
		t.Fatalf("IssueSessionForUser() error = %v", err)
	}
	if session.User.ID != 42 || session.User.Username != "oidc-operator" {
		t.Fatalf("session user = %#v, want id=42 username=oidc-operator", session.User)
	}
	if session.AccessToken == "" || session.RefreshToken() == "" {
		t.Fatalf("session missing tokens: %#v", session)
	}
	if repository.refresh.UserID != 42 || repository.refresh.UserAgent != "oidc-agent" || repository.refresh.IPAddress != "10.0.0.1" {
		t.Fatalf("persisted refresh token = %#v, want user=42 agent=oidc-agent ip=10.0.0.1", repository.refresh)
	}
	claims, err := service.Authenticate(context.Background(), session.AccessToken)
	if err != nil || claims.Subject != "42" || len(claims.Roles) != 1 || claims.Roles[0] != SystemAdmin {
		t.Fatalf("Authenticate() claims=%#v err=%v", claims, err)
	}
}

func TestIssueSessionForUserFailsClosedForDisabledUser(t *testing.T) {
	repository := &repositoryStub{user: User{ID: 7, Username: "disabled", Status: StatusDisabled}}
	service := NewService(repository, NewPasswordHasher(), NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)

	if _, err := service.IssueSessionForUser(context.Background(), 7, "agent", "ip"); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("IssueSessionForUser() disabled error = %v, want ErrUserDisabled", err)
	}
}

func TestIssueSessionForUserFailsClosedForMissingUser(t *testing.T) {
	repository := &repositoryStub{user: User{}, findErr: ErrUserNotFound}
	service := NewService(repository, NewPasswordHasher(), NewTokenManager("test-signing-key-that-is-long-enough", time.Minute), time.Hour)

	if _, err := service.IssueSessionForUser(context.Background(), 99, "agent", "ip"); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("IssueSessionForUser() missing user error = %v, want ErrUserDisabled", err)
	}
}
