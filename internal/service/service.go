// Package service contains the application's business logic. It depends on the
// repository interfaces defined here (not on concrete database types), which
// keeps the rules testable with in-memory fakes and independent of Postgres.
package service

import (
	"context"

	"github.com/yourusername/orderflow/internal/domain"
)

// UserRepository persists user accounts.
type UserRepository interface {
	// Create inserts u and populates u.ID and u.CreatedAt. It returns
	// domain.ErrConflict if the email is already registered.
	Create(ctx context.Context, u *domain.User) error
	// GetByEmail returns the user with the given email, or domain.ErrNotFound.
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
}

// ProductRepository persists catalog products.
type ProductRepository interface {
	Create(ctx context.Context, p *domain.Product) error
	// GetByID returns the product, or domain.ErrNotFound.
	GetByID(ctx context.Context, id int64) (*domain.Product, error)
	// ListByIDs returns the products matching ids, in no guaranteed order.
	// Missing IDs are simply absent from the result.
	ListByIDs(ctx context.Context, ids []int64) ([]domain.Product, error)
	List(ctx context.Context, limit, offset int) ([]domain.Product, error)
	// Update saves p by its ID, returning domain.ErrNotFound if it is gone.
	Update(ctx context.Context, p *domain.Product) error
	// Delete removes the product, returning domain.ErrNotFound if absent.
	Delete(ctx context.Context, id int64) error
}

// OrderRepository persists orders and their items.
type OrderRepository interface {
	// Create atomically reserves stock for every item and inserts the order
	// with its items, all in one transaction. If any item cannot be fulfilled
	// it returns domain.ErrInsufficientStock and reserves nothing. On success
	// it populates IDs and CreatedAt.
	Create(ctx context.Context, o *domain.Order) error
	// GetByID returns the order (with items), or domain.ErrNotFound.
	GetByID(ctx context.Context, id int64) (*domain.Order, error)
	// ListByUser returns a user's orders (with items), newest first.
	ListByUser(ctx context.Context, userID int64, limit, offset int) ([]domain.Order, error)
	// UpdateStatus sets the order status, returning domain.ErrNotFound if absent.
	UpdateStatus(ctx context.Context, id int64, status domain.OrderStatus) error
}

// Pagination defaults and bounds shared by list endpoints.
const (
	defaultLimit = 20
	maxLimit     = 100
)

// normalizePage clamps client-supplied paging parameters into a safe range so a
// caller cannot request an unbounded result set or a negative offset.
func normalizePage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
