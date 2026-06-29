package service

import (
	"context"
	"errors"
	"testing"

	"github.com/yourusername/orderflow/internal/domain"
)

func TestCreateProductValidation(t *testing.T) {
	svc := NewProductService(newFakeProductRepo())

	_, err := svc.Create(context.Background(), ProductInput{
		Name:       "   ",
		PriceCents: -100,
		Stock:      -5,
	})
	var fields domain.FieldErrors
	if !errors.As(err, &fields) {
		t.Fatalf("expected FieldErrors, got %v", err)
	}
	for _, f := range []string{"name", "price_cents", "stock"} {
		if _, ok := fields[f]; !ok {
			t.Errorf("expected field error for %q", f)
		}
	}
}

func TestCreateProductTrimsAndStores(t *testing.T) {
	repo := newFakeProductRepo()
	svc := NewProductService(repo)

	p, err := svc.Create(context.Background(), ProductInput{
		Name:        "  Keyboard  ",
		Description: "  mechanical  ",
		PriceCents:  12900,
		Stock:       10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == 0 {
		t.Error("expected an assigned ID")
	}
	if p.Name != "Keyboard" || p.Description != "mechanical" {
		t.Errorf("fields not trimmed: %+v", p)
	}
}

func TestUpdateMissingProduct(t *testing.T) {
	svc := NewProductService(newFakeProductRepo())

	_, err := svc.Update(context.Background(), 999, ProductInput{
		Name:       "Ghost",
		PriceCents: 100,
		Stock:      1,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteMissingProduct(t *testing.T) {
	svc := NewProductService(newFakeProductRepo())
	if err := svc.Delete(context.Background(), 12345); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListProductsPaginationClamp(t *testing.T) {
	repo := newFakeProductRepo()
	svc := NewProductService(repo)
	for i := 0; i < 5; i++ {
		if _, err := svc.Create(context.Background(), ProductInput{Name: "p", PriceCents: 1, Stock: 1}); err != nil {
			t.Fatal(err)
		}
	}

	// A non-positive limit should fall back to the default and still return rows.
	got, err := svc.List(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Errorf("got %d products, want 5", len(got))
	}
}
