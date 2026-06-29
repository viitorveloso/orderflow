package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/yourusername/orderflow/internal/domain"
)

// OrderRepo is the Postgres-backed OrderRepository.
type OrderRepo struct {
	db *sql.DB
}

// NewOrderRepo wires an OrderRepo.
func NewOrderRepo(db *sql.DB) *OrderRepo {
	return &OrderRepo{db: db}
}

// Create reserves stock for every item and persists the order and its items in
// a single transaction.
//
// Stock is reserved with a conditional update:
//
//	UPDATE products SET stock = stock - $qty WHERE id = $id AND stock >= $qty
//
// Because the decrement and the "enough stock?" check happen in one atomic
// statement, two concurrent orders for the last unit cannot both succeed: the
// second sees zero rows affected and the whole transaction rolls back with
// domain.ErrInsufficientStock. This is the authoritative guard against
// overselling (the service layer's earlier check is only a fast, friendly
// pre-validation).
func (r *OrderRepo) Create(ctx context.Context, o *domain.Order) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // safe no-op once committed

	const reserve = `
		UPDATE products
		SET stock = stock - $1, updated_at = now()
		WHERE id = $2 AND stock >= $1`
	for _, item := range o.Items {
		res, err := tx.ExecContext(ctx, reserve, item.Quantity, item.ProductID)
		if err != nil {
			return fmt.Errorf("reserve stock for product %d: %w", item.ProductID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if n == 0 {
			// Not enough stock, or the product disappeared between the service
			// pre-check and here. Either way this order cannot be fulfilled.
			return domain.ErrInsufficientStock
		}
	}

	const insertOrder = `
		INSERT INTO orders (user_id, status, total_cents)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`
	if err := tx.QueryRowContext(ctx, insertOrder, o.UserID, o.Status, o.TotalCents).
		Scan(&o.ID, &o.CreatedAt); err != nil {
		return fmt.Errorf("insert order: %w", err)
	}

	const insertItem = `
		INSERT INTO order_items (order_id, product_id, quantity, unit_price_cents)
		VALUES ($1, $2, $3, $4)
		RETURNING id`
	for i := range o.Items {
		if err := tx.QueryRowContext(ctx, insertItem,
			o.ID, o.Items[i].ProductID, o.Items[i].Quantity, o.Items[i].UnitPriceCents,
		).Scan(&o.Items[i].ID); err != nil {
			return fmt.Errorf("insert order item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit order: %w", err)
	}
	return nil
}

// GetByID returns an order with its items, or domain.ErrNotFound.
func (r *OrderRepo) GetByID(ctx context.Context, id int64) (*domain.Order, error) {
	const q = `
		SELECT id, user_id, status, total_cents, created_at
		FROM orders
		WHERE id = $1`
	var o domain.Order
	err := r.db.QueryRowContext(ctx, q, id).
		Scan(&o.ID, &o.UserID, &o.Status, &o.TotalCents, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get order: %w", err)
	}

	itemsByOrder, err := r.loadItems(ctx, []int64{o.ID})
	if err != nil {
		return nil, err
	}
	o.Items = itemsByOrder[o.ID]
	return &o, nil
}

// ListByUser returns a page of a user's orders (with items), newest first.
func (r *OrderRepo) ListByUser(ctx context.Context, userID int64, limit, offset int) ([]domain.Order, error) {
	const q = `
		SELECT id, user_id, status, total_cents, created_at
		FROM orders
		WHERE user_id = $1
		ORDER BY id DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.db.QueryContext(ctx, q, userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	var orders []domain.Order
	var ids []int64
	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.Status, &o.TotalCents, &o.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		orders = append(orders, o)
		ids = append(ids, o.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(orders) == 0 {
		return orders, nil
	}

	// One query for all items, then attach them to their orders (no N+1).
	itemsByOrder, err := r.loadItems(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range orders {
		orders[i].Items = itemsByOrder[orders[i].ID]
	}
	return orders, nil
}

// UpdateStatus sets the status of an order, returning domain.ErrNotFound if it
// does not exist.
func (r *OrderRepo) UpdateStatus(ctx context.Context, id int64, status domain.OrderStatus) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE orders SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// loadItems fetches the items for the given order IDs in a single query and
// groups them by order ID.
func (r *OrderRepo) loadItems(ctx context.Context, orderIDs []int64) (map[int64][]domain.OrderItem, error) {
	const q = `
		SELECT id, order_id, product_id, quantity, unit_price_cents
		FROM order_items
		WHERE order_id = ANY($1)
		ORDER BY id`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(orderIDs))
	if err != nil {
		return nil, fmt.Errorf("load order items: %w", err)
	}
	defer rows.Close()

	byOrder := make(map[int64][]domain.OrderItem)
	for rows.Next() {
		var (
			item    domain.OrderItem
			orderID int64
		)
		if err := rows.Scan(
			&item.ID, &orderID, &item.ProductID, &item.Quantity, &item.UnitPriceCents,
		); err != nil {
			return nil, fmt.Errorf("scan order item: %w", err)
		}
		byOrder[orderID] = append(byOrder[orderID], item)
	}
	return byOrder, rows.Err()
}
