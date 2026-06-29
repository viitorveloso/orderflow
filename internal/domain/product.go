package domain

import "time"

// Product is a catalog item that can be ordered. Money is stored as an integer
// number of cents (PriceCents) rather than a float to avoid rounding error in
// monetary arithmetic.
type Product struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	PriceCents  int64     `json:"price_cents"`
	Stock       int       `json:"stock"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
