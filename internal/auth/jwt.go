package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/yourusername/orderflow/internal/domain"
)

// ErrInvalidToken is returned when a token is missing, malformed, expired, or
// signed with the wrong key or algorithm.
var ErrInvalidToken = errors.New("invalid or expired token")

// Claims is the JWT payload. The user ID is carried in the standard "sub"
// claim; the role is a custom claim used for authorization.
type Claims struct {
	Role domain.Role `json:"role"`
	jwt.RegisteredClaims
}

// TokenManager issues and validates signed JWTs. It is safe for concurrent use.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

// NewTokenManager returns a TokenManager that signs tokens with the given secret
// and expiry. The secret must be non-empty.
func NewTokenManager(secret string, ttl time.Duration) (*TokenManager, error) {
	if secret == "" {
		return nil, errors.New("jwt secret must not be empty")
	}
	return &TokenManager{
		secret: []byte(secret),
		ttl:    ttl,
		issuer: "orderflow",
	}, nil
}

// Generate creates a signed token for the given user.
func (m *TokenManager) Generate(userID int64, role domain.Role) (string, error) {
	now := time.Now()
	claims := Claims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			Issuer:    m.issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

// Parse validates a token string and returns its claims. The signing algorithm
// is pinned to HS256: accepting the algorithm advertised in the token header
// would expose the classic "alg confusion" / "alg=none" vulnerabilities, so any
// other algorithm is rejected outright.
func (m *TokenManager) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return m.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	return claims, nil
}
