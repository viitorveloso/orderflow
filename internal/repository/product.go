package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/yourusername/orderflow/internal/domain"
)

// ProductRepo is the Postgres-backed ProductRepository.
type ProductRepo struct {
	db *sql.DB
}

// NewProductRepo wires a ProductRepo.
func NewProductRepo(db *sql.DB) *ProductRepo {
	return &ProductRepo{db: db}
}

// Create inserts a product and fills in its ID and timestamps.
func (r *ProductRepo) Create(ctx context.Context, p *domain.Product) error {
	const q = `
		INSERT INTO products (name, description, price_cents, stock)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`
	err := r.db.QueryRowContext(ctx, q, p.Name, p.Description, p.PriceCents, p.Stock).
		Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert product: %w", err)
	}
	return nil
}

// GetByID returns a product or domain.ErrNotFound.
func (r *ProductRepo) GetByID(ctx context.Context, id int64) (*domain.Product, error) {
	const q = `
		SELECT id, name, description, price_cents, stock, created_at, updated_at
		FROM products
		WHERE id = $1`
	p, err := scanProduct(r.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	return p, nil
}

// ListByIDs returns the products whose IDs are in the set. It issues a single
// query with `id = ANY($1)` rather than a query per ID, avoiding an N+1.
func (r *ProductRepo) ListByIDs(ctx context.Context, ids []int64) ([]domain.Product, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	const q = `
		SELECT id, name, description, price_cents, stock, created_at, updated_at
		FROM products
		WHERE id = ANY($1)`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("list products by ids: %w", err)
	}
	defer rows.Close()

	return scanProducts(rows)
}

// List returns a page of products ordered by ID.
func (r *ProductRepo) List(ctx context.Context, limit, offset int) ([]domain.Product, error) {
	const q = `
		SELECT id, name, description, price_cents, stock, created_at, updated_at
		FROM products
		ORDER BY id
		LIMIT $1 OFFSET $2`
	rows, err := r.db.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	return scanProducts(rows)
}

// Update replaces a product's writable fields, returning domain.ErrNotFound if
// it no longer exists.
func (r *ProductRepo) Update(ctx context.Context, p *domain.Product) error {
	const q = `
		UPDATE products
		SET name = $1, description = $2, price_cents = $3, stock = $4, updated_at = now()
		WHERE id = $5
		RETURNING created_at, updated_at`
	err := r.db.QueryRowContext(ctx, q, p.Name, p.Description, p.PriceCents, p.Stock, p.ID).
		Scan(&p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("update product: %w", err)
	}
	return nil
}

// Delete removes a product, returning domain.ErrNotFound if absent.
func (r *ProductRepo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM products WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete product: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete product rows affected: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// rowScanner is satisfied by both *sql.Row and *sql.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanProduct(s rowScanner) (*domain.Product, error) {
	var p domain.Product
	if err := s.Scan(
		&p.ID, &p.Name, &p.Description, &p.PriceCents, &p.Stock, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &p, nil
}

func scanProducts(rows *sql.Rows) ([]domain.Product, error) {
	var out []domain.Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}
