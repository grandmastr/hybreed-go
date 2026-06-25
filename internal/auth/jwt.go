package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const tokenIssuer = "hybreed"

// TokenManager signs and verifies short-lived HS256 access tokens.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewTokenManager builds a TokenManager from a shared secret and access-token TTL.
func NewTokenManager(secret string, ttl time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), ttl: ttl, now: time.Now}
}

// Claims is the access-token payload (subject is the user ID).
type Claims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

// Generate signs an access token for the user and returns it with its expiry.
func (m *TokenManager) Generate(userID uuid.UUID, email string) (string, time.Time, error) {
	now := m.now()
	expiresAt := now.Add(m.ttl)
	claims := Claims{
		Email: email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, expiresAt, nil
}

// Parse verifies a token and returns the user ID and email it carries.
func (m *TokenManager) Parse(token string) (uuid.UUID, string, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithIssuer(tokenIssuer), jwt.WithValidMethods([]string{"HS256"}))
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("parse token: %w", err)
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("invalid subject: %w", err)
	}
	return id, claims.Email, nil
}
