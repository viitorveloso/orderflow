package service

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/yourusername/orderflow/internal/auth"
	"github.com/yourusername/orderflow/internal/domain"
)

// minPasswordLen is the shortest password we accept at registration.
const minPasswordLen = 8

// AuthService handles registration and login.
type AuthService struct {
	users  UserRepository
	tokens *auth.TokenManager
}

// NewAuthService wires an AuthService.
func NewAuthService(users UserRepository, tokens *auth.TokenManager) *AuthService {
	return &AuthService{users: users, tokens: tokens}
}

// RegisterInput is the data required to create an account.
type RegisterInput struct {
	Email    string
	Password string
}

// Register validates the input, hashes the password, and creates a user with
// the default "user" role. The returned user never carries the password hash in
// its JSON form.
func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*domain.User, error) {
	email := normalizeEmail(in.Email)

	fields := domain.FieldErrors{}
	if !validEmail(email) {
		fields.Add("email", "must be a valid email address")
	}
	if len(in.Password) < minPasswordLen {
		fields.Add("password", fmt.Sprintf("must be at least %d characters", minPasswordLen))
	}
	if fields.Has() {
		return nil, fields
	}

	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &domain.User{
		Email:        email,
		PasswordHash: hash,
		Role:         domain.RoleUser,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err // includes domain.ErrConflict for duplicate email
	}
	return user, nil
}

// Login verifies credentials and returns a signed token plus the user. To avoid
// leaking which emails are registered, an unknown email and a wrong password
// both yield domain.ErrInvalidCredentials.
func (s *AuthService) Login(ctx context.Context, email, password string) (string, *domain.User, error) {
	user, err := s.users.GetByEmail(ctx, normalizeEmail(email))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return "", nil, domain.ErrInvalidCredentials
		}
		return "", nil, err
	}

	ok, err := auth.VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return "", nil, fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return "", nil, domain.ErrInvalidCredentials
	}

	token, err := s.tokens.Generate(user.ID, user.Role)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	return token, user, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validEmail(email string) bool {
	if email == "" || len(email) > 254 {
		return false
	}
	_, err := mail.ParseAddress(email)
	return err == nil
}
