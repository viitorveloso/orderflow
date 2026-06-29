// Package httpapi is the HTTP transport layer: routing, middleware, request
// decoding, and response encoding. It depends on the service interfaces defined
// here, so handlers can be tested with fake services and no network or DB.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/yourusername/orderflow/internal/auth"
	"github.com/yourusername/orderflow/internal/domain"
	"github.com/yourusername/orderflow/internal/service"
)

// AuthService is the subset of the auth use cases the handlers need.
type AuthService interface {
	Register(ctx context.Context, in service.RegisterInput) (*domain.User, error)
	Login(ctx context.Context, email, password string) (string, *domain.User, error)
}

// ProductService is the subset of catalog use cases the handlers need.
type ProductService interface {
	List(ctx context.Context, limit, offset int) ([]domain.Product, error)
	Get(ctx context.Context, id int64) (*domain.Product, error)
	Create(ctx context.Context, in service.ProductInput) (*domain.Product, error)
	Update(ctx context.Context, id int64, in service.ProductInput) (*domain.Product, error)
	Delete(ctx context.Context, id int64) error
}

// OrderService is the subset of order use cases the handlers need.
type OrderService interface {
	Create(ctx context.Context, userID int64, in service.CreateOrderInput) (*domain.Order, error)
	Get(ctx context.Context, actorID int64, role domain.Role, orderID int64) (*domain.Order, error)
	ListByUser(ctx context.Context, userID int64, limit, offset int) ([]domain.Order, error)
	UpdateStatus(ctx context.Context, orderID int64, next domain.OrderStatus) (*domain.Order, error)
}

// Server holds the dependencies shared by all handlers.
type Server struct {
	auth     AuthService
	products ProductService
	orders   OrderService
	tokens   *auth.TokenManager
	logger   *slog.Logger
}

// NewServer wires a Server.
func NewServer(
	authSvc AuthService,
	products ProductService,
	orders OrderService,
	tokens *auth.TokenManager,
	logger *slog.Logger,
) *Server {
	return &Server{
		auth:     authSvc,
		products: products,
		orders:   orders,
		tokens:   tokens,
		logger:   logger,
	}
}

// Routes builds the HTTP handler with all routes and the global middleware
// chain. It uses the Go 1.22 ServeMux, whose patterns carry the method and path
// wildcards (e.g. "GET /products/{id}"), so no third-party router is needed.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// Health check.
	mux.HandleFunc("GET /healthz", s.handleHealth)

	// Authentication (public).
	mux.HandleFunc("POST /auth/register", s.handleRegister)
	mux.HandleFunc("POST /auth/login", s.handleLogin)

	// Catalog reads (public).
	mux.HandleFunc("GET /products", s.handleListProducts)
	mux.HandleFunc("GET /products/{id}", s.handleGetProduct)

	// Catalog writes (admin only).
	mux.Handle("POST /products", s.requireAdmin(http.HandlerFunc(s.handleCreateProduct)))
	mux.Handle("PUT /products/{id}", s.requireAdmin(http.HandlerFunc(s.handleUpdateProduct)))
	mux.Handle("DELETE /products/{id}", s.requireAdmin(http.HandlerFunc(s.handleDeleteProduct)))

	// Orders (authenticated).
	mux.Handle("POST /orders", s.requireAuth(http.HandlerFunc(s.handleCreateOrder)))
	mux.Handle("GET /orders", s.requireAuth(http.HandlerFunc(s.handleListOrders)))
	mux.Handle("GET /orders/{id}", s.requireAuth(http.HandlerFunc(s.handleGetOrder)))
	mux.Handle("PATCH /orders/{id}/status", s.requireAdmin(http.HandlerFunc(s.handleUpdateOrderStatus)))

	// Global middleware, outermost first.
	return s.recoverer(s.logging(mux))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// idParam parses the {id} path wildcard as an int64.
func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// pageParams reads optional ?limit= and ?offset= query parameters. Invalid or
// missing values become zero, which the service clamps to safe defaults.
func pageParams(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	return limit, offset
}
