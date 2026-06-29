package domain

import "time"

// OrderStatus is the lifecycle state of an order.
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusPaid      OrderStatus = "paid"
	StatusShipped   OrderStatus = "shipped"
	StatusCancelled OrderStatus = "cancelled"
)

// allowedTransitions encodes the order state machine. A status may only move to
// one of its listed successors; everything else is rejected with
// ErrInvalidStatusChange. Keeping this as data (rather than scattered if-checks)
// makes the rules easy to read and extend.
var allowedTransitions = map[OrderStatus][]OrderStatus{
	StatusPending:   {StatusPaid, StatusCancelled},
	StatusPaid:      {StatusShipped, StatusCancelled},
	StatusShipped:   {}, // terminal
	StatusCancelled: {}, // terminal
}

// Valid reports whether s is a recognized status.
func (s OrderStatus) Valid() bool {
	_, ok := allowedTransitions[s]
	return ok
}

// CanTransitionTo reports whether moving from s to next is permitted by the
// order state machine.
func (s OrderStatus) CanTransitionTo(next OrderStatus) bool {
	for _, allowed := range allowedTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// OrderItem is a single line of an order. UnitPriceCents is a snapshot of the
// product price at the time the order was placed, so later catalog price
// changes never alter historical orders.
type OrderItem struct {
	ID             int64 `json:"id"`
	ProductID      int64 `json:"product_id"`
	Quantity       int   `json:"quantity"`
	UnitPriceCents int64 `json:"unit_price_cents"`
}

// LineTotalCents returns the cost of this line.
func (i OrderItem) LineTotalCents() int64 {
	return i.UnitPriceCents * int64(i.Quantity)
}

// Order is a customer purchase composed of one or more items. TotalCents is the
// sum of the line totals, persisted so it never has to be recomputed and can be
// reconciled against the items.
type Order struct {
	ID         int64       `json:"id"`
	UserID     int64       `json:"user_id"`
	Status     OrderStatus `json:"status"`
	TotalCents int64       `json:"total_cents"`
	Items      []OrderItem `json:"items"`
	CreatedAt  time.Time   `json:"created_at"`
}
