package service

import (
	"context"
	"sort"
	"strconv"

	"github.com/yourusername/orderflow/internal/domain"
)

// OrderService handles order placement and lifecycle.
type OrderService struct {
	orders   OrderRepository
	products ProductRepository
}

// NewOrderService wires an OrderService.
func NewOrderService(orders OrderRepository, products ProductRepository) *OrderService {
	return &OrderService{orders: orders, products: products}
}

// OrderLineInput is one requested line of a new order.
type OrderLineInput struct {
	ProductID int64
	Quantity  int
}

// CreateOrderInput is the payload for placing an order.
type CreateOrderInput struct {
	Items []OrderLineInput
}

// Create places an order for the given user.
//
// The flow is:
//  1. Validate and merge the requested lines (duplicate product IDs are summed).
//  2. Load the referenced products to snapshot their current prices and run a
//     friendly, early stock check.
//  3. Build the order with line totals and a grand total.
//  4. Hand the order to the repository, which authoritatively reserves stock and
//     persists everything in a single transaction.
//
// Note on concurrency: the stock check in step 2 is optimistic and exists only
// to return a clear error before touching the database. It is NOT the guard
// against overselling — two requests could both pass it for the last unit. The
// authoritative, race-safe reservation happens in the repository via a
// conditional UPDATE (see repository.OrderRepo.Create), which is why a second
// check there is intentional rather than redundant.
func (s *OrderService) Create(ctx context.Context, userID int64, in CreateOrderInput) (*domain.Order, error) {
	lines, err := mergeAndValidateLines(in.Items)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(lines))
	for id := range lines {
		ids = append(ids, id)
	}

	products, err := s.products.ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]domain.Product, len(products))
	for _, p := range products {
		byID[p.ID] = p
	}

	fields := domain.FieldErrors{}
	items := make([]domain.OrderItem, 0, len(lines))
	var total int64

	// Iterate ids (not the map) so error messages and totals are deterministic.
	for _, id := range sortedKeys(lines) {
		qty := lines[id]
		product, ok := byID[id]
		if !ok {
			fields.Add(productField(id), "product not found")
			continue
		}
		if product.Stock < qty {
			return nil, domain.ErrInsufficientStock
		}
		items = append(items, domain.OrderItem{
			ProductID:      id,
			Quantity:       qty,
			UnitPriceCents: product.PriceCents,
		})
		total += product.PriceCents * int64(qty)
	}
	if fields.Has() {
		return nil, fields
	}

	order := &domain.Order{
		UserID:     userID,
		Status:     domain.StatusPending,
		TotalCents: total,
		Items:      items,
	}
	if err := s.orders.Create(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// Get returns an order if the actor is allowed to see it. A user may read their
// own orders; an admin may read any. Otherwise domain.ErrForbidden is returned.
// To avoid revealing whether an order exists, a missing order and a forbidden
// one are both reported as domain.ErrNotFound to non-owners.
func (s *OrderService) Get(ctx context.Context, actorID int64, role domain.Role, orderID int64) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if role != domain.RoleAdmin && order.UserID != actorID {
		return nil, domain.ErrNotFound
	}
	return order, nil
}

// ListByUser returns a page of the user's own orders.
func (s *OrderService) ListByUser(ctx context.Context, userID int64, limit, offset int) ([]domain.Order, error) {
	limit, offset = normalizePage(limit, offset)
	return s.orders.ListByUser(ctx, userID, limit, offset)
}

// UpdateStatus moves an order to a new status, enforcing the order state
// machine. It returns domain.ErrInvalidStatusChange if the transition is not
// allowed.
func (s *OrderService) UpdateStatus(ctx context.Context, orderID int64, next domain.OrderStatus) (*domain.Order, error) {
	if !next.Valid() {
		f := domain.FieldErrors{}
		f.Add("status", "must be one of pending, paid, shipped, cancelled")
		return nil, f
	}

	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.Status == next {
		return order, nil // no-op, already in the requested state
	}
	if !order.Status.CanTransitionTo(next) {
		return nil, domain.ErrInvalidStatusChange
	}

	if err := s.orders.UpdateStatus(ctx, orderID, next); err != nil {
		return nil, err
	}
	order.Status = next
	return order, nil
}

// mergeAndValidateLines collapses duplicate product IDs into summed quantities
// and rejects empty orders or non-positive quantities.
func mergeAndValidateLines(in []OrderLineInput) (map[int64]int, error) {
	fields := domain.FieldErrors{}
	if len(in) == 0 {
		fields.Add("items", "must contain at least one item")
		return nil, fields
	}

	merged := make(map[int64]int, len(in))
	for _, line := range in {
		if line.Quantity <= 0 {
			fields.Add(productField(line.ProductID), "quantity must be positive")
			continue
		}
		merged[line.ProductID] += line.Quantity
	}
	if fields.Has() {
		return nil, fields
	}
	return merged, nil
}

// sortedKeys returns the keys of m in ascending order, for deterministic
// iteration (stable error messages and totals).
func sortedKeys(m map[int64]int) []int64 {
	keys := make([]int64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

// productField builds the validation-field name for a given product line.
func productField(id int64) string {
	return "items.product_" + strconv.FormatInt(id, 10)
}
