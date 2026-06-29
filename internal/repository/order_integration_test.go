//go:build integration

// These tests require a real Postgres instance. Set TEST_DATABASE_URL and run:
//
//	go test -tags=integration ./internal/repository/...
//
// They are excluded from the default build so the unit suite runs anywhere with
// no external services.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"testing"

	"github.com/yourusername/orderflow/internal/database"
	"github.com/yourusername/orderflow/internal/domain"
	"github.com/yourusername/orderflow/migrations"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration tests")
	}
	ctx := context.Background()
	db, err := database.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := database.RunMigrations(ctx, db, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`TRUNCATE order_items, orders, products, users RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedUser(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(
		`INSERT INTO users (email, password_hash, role) VALUES ($1, $2, $3) RETURNING id`,
		"buyer@example.com", "x", domain.RoleUser,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func TestOrderCreateDecrementsStock(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	userID := seedUser(t, db)

	products := NewProductRepo(db)
	widget := &domain.Product{Name: "Widget", PriceCents: 1000, Stock: 10}
	if err := products.Create(ctx, widget); err != nil {
		t.Fatal(err)
	}

	orders := NewOrderRepo(db)
	order := &domain.Order{
		UserID:     userID,
		Status:     domain.StatusPending,
		TotalCents: 3000,
		Items: []domain.OrderItem{
			{ProductID: widget.ID, Quantity: 3, UnitPriceCents: 1000},
		},
	}
	if err := orders.Create(ctx, order); err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.ID == 0 || order.Items[0].ID == 0 {
		t.Error("expected generated IDs")
	}

	got, err := products.GetByID(ctx, widget.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stock != 7 {
		t.Errorf("stock = %d, want 7", got.Stock)
	}
}

func TestOrderCreateRejectsInsufficientStock(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	userID := seedUser(t, db)

	products := NewProductRepo(db)
	p := &domain.Product{Name: "Rare", PriceCents: 500, Stock: 2}
	if err := products.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	orders := NewOrderRepo(db)
	err := orders.Create(ctx, &domain.Order{
		UserID:     userID,
		Status:     domain.StatusPending,
		TotalCents: 1500,
		Items:      []domain.OrderItem{{ProductID: p.ID, Quantity: 3, UnitPriceCents: 500}},
	})
	if !errors.Is(err, domain.ErrInsufficientStock) {
		t.Fatalf("expected ErrInsufficientStock, got %v", err)
	}

	// Stock must be untouched after the failed, rolled-back order.
	got, err := products.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stock != 2 {
		t.Errorf("stock = %d, want 2 (rolled back)", got.Stock)
	}
}

// TestConcurrentOrdersDoNotOversell hammers a single-unit product with many
// simultaneous orders and asserts exactly one succeeds. This is the core
// race-condition guarantee of the conditional UPDATE.
func TestConcurrentOrdersDoNotOversell(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	userID := seedUser(t, db)

	products := NewProductRepo(db)
	p := &domain.Product{Name: "LastOne", PriceCents: 100, Stock: 1}
	if err := products.Create(ctx, p); err != nil {
		t.Fatal(err)
	}

	orders := NewOrderRepo(db)
	const goroutines = 20

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		success  int
		conflict int
	)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := orders.Create(ctx, &domain.Order{
				UserID:     userID,
				Status:     domain.StatusPending,
				TotalCents: 100,
				Items:      []domain.OrderItem{{ProductID: p.ID, Quantity: 1, UnitPriceCents: 100}},
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				success++
			case errors.Is(err, domain.ErrInsufficientStock):
				conflict++
			default:
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if success != 1 {
		t.Errorf("successful orders = %d, want exactly 1", success)
	}
	if conflict != goroutines-1 {
		t.Errorf("conflicts = %d, want %d", conflict, goroutines-1)
	}

	got, err := products.GetByID(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stock != 0 {
		t.Errorf("final stock = %d, want 0", got.Stock)
	}
}
