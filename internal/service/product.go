package service

import (
	"context"
	"strings"

	"github.com/yourusername/orderflow/internal/domain"
)

// ProductService handles catalog operations.
type ProductService struct {
	products ProductRepository
}

// NewProductService wires a ProductService.
func NewProductService(products ProductRepository) *ProductService {
	return &ProductService{products: products}
}

// ProductInput carries the writable fields of a product, shared by create and
// update (full-replace) operations.
type ProductInput struct {
	Name        string
	Description string
	PriceCents  int64
	Stock       int
}

func (in ProductInput) validate() error {
	fields := domain.FieldErrors{}
	if strings.TrimSpace(in.Name) == "" {
		fields.Add("name", "must not be empty")
	}
	if in.PriceCents < 0 {
		fields.Add("price_cents", "must not be negative")
	}
	if in.Stock < 0 {
		fields.Add("stock", "must not be negative")
	}
	if fields.Has() {
		return fields
	}
	return nil
}

// List returns a page of products.
func (s *ProductService) List(ctx context.Context, limit, offset int) ([]domain.Product, error) {
	limit, offset = normalizePage(limit, offset)
	return s.products.List(ctx, limit, offset)
}

// Get returns a single product or domain.ErrNotFound.
func (s *ProductService) Get(ctx context.Context, id int64) (*domain.Product, error) {
	return s.products.GetByID(ctx, id)
}

// Create validates and inserts a new product.
func (s *ProductService) Create(ctx context.Context, in ProductInput) (*domain.Product, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	p := &domain.Product{
		Name:        strings.TrimSpace(in.Name),
		Description: strings.TrimSpace(in.Description),
		PriceCents:  in.PriceCents,
		Stock:       in.Stock,
	}
	if err := s.products.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Update validates and replaces the writable fields of an existing product. It
// returns domain.ErrNotFound if the product does not exist.
func (s *ProductService) Update(ctx context.Context, id int64, in ProductInput) (*domain.Product, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	p := &domain.Product{
		ID:          id,
		Name:        strings.TrimSpace(in.Name),
		Description: strings.TrimSpace(in.Description),
		PriceCents:  in.PriceCents,
		Stock:       in.Stock,
	}
	if err := s.products.Update(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Delete removes a product, returning domain.ErrNotFound if it is absent.
func (s *ProductService) Delete(ctx context.Context, id int64) error {
	return s.products.Delete(ctx, id)
}
