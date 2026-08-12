package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrUserNotFound            = errors.New("user not found")
	ErrRefreshTokenInvalid     = errors.New("refresh token is invalid")
	ErrUsernameExists          = errors.New("username already exists")
	ErrLastSystemAdmin         = errors.New("cannot remove the last active system administrator")
	ErrCurrentPasswordInvalid  = errors.New("current password is invalid")
	ErrSessionNotFound         = errors.New("refresh session not found")
	ErrCurrentSessionProtected = errors.New("current refresh session cannot be revoked by id")
)

type UserUpdate struct {
	DisplayName *string
	Status      *string
	Roles       *[]string
}

type Repository interface {
	CountUsers(context.Context) (int64, error)
	CreateBootstrapAdmin(context.Context, User) error
	FindUserByUsername(context.Context, string) (User, error)
	FindUserByID(context.Context, int64) (User, error)
	ListActiveUsers(context.Context) ([]User, error)
	ListUsers(context.Context, int) ([]User, int64, error)
	CreateUser(context.Context, User, []string) (User, error)
	UpdateUser(context.Context, int64, UserUpdate) (User, error)
	ResetPassword(context.Context, int64, string, time.Time) (User, error)
	ChangePassword(context.Context, int64, string, string, time.Time) error
	UpdateLastLogin(context.Context, int64, time.Time) error
	CreateRefreshToken(context.Context, RefreshToken) error
	RotateRefreshToken(context.Context, string, RefreshToken, time.Time) (User, error)
	RevokeRefreshToken(context.Context, string, time.Time) error
	ListRefreshTokens(context.Context, int64, time.Time) ([]RefreshToken, error)
	RevokeRefreshTokenForUser(context.Context, int64, int64, string, time.Time) error
	RevokeOtherRefreshTokens(context.Context, int64, string, time.Time) (int64, error)
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (r *GormRepository) CountUsers(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&User{}).Count(&count).Error
	return count, err
}

func (r *GormRepository) CreateBootstrapAdmin(ctx context.Context, user User) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var role Role
		if err := tx.Where("code = ?", SystemAdmin).First(&role).Error; err != nil {
			return fmt.Errorf("find system administrator role: %w", err)
		}
		user.Roles = []Role{role}
		return tx.Create(&user).Error
	})
}

func (r *GormRepository) FindUserByUsername(ctx context.Context, username string) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Preload("Roles").Where("username = ?", username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func (r *GormRepository) FindUserByID(ctx context.Context, id int64) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Preload("Roles").First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func (r *GormRepository) ListActiveUsers(ctx context.Context) ([]User, error) {
	var users []User
	err := r.db.WithContext(ctx).Preload("Roles").Where("status = ?", StatusActive).Order("display_name ASC").Find(&users).Error
	return users, err
}

func (r *GormRepository) ListUsers(ctx context.Context, limit int) ([]User, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []User
	if err := r.db.WithContext(ctx).Preload("Roles").Order("created_at ASC, id ASC").Limit(limit).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

func (r *GormRepository) CreateUser(ctx context.Context, user User, roleCodes []string) (User, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Raw(`INSERT INTO users (username, password_hash, display_name, status)
			VALUES (?, ?, ?, ?) ON CONFLICT (username) DO NOTHING RETURNING id, created_at, updated_at`,
			user.Username, user.PasswordHash, user.DisplayName, user.Status).Row()
		if err := result.Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt); errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
			return ErrUsernameExists
		} else if err != nil {
			return err
		}
		var roles []Role
		if err := tx.Where("code IN ?", roleCodes).Find(&roles).Error; err != nil {
			return err
		}
		if len(roles) != len(roleCodes) {
			return fmt.Errorf("role set changed during user creation")
		}
		for _, role := range roles {
			if err := tx.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)`, user.ID, role.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return r.FindUserByID(ctx, user.ID)
}

func (r *GormRepository) UpdateUser(ctx context.Context, id int64, update UserUpdate) (User, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(?)`, int64(741011)).Error; err != nil {
			return err
		}
		var user User
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Roles").First(&user, id).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		status := user.Status
		if update.Status != nil {
			status = *update.Status
		}
		roles := user.RoleCodes()
		if update.Roles != nil {
			roles = append([]string(nil), (*update.Roles)...)
		}
		if user.Status == StatusActive && hasRole(user.RoleCodes(), SystemAdmin) && (status != StatusActive || !hasRole(roles, SystemAdmin)) {
			var activeAdmins int64
			if err := tx.Raw(`SELECT COUNT(DISTINCT u.id) FROM users u
				JOIN user_roles ur ON ur.user_id = u.id JOIN roles r ON r.id = ur.role_id
				WHERE u.status = ? AND r.code = ?`, StatusActive, SystemAdmin).Row().Scan(&activeAdmins); err != nil {
				return err
			}
			if activeAdmins <= 1 {
				return ErrLastSystemAdmin
			}
		}
		values := map[string]any{"updated_at": time.Now().UTC()}
		if update.DisplayName != nil {
			values["display_name"] = *update.DisplayName
		}
		if update.Status != nil {
			values["status"] = *update.Status
		}
		if err := tx.Model(&User{}).Where("id = ?", id).Updates(values).Error; err != nil {
			return err
		}
		securityChanged := update.Status != nil && *update.Status == StatusDisabled
		if update.Roles != nil {
			securityChanged = true
			var storedRoles []Role
			if err := tx.Where("code IN ?", roles).Find(&storedRoles).Error; err != nil {
				return err
			}
			if len(storedRoles) != len(roles) {
				return fmt.Errorf("role set changed during user update")
			}
			if err := tx.Exec(`DELETE FROM user_roles WHERE user_id = ?`, id).Error; err != nil {
				return err
			}
			for _, role := range storedRoles {
				if err := tx.Exec(`INSERT INTO user_roles (user_id, role_id) VALUES (?, ?)`, id, role.ID).Error; err != nil {
					return err
				}
			}
		}
		if securityChanged {
			// M100-B: a security-relevant change (disable or role change)
			// bumps auth_version so every outstanding access token is
			// rejected immediately, and revokes all refresh sessions so the
			// user must re-authenticate. This matches the invalidation
			// contract of ChangePassword/ResetPassword.
			if err := tx.Model(&User{}).Where("id = ?", id).Update("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
				return err
			}
			if err := tx.Model(&RefreshToken{}).Where("user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", time.Now().UTC()).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return User{}, err
	}
	return r.FindUserByID(ctx, id)
}

