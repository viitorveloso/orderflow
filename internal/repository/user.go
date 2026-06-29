package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/yourusername/orderflow/internal/domain"
)

// UserRepo is the Postgres-backed UserRepository.
type UserRepo struct {
	db *sql.DB
}

// NewUserRepo wires a UserRepo.
func NewUserRepo(db *sql.DB) *UserRepo {
	return &UserRepo{db: db}
}

// Create inserts a user and fills in the generated ID and timestamp. A duplicate
// email surfaces as domain.ErrConflict.
func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	const q = `
		INSERT INTO users (email, password_hash, role)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	err := r.db.QueryRowContext(ctx, q, u.Email, u.PasswordHash, u.Role).
		Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return domain.ErrConflict
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetByEmail looks up a user by email, returning domain.ErrNotFound if absent.
func (r *UserRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	const q = `
		SELECT id, email, password_hash, role, created_at
		FROM users
		WHERE email = $1`
	var u domain.User
	err := r.db.QueryRowContext(ctx, q, email).
		Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}
