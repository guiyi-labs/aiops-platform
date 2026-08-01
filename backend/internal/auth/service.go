package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidCredentials     = errors.New("invalid username or password")
	ErrUserDisabled           = errors.New("user is disabled")
	ErrInvalidAccessToken     = errors.New("invalid access token")
	ErrUserNotAssignable      = errors.New("user cannot be assigned diagnoses")
	ErrInvalidUser            = errors.New("invalid user management input")
	ErrSelfProtection         = errors.New("cannot disable or change roles of the current user")
	ErrSelfPasswordReset      = errors.New("cannot administratively reset the current user's password")
	ErrPasswordUnchanged      = errors.New("new password must differ from current password")
	ErrCurrentSessionRequired = errors.New("current refresh session is required")
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

type UserView struct {
	ID          int64    `json:"id"`
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Roles       []string `json:"roles"`
}

type ManagedUserView struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Roles       []string   `json:"roles"`
	Status      string     `json:"status"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type RefreshSessionView struct {
	ID        int64     `json:"id"`
	UserAgent string    `json:"user_agent"`
	IPAddress string    `json:"ip_address"`
	Current   bool      `json:"current"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type CreateUserInput struct {
	Username, Password, DisplayName string
	Roles                           []string
}

type UpdateUserInput struct {
	DisplayName *string
	Status      *string
	Roles       *[]string
}

type Session struct {
	AccessToken          string   `json:"access_token"`
	TokenType            string   `json:"token_type"`
	AccessTokenExpiresIn int64    `json:"expires_in"`
	User                 UserView `json:"user"`
	refreshToken         string
}

func (s Session) RefreshToken() string { return s.refreshToken }

type Service struct {
	repository Repository
	passwords  PasswordHasher
	tokens     TokenManager
	refreshTTL time.Duration
	now        func() time.Time
}

func NewService(repository Repository, passwords PasswordHasher, tokens TokenManager, refreshTTL time.Duration) *Service {
	return &Service{repository: repository, passwords: passwords, tokens: tokens, refreshTTL: refreshTTL, now: time.Now}
}

func (s *Service) BootstrapAdmin(ctx context.Context, username, password string) (bool, error) {
	count, err := s.repository.CountUsers(ctx)
	if err != nil || count > 0 {
		return false, err
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return false, fmt.Errorf("hash bootstrap password: %w", err)
	}
	user := User{Username: username, PasswordHash: hash, DisplayName: "System Administrator", Status: StatusActive}
	if err := s.repository.CreateBootstrapAdmin(ctx, user); err != nil {
		return false, fmt.Errorf("create bootstrap administrator: %w", err)
	}
	return true, nil
}

func (s *Service) Login(ctx context.Context, username, password, userAgent, ipAddress string) (Session, error) {
	user, err := s.repository.FindUserByUsername(ctx, username)
	if err != nil || !s.passwords.Compare(user.PasswordHash, password) {
		return Session{}, ErrInvalidCredentials
	}
	if user.Status != StatusActive {
		return Session{}, ErrUserDisabled
	}
	now := s.now().UTC()
	if err := s.repository.UpdateLastLogin(ctx, user.ID, now); err != nil {
		return Session{}, fmt.Errorf("update last login: %w", err)
	}
	return s.newSession(ctx, user, userAgent, ipAddress, now)
}

// IssueSessionForUser issues a local session for an already-authenticated user
// identified by userID. It is the entry point for OIDC login: the OIDC session
// manager resolves the provider subject to a prelinked local user, checks the
// user's status, and delegates here so the resulting session flows through the
// same refresh-token rotation, auth_version revocation and audit semantics as
// password login. The user must be active; a disabled user fails closed with
// ErrUserDisabled.
func (s *Service) IssueSessionForUser(ctx context.Context, userID int64, userAgent, ipAddress string) (Session, error) {
	user, err := s.repository.FindUserByID(ctx, userID)
	if err != nil {
		return Session{}, ErrUserDisabled
	}
	if user.Status != StatusActive {
		return Session{}, ErrUserDisabled
	}
	now := s.now().UTC()
	return s.newSession(ctx, user, userAgent, ipAddress, now)
}

func (s *Service) Refresh(ctx context.Context, rawRefreshToken, userAgent, ipAddress string) (Session, error) {
	if rawRefreshToken == "" {
		return Session{}, ErrRefreshTokenInvalid
	}
	now := s.now().UTC()
	raw, hash, err := NewRefreshToken()
	if err != nil {
		return Session{}, fmt.Errorf("generate refresh token: %w", err)
	}
	next := RefreshToken{TokenHash: hash, UserAgent: userAgent, IPAddress: ipAddress, ExpiresAt: now.Add(s.refreshTTL)}
	user, err := s.repository.RotateRefreshToken(ctx, HashRefreshToken(rawRefreshToken), next, now)
	if err != nil {
		if errors.Is(err, ErrUserDisabled) {
			return Session{}, ErrUserDisabled
		}
		return Session{}, ErrRefreshTokenInvalid
	}
	accessToken, expiresAt, err := s.tokens.IssueAccessToken(user)
	if err != nil {
		return Session{}, fmt.Errorf("issue access token: %w", err)
	}
	return sessionFrom(user, accessToken, expiresAt, now, raw), nil
}