func (r *GormRepository) ResetPassword(ctx context.Context, id int64, passwordHash string, now time.Time) (User, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&User{}).Where("id = ?", id).Updates(map[string]any{
			"password_hash": passwordHash,
			"auth_version":  gorm.Expr("auth_version + 1"),
			"updated_at":    now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrUserNotFound
		}
		return tx.Model(&RefreshToken{}).Where("user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", now).Error
	})
	if err != nil {
		return User{}, err
	}
	return r.FindUserByID(ctx, id)
}

func (r *GormRepository) ChangePassword(ctx context.Context, id int64, expectedHash, passwordHash string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&User{}).Where("id = ? AND password_hash = ?", id, expectedHash).Updates(map[string]any{
			"password_hash": passwordHash,
			"auth_version":  gorm.Expr("auth_version + 1"),
			"updated_at":    now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrCurrentPasswordInvalid
		}
		return tx.Model(&RefreshToken{}).Where("user_id = ? AND revoked_at IS NULL", id).Update("revoked_at", now).Error
	})
}

func hasRole(roles []string, expected string) bool {
	for _, role := range roles {
		if role == expected {
			return true
		}
	}
	return false
}

func (r *GormRepository) UpdateLastLogin(ctx context.Context, id int64, loggedInAt time.Time) error {
	return r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Update("last_login_at", loggedInAt).Error
}

func (r *GormRepository) CreateRefreshToken(ctx context.Context, token RefreshToken) error {
	return r.db.WithContext(ctx).Create(&token).Error
}

func (r *GormRepository) RotateRefreshToken(ctx context.Context, oldHash string, next RefreshToken, now time.Time) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current RefreshToken
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", oldHash).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRefreshTokenInvalid
		}
		if err != nil {
			return err
		}
		if current.RevokedAt != nil || !current.ExpiresAt.After(now) {
			return ErrRefreshTokenInvalid
		}
		if err := tx.Model(&current).Update("revoked_at", now).Error; err != nil {
			return err
		}
		next.UserID = current.UserID
		if err := tx.Create(&next).Error; err != nil {
			return err
		}
		if err := tx.Preload("Roles").First(&user, current.UserID).Error; err != nil {
			return err
		}
		if user.Status != StatusActive {
			return ErrUserDisabled
		}
		return nil
	})
	return user, err
}

func (r *GormRepository) RevokeRefreshToken(ctx context.Context, hash string, now time.Time) error {
	result := r.db.WithContext(ctx).Model(&RefreshToken{}).
		Where("token_hash = ? AND revoked_at IS NULL", hash).
		Update("revoked_at", now)
	return result.Error
}

func (r *GormRepository) ListRefreshTokens(ctx context.Context, userID int64, now time.Time) ([]RefreshToken, error) {
	var tokens []RefreshToken
	err := r.db.WithContext(ctx).Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, now).Order("created_at DESC, id DESC").Find(&tokens).Error
	return tokens, err
}

func (r *GormRepository) RevokeRefreshTokenForUser(ctx context.Context, userID, sessionID int64, currentHash string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current int64
		if err := tx.Model(&RefreshToken{}).Where("user_id = ? AND token_hash = ? AND revoked_at IS NULL AND expires_at > ?", userID, currentHash, now).Count(&current).Error; err != nil {
			return err
		}
		if current == 0 {
			return ErrCurrentSessionRequired
		}
		var token RefreshToken
		err := tx.Where("id = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?", sessionID, userID, now).First(&token).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrSessionNotFound
		}
		if err != nil {
			return err
		}
		if token.TokenHash == currentHash {
			return ErrCurrentSessionProtected
		}
		return tx.Model(&token).Update("revoked_at", now).Error
	})
}

func (r *GormRepository) RevokeOtherRefreshTokens(ctx context.Context, userID int64, currentHash string, now time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current int64
		if err := tx.Model(&RefreshToken{}).Where("user_id = ? AND token_hash = ? AND revoked_at IS NULL AND expires_at > ?", userID, currentHash, now).Count(&current).Error; err != nil {
			return err
		}
		if current == 0 {
			return ErrCurrentSessionRequired
		}
		result := tx.Model(&RefreshToken{}).
			Where("user_id = ? AND token_hash <> ? AND revoked_at IS NULL AND expires_at > ?", userID, currentHash, now).
			Update("revoked_at", now)
		count = result.RowsAffected
		return result.Error
	})
	return count, err
}
