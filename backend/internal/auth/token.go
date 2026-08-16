package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenIssuer = "k8s-aiops-api" // #nosec G101 -- issuer claim, not a credential

type Claims struct {
	Username    string   `json:"username"`
	DisplayName string   `json:"display_name"`
	Roles       []string `json:"roles"`
	AuthVersion int64    `json:"auth_version"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	signingKey []byte
	accessTTL  time.Duration
	now        func() time.Time
}

func NewTokenManager(signingKey string, accessTTL time.Duration) TokenManager {
	return TokenManager{signingKey: []byte(signingKey), accessTTL: accessTTL, now: time.Now}
}

func (m TokenManager) IssueAccessToken(user User) (string, time.Time, error) {
	now := m.now().UTC()
	expiresAt := now.Add(m.accessTTL)
	claims := Claims{
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Roles:       user.RoleCodes(),
		AuthVersion: user.AuthVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   fmt.Sprintf("%d", user.ID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.signingKey)
	return signed, expiresAt, err
}

func (m TokenManager) ParseAccessToken(raw string) (Claims, error) {
	claims := Claims{}
	token, err := jwt.ParseWithClaims(raw, &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return m.signingKey, nil
	}, jwt.WithIssuer(tokenIssuer), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return Claims{}, fmt.Errorf("invalid access token")
	}
	return claims, nil
}

func NewRefreshToken() (raw, hash string, err error) {
	buffer := make([]byte, 32)
	if _, err = rand.Read(buffer); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(buffer)
	return raw, HashRefreshToken(raw), nil
}

func HashRefreshToken(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(digest[:])
}