func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	if rawRefreshToken == "" {
		return nil
	}
	return s.repository.RevokeRefreshToken(ctx, HashRefreshToken(rawRefreshToken), s.now().UTC())
}

func (s *Service) Authenticate(ctx context.Context, rawAccessToken string) (Claims, error) {
	claims, err := s.tokens.ParseAccessToken(rawAccessToken)
	if err != nil {
		return Claims{}, ErrInvalidAccessToken
	}
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return Claims{}, ErrInvalidAccessToken
	}
	user, err := s.repository.FindUserByID(ctx, id)
	if err != nil {
		return Claims{}, ErrInvalidAccessToken
	}
	if user.Status != StatusActive {
		return Claims{}, ErrUserDisabled
	}
	if claims.AuthVersion != user.AuthVersion {
		return Claims{}, ErrInvalidAccessToken
	}
	claims.Username = user.Username
	claims.DisplayName = user.DisplayName
	claims.Roles = user.RoleCodes()
	return claims, nil
}

func (s *Service) AssignableUsers(ctx context.Context) ([]UserView, error) {
	users, err := s.repository.ListActiveUsers(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]UserView, 0, len(users))
	for _, user := range users {
		if user.Status == StatusActive && isAssignable(user) {
			items = append(items, UserView{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Roles: user.RoleCodes()})
		}
	}
	return items, nil
}

func (s *Service) AssignableUser(ctx context.Context, id int64) (UserView, error) {
	user, err := s.repository.FindUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return UserView{}, ErrUserNotAssignable
		}
		return UserView{}, err
	}
	if user.Status != StatusActive || !isAssignable(user) {
		return UserView{}, ErrUserNotAssignable
	}
	return UserView{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Roles: user.RoleCodes()}, nil
}

func (s *Service) ListUsers(ctx context.Context, limit int) ([]ManagedUserView, int64, error) {
	users, total, err := s.repository.ListUsers(ctx, limit)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ManagedUserView, 0, len(users))
	for _, user := range users {
		items = append(items, managedUserView(user))
	}
	return items, total, nil
}

func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) (ManagedUserView, error) {
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	roles, ok := normalizeRoles(input.Roles)
	if !usernamePattern.MatchString(input.Username) || len([]rune(input.DisplayName)) < 1 || len([]rune(input.DisplayName)) > 128 || len(input.Password) < 12 || len(input.Password) > 128 || !ok {
		return ManagedUserView{}, ErrInvalidUser
	}
	hash, err := s.passwords.Hash(input.Password)
	if err != nil {
		return ManagedUserView{}, fmt.Errorf("hash user password: %w", err)
	}
	user, err := s.repository.CreateUser(ctx, User{Username: input.Username, PasswordHash: hash, DisplayName: input.DisplayName, Status: StatusActive}, roles)
	if err != nil {
		return ManagedUserView{}, err
	}
	return managedUserView(user), nil
}

func (s *Service) UpdateUser(ctx context.Context, id, actorID int64, input UpdateUserInput) (ManagedUserView, error) {
	if input.DisplayName == nil && input.Status == nil && input.Roles == nil {
		return ManagedUserView{}, ErrInvalidUser
	}
	if input.DisplayName != nil {
		value := strings.TrimSpace(*input.DisplayName)
		if len([]rune(value)) < 1 || len([]rune(value)) > 128 {
			return ManagedUserView{}, ErrInvalidUser
		}
		input.DisplayName = &value
	}
	if input.Status != nil && *input.Status != StatusActive && *input.Status != StatusDisabled {
		return ManagedUserView{}, ErrInvalidUser
	}
	if input.Roles != nil {
		roles, ok := normalizeRoles(*input.Roles)
		if !ok {
			return ManagedUserView{}, ErrInvalidUser
		}
		input.Roles = &roles
	}
	if id == actorID && ((input.Status != nil && *input.Status == StatusDisabled) || input.Roles != nil) {
		return ManagedUserView{}, ErrSelfProtection
	}
	user, err := s.repository.UpdateUser(ctx, id, UserUpdate{DisplayName: input.DisplayName, Status: input.Status, Roles: input.Roles})
	if err != nil {
		return ManagedUserView{}, err
	}
	return managedUserView(user), nil
}

func (s *Service) ResetPassword(ctx context.Context, id, actorID int64, password string) (ManagedUserView, error) {
	if id == actorID {
		return ManagedUserView{}, ErrSelfPasswordReset
	}
	if len(password) < 12 || len(password) > 128 {
		return ManagedUserView{}, ErrInvalidUser
	}
	hash, err := s.passwords.Hash(password)
	if err != nil {
		return ManagedUserView{}, fmt.Errorf("hash reset password: %w", err)
	}
	user, err := s.repository.ResetPassword(ctx, id, hash, s.now().UTC())
	if err != nil {
		return ManagedUserView{}, err
	}
	return managedUserView(user), nil
}

