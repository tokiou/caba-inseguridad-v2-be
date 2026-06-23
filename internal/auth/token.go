package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const accessTokenType = "access"

// accessClaims are the JWT claims for an access token. typ pins the token to its
// purpose so a refresh-style token can never be replayed as an access token.
type accessClaims struct {
	jwt.RegisteredClaims
	Typ string `json:"typ"`
}

// TokenManager mints and verifies HS256 access tokens.
type TokenManager struct {
	secret    []byte
	accessTTL time.Duration
}

func NewTokenManager(secret string, accessTTL time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), accessTTL: accessTTL}
}

// MintAccessToken returns a signed access JWT for the user and its lifetime in
// seconds.
func (m *TokenManager) MintAccessToken(userID uuid.UUID, now time.Time) (string, int, error) {
	claims := accessClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.accessTTL)),
			ID:        uuid.NewString(),
		},
		Typ: accessTokenType,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", 0, fmt.Errorf("auth: sign access token: %w", err)
	}

	return signed, int(m.accessTTL.Seconds()), nil
}

// VerifyAccessToken validates the signature, expiry, and token type, returning
// the subject user id. Any failure maps to ErrInvalidToken.
func (m *TokenManager) VerifyAccessToken(token string) (uuid.UUID, error) {
	var claims accessClaims
	parsed, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("auth: unexpected signing method %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil || !parsed.Valid {
		return uuid.Nil, ErrInvalidToken
	}
	if claims.Typ != accessTokenType {
		return uuid.Nil, ErrInvalidToken
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, ErrInvalidToken
	}

	return userID, nil
}

// generateRefreshToken returns a 256-bit opaque token, base64url-encoded.
func generateRefreshToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate refresh token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken returns the hex SHA-256 of an opaque token. The token is
// high-entropy, so a fast hash is safe and lets us look the session up by hash.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
