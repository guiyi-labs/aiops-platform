package auth

import "time"

const (
	StatusActive    = "active"
	StatusDisabled  = "disabled"
	SystemAdmin     = "system_admin"
	OperationsAdmin = "operations_admin"
	SecurityAuditor = "security_auditor"
	Viewer          = "viewer"
)

type Role struct {
	ID   int64  `gorm:"primaryKey"`
	Code string `gorm:"size:64;uniqueIndex;not null"`
	Name string `gorm:"size:128;not null"`
}

func (Role) TableName() string { return "roles" }

type User struct {
	ID           int64  `gorm:"primaryKey"`
	Username     string `gorm:"size:64;uniqueIndex;not null"`
	PasswordHash string `gorm:"size:255;not null"`
	AuthVersion  int64  `gorm:"not null;default:1"`
	DisplayName  string `gorm:"size:128;not null"`
	Status       string `gorm:"size:32;not null"`
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Roles        []Role `gorm:"many2many:user_roles"`
}

func (User) TableName() string { return "users" }

type RefreshToken struct {
	ID        int64  `gorm:"primaryKey"`
	UserID    int64  `gorm:"not null;index"`
	TokenHash string `gorm:"size:64;uniqueIndex;not null"`
	UserAgent string `gorm:"size:512;not null"`
	IPAddress string `gorm:"size:64;not null"`
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	User      User
}

func (RefreshToken) TableName() string { return "refresh_tokens" }

func (u User) RoleCodes() []string {
	roles := make([]string, 0, len(u.Roles))
	for _, role := range u.Roles {
		roles = append(roles, role.Code)
	}
	return roles
}