func (s *Service) ChangePassword(ctx context.Context, id int64, currentPassword, newPassword string) error {
	if len(newPassword) < 12 || len(newPassword) > 128 {
		return ErrInvalidUser
	}
	user, err := s.repository.FindUserByID(ctx, id)
	if err != nil {
		return err
	}
	if !s.passwords.Compare(user.PasswordHash, currentPassword) {
		return ErrCurrentPasswordInvalid
	}
	if s.passwords.Compare(user.PasswordHash, newPassword) {
		return ErrPasswordUnchanged
	}
	hash, err := s.passwords.Hash(newPassword)
	if err != nil {
		return fmt.Errorf("hash changed password: %w", err)
	}
	return s.repository.ChangePassword(ctx, id, user.PasswordHash, hash, s.now().UTC())
}

func (s *Service) ListSessions(ctx context.Context, userID int64, currentRaw string) ([]RefreshSessionView, error) {
	currentHash := ""
	if currentRaw != "" {
		currentHash = HashRefreshToken(currentRaw)
	}
	tokens, err := s.repository.ListRefreshTokens(ctx, userID, s.now().UTC())
	if err != nil {
		return nil, err
	}
	items := make([]RefreshSessionView, 0, len(tokens))
	for _, token := range tokens {
		items = append(items, RefreshSessionView{ID: token.ID, UserAgent: token.UserAgent, IPAddress: token.IPAddress, Current: currentHash != "" && token.TokenHash == currentHash, CreatedAt: token.CreatedAt, ExpiresAt: token.ExpiresAt})
	}
	return items, nil
}

func (s *Service) RevokeSession(ctx context.Context, userID, sessionID int64, currentRaw string) error {
	if currentRaw == "" {
		return ErrCurrentSessionRequired
	}
	return s.repository.RevokeRefreshTokenForUser(ctx, userID, sessionID, HashRefreshToken(currentRaw), s.now().UTC())
}

func (s *Service) RevokeOtherSessions(ctx context.Context, userID int64, currentRaw string) (int64, error) {
	if currentRaw == "" {
		return 0, ErrCurrentSessionRequired
	}
	return s.repository.RevokeOtherRefreshTokens(ctx, userID, HashRefreshToken(currentRaw), s.now().UTC())
}

func normalizeRoles(input []string) ([]string, bool) {
	if len(input) == 0 {
		return nil, false
	}
	allowed := map[string]struct{}{SystemAdmin: {}, OperationsAdmin: {}, SecurityAuditor: {}, Viewer: {}}
	roles := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, role := range input {
		if _, ok := allowed[role]; !ok {
			return nil, false
		}
		if _, duplicate := seen[role]; !duplicate {
			seen[role] = struct{}{}
			roles = append(roles, role)
		}
	}
	slices.Sort(roles)
	return roles, len(roles) > 0
}

func managedUserView(user User) ManagedUserView {
	roles := user.RoleCodes()
	slices.Sort(roles)
	return ManagedUserView{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Roles: roles, Status: user.Status, LastLoginAt: user.LastLoginAt, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}
}

func isAssignable(user User) bool {
	for _, role := range user.Roles {
		if role.Code == SystemAdmin || role.Code == OperationsAdmin {
			return true
		}
	}
	return false
}

func (s *Service) newSession(ctx context.Context, user User, userAgent, ipAddress string, now time.Time) (Session, error) {
	accessToken, expiresAt, err := s.tokens.IssueAccessToken(user)
	if err != nil {
		return Session{}, fmt.Errorf("issue access token: %w", err)
	}
	raw, hash, err := NewRefreshToken()
	if err != nil {
		return Session{}, fmt.Errorf("generate refresh token: %w", err)
	}
	refresh := RefreshToken{UserID: user.ID, TokenHash: hash, UserAgent: userAgent, IPAddress: ipAddress, ExpiresAt: now.Add(s.refreshTTL)}
	if err := s.repository.CreateRefreshToken(ctx, refresh); err != nil {
		return Session{}, fmt.Errorf("persist refresh token: %w", err)
	}
	return sessionFrom(user, accessToken, expiresAt, now, raw), nil
}

func sessionFrom(user User, accessToken string, expiresAt, now time.Time, refreshToken string) Session {
	return Session{
		AccessToken:          accessToken,
		TokenType:            "Bearer",
		AccessTokenExpiresIn: int64(expiresAt.Sub(now).Seconds()),
		User:                 UserView{ID: user.ID, Username: user.Username, DisplayName: user.DisplayName, Roles: user.RoleCodes()},
		refreshToken:         refreshToken,
	}
}
