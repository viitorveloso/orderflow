package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/yourusername/orderflow/internal/domain"
)

func TestTokenRoundTrip(t *testing.T) {
	tm, err := NewTokenManager("test-secret-please-change", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	token, err := tm.Generate(42, domain.RoleAdmin)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	claims, err := tm.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.Subject != "42" {
		t.Errorf("subject = %q, want 42", claims.Subject)
	}
	if claims.Role != domain.RoleAdmin {
		t.Errorf("role = %q, want admin", claims.Role)
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	tm, err := NewTokenManager("test-secret", -time.Minute) // already expired
	if err != nil {
		t.Fatal(err)
	}
	token, err := tm.Generate(1, domain.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tm.Parse(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for expired token, got %v", err)
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	issuer, _ := NewTokenManager("secret-a", time.Hour)
	verifier, _ := NewTokenManager("secret-b", time.Hour)

	token, err := issuer.Generate(1, domain.RoleUser)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Parse(token); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for wrong secret, got %v", err)
	}
}

func TestParseRejectsTamperedToken(t *testing.T) {
	tm, _ := NewTokenManager("secret", time.Hour)
	token, err := tm.Generate(1, domain.RoleUser)
	if err != nil {
		t.Fatal(err)
	}

	// Flip a character in the payload segment.
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}
	if parts[1][0] == 'A' {
		parts[1] = "B" + parts[1][1:]
	} else {
		parts[1] = "A" + parts[1][1:]
	}
	tampered := strings.Join(parts, ".")

	if _, err := tm.Parse(tampered); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for tampered token, got %v", err)
	}
}

// TestParseRejectsNoneAlgorithm ensures an attacker cannot strip the signature
// by setting alg=none.
func TestParseRejectsNoneAlgorithm(t *testing.T) {
	tm, _ := NewTokenManager("secret", time.Hour)

	claims := Claims{
		Role: domain.RoleAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "1",
			Issuer:    "orderflow",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).
		SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("building none-signed token: %v", err)
	}

	if _, err := tm.Parse(unsigned); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken for alg=none token, got %v", err)
	}
}
