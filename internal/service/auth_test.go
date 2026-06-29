package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yourusername/orderflow/internal/auth"
	"github.com/yourusername/orderflow/internal/domain"
)

func newTestAuthService(t *testing.T) (*AuthService, *fakeUserRepo) {
	t.Helper()
	tm, err := auth.NewTokenManager("unit-test-secret-1234", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakeUserRepo()
	return NewAuthService(repo, tm), repo
}

func TestRegisterSuccess(t *testing.T) {
	svc, _ := newTestAuthService(t)

	user, err := svc.Register(context.Background(), RegisterInput{
		Email:    "  Alice@Example.com ",
		Password: "supersecret",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Email != "alice@example.com" {
		t.Errorf("email = %q, want normalized alice@example.com", user.Email)
	}
	if user.Role != domain.RoleUser {
		t.Errorf("role = %q, want user", user.Role)
	}
	if user.PasswordHash == "" || user.PasswordHash == "supersecret" {
		t.Error("password was not hashed")
	}
}

func TestRegisterValidation(t *testing.T) {
	svc, _ := newTestAuthService(t)

	_, err := svc.Register(context.Background(), RegisterInput{
		Email:    "not-an-email",
		Password: "short",
	})
	var fields domain.FieldErrors
	if !errors.As(err, &fields) {
		t.Fatalf("expected FieldErrors, got %v", err)
	}
	if _, ok := fields["email"]; !ok {
		t.Error("expected email field error")
	}
	if _, ok := fields["password"]; !ok {
		t.Error("expected password field error")
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	svc, _ := newTestAuthService(t)
	in := RegisterInput{Email: "dup@example.com", Password: "supersecret"}

	if _, err := svc.Register(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Register(context.Background(), in)
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestLoginSuccess(t *testing.T) {
	svc, _ := newTestAuthService(t)
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:    "bob@example.com",
		Password: "supersecret",
	})
	if err != nil {
		t.Fatal(err)
	}

	token, user, err := svc.Login(context.Background(), "bob@example.com", "supersecret")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if token == "" {
		t.Error("expected a token")
	}
	if user.Email != "bob@example.com" {
		t.Errorf("unexpected user: %+v", user)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	svc, _ := newTestAuthService(t)
	_, err := svc.Register(context.Background(), RegisterInput{
		Email:    "carol@example.com",
		Password: "supersecret",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = svc.Login(context.Background(), "carol@example.com", "wrong")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestLoginUnknownEmailLooksLikeWrongPassword(t *testing.T) {
	svc, _ := newTestAuthService(t)

	_, _, err := svc.Login(context.Background(), "ghost@example.com", "whatever")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials (no user enumeration), got %v", err)
	}
}
