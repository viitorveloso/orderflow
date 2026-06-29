package service

import (
	"context"
	"errors"
	"testing"

	"github.com/yourusername/orderflow/internal/domain"
)

// seedProducts inserts products and returns the repo for order tests.
func seedProducts(t *testing.T) *fakeProductRepo {
	t.Helper()
	repo := newFakeProductRepo()
	// id 1: 1000 cents, stock 5
	// id 2: 250 cents, stock 100
	mustCreate(t, repo, &domain.Product{Name: "Widget", PriceCents: 1000, Stock: 5})
	mustCreate(t, repo, &domain.Product{Name: "Gadget", PriceCents: 250, Stock: 100})
	return repo
}

func mustCreate(t *testing.T, repo *fakeProductRepo, p *domain.Product) {
	t.Helper()
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func TestCreateOrderComputesTotalAndSnapshotsPrice(t *testing.T) {
	products := seedProducts(t)
	orders := newFakeOrderRepo()
	svc := NewOrderService(orders, products)

	order, err := svc.Create(context.Background(), 7, CreateOrderInput{
		Items: []OrderLineInput{
			{ProductID: 1, Quantity: 2}, // 2 * 1000 = 2000
			{ProductID: 2, Quantity: 3}, // 3 *  250 =  750
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if order.UserID != 7 {
		t.Errorf("user_id = %d, want 7", order.UserID)
	}
	if order.Status != domain.StatusPending {
		t.Errorf("status = %q, want pending", order.Status)
	}
	if order.TotalCents != 2750 {
		t.Errorf("total = %d, want 2750", order.TotalCents)
	}
	if len(order.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(order.Items))
	}
	// Each line carries a snapshot of the product price.
	for _, it := range order.Items {
		switch it.ProductID {
		case 1:
			if it.UnitPriceCents != 1000 {
				t.Errorf("product 1 unit price = %d, want 1000", it.UnitPriceCents)
			}
		case 2:
			if it.UnitPriceCents != 250 {
				t.Errorf("product 2 unit price = %d, want 250", it.UnitPriceCents)
			}
		}
	}
}

func TestCreateOrderMergesDuplicateLines(t *testing.T) {
	products := seedProducts(t)
	orders := newFakeOrderRepo()
	svc := NewOrderService(orders, products)

	order, err := svc.Create(context.Background(), 1, CreateOrderInput{
		Items: []OrderLineInput{
			{ProductID: 1, Quantity: 2},
			{ProductID: 1, Quantity: 1}, // merged -> qty 3, total 3000
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(order.Items) != 1 {
		t.Fatalf("items = %d, want 1 merged line", len(order.Items))
	}
	if order.Items[0].Quantity != 3 {
		t.Errorf("merged quantity = %d, want 3", order.Items[0].Quantity)
	}
	if order.TotalCents != 3000 {
		t.Errorf("total = %d, want 3000", order.TotalCents)
	}
}

func TestCreateOrderEmptyItems(t *testing.T) {
	svc := NewOrderService(newFakeOrderRepo(), seedProducts(t))

	_, err := svc.Create(context.Background(), 1, CreateOrderInput{Items: nil})
	var fields domain.FieldErrors
	if !errors.As(err, &fields) {
		t.Fatalf("expected FieldErrors, got %v", err)
	}
	if _, ok := fields["items"]; !ok {
		t.Error("expected items field error")
	}
}

func TestCreateOrderNonPositiveQuantity(t *testing.T) {
	svc := NewOrderService(newFakeOrderRepo(), seedProducts(t))

	_, err := svc.Create(context.Background(), 1, CreateOrderInput{
		Items: []OrderLineInput{{ProductID: 1, Quantity: 0}},
	})
	var fields domain.FieldErrors
	if !errors.As(err, &fields) {
		t.Fatalf("expected FieldErrors, got %v", err)
	}
}

func TestCreateOrderProductNotFound(t *testing.T) {
	svc := NewOrderService(newFakeOrderRepo(), seedProducts(t))

	_, err := svc.Create(context.Background(), 1, CreateOrderInput{
		Items: []OrderLineInput{{ProductID: 999, Quantity: 1}},
	})
	var fields domain.FieldErrors
	if !errors.As(err, &fields) {
		t.Fatalf("expected FieldErrors, got %v", err)
	}
}

func TestCreateOrderInsufficientStockPreCheck(t *testing.T) {
	svc := NewOrderService(newFakeOrderRepo(), seedProducts(t))

	// Product 1 only has stock 5.
	_, err := svc.Create(context.Background(), 1, CreateOrderInput{
		Items: []OrderLineInput{{ProductID: 1, Quantity: 6}},
	})
	if !errors.Is(err, domain.ErrInsufficientStock) {
		t.Errorf("expected ErrInsufficientStock, got %v", err)
	}
}

func TestCreateOrderPropagatesRepositoryStockError(t *testing.T) {
	orders := newFakeOrderRepo()
	// Simulate the authoritative conditional UPDATE failing under a race even
	// though the optimistic pre-check passed.
	orders.createErr = domain.ErrInsufficientStock
	svc := NewOrderService(orders, seedProducts(t))

	_, err := svc.Create(context.Background(), 1, CreateOrderInput{
		Items: []OrderLineInput{{ProductID: 1, Quantity: 1}},
	})
	if !errors.Is(err, domain.ErrInsufficientStock) {
		t.Errorf("expected propagated ErrInsufficientStock, got %v", err)
	}
}

func TestGetOrderAuthorization(t *testing.T) {
	products := seedProducts(t)
	orders := newFakeOrderRepo()
	svc := NewOrderService(orders, products)

	owned, err := svc.Create(context.Background(), 10, CreateOrderInput{
		Items: []OrderLineInput{{ProductID: 2, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Owner can read.
	if _, err := svc.Get(context.Background(), 10, domain.RoleUser, owned.ID); err != nil {
		t.Errorf("owner read failed: %v", err)
	}
	// Admin can read anyone's order.
	if _, err := svc.Get(context.Background(), 999, domain.RoleAdmin, owned.ID); err != nil {
		t.Errorf("admin read failed: %v", err)
	}
	// A different user is told it does not exist (no leak).
	if _, err := svc.Get(context.Background(), 11, domain.RoleUser, owned.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("expected ErrNotFound for non-owner, got %v", err)
	}
}

func TestUpdateStatusTransitions(t *testing.T) {
	products := seedProducts(t)
	orders := newFakeOrderRepo()
	svc := NewOrderService(orders, products)

	order, err := svc.Create(context.Background(), 1, CreateOrderInput{
		Items: []OrderLineInput{{ProductID: 2, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}

	// pending -> paid is allowed.
	updated, err := svc.UpdateStatus(context.Background(), order.ID, domain.StatusPaid)
	if err != nil {
		t.Fatalf("pending->paid: %v", err)
	}
	if updated.Status != domain.StatusPaid {
		t.Errorf("status = %q, want paid", updated.Status)
	}

	// paid -> shipped is allowed.
	if _, err := svc.UpdateStatus(context.Background(), order.ID, domain.StatusShipped); err != nil {
		t.Fatalf("paid->shipped: %v", err)
	}

	// shipped is terminal: shipped -> paid must be rejected.
	if _, err := svc.UpdateStatus(context.Background(), order.ID, domain.StatusPaid); !errors.Is(err, domain.ErrInvalidStatusChange) {
		t.Errorf("expected ErrInvalidStatusChange, got %v", err)
	}
}

func TestUpdateStatusInvalidValue(t *testing.T) {
	products := seedProducts(t)
	orders := newFakeOrderRepo()
	svc := NewOrderService(orders, products)
	order, err := svc.Create(context.Background(), 1, CreateOrderInput{
		Items: []OrderLineInput{{ProductID: 2, Quantity: 1}},
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.UpdateStatus(context.Background(), order.ID, domain.OrderStatus("frozen"))
	var fields domain.FieldErrors
	if !errors.As(err, &fields) {
		t.Errorf("expected FieldErrors for invalid status, got %v", err)
	}
}
