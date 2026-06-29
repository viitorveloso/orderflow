package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/yourusername/orderflow/internal/auth"
	"github.com/yourusername/orderflow/internal/domain"
	"github.com/yourusername/orderflow/internal/service"
)

// --- fakes implementing the handler-facing service interfaces ---------------

type fakeAuthService struct {
	registerFn func(context.Context, service.RegisterInput) (*domain.User, error)
	loginFn    func(context.Context, string, string) (string, *domain.User, error)
}

func (f fakeAuthService) Register(ctx context.Context, in service.RegisterInput) (*domain.User, error) {
	return f.registerFn(ctx, in)
}
func (f fakeAuthService) Login(ctx context.Context, email, pw string) (string, *domain.User, error) {
	return f.loginFn(ctx, email, pw)
}

type fakeProductService struct {
	listFn   func(context.Context, int, int) ([]domain.Product, error)
	getFn    func(context.Context, int64) (*domain.Product, error)
	createFn func(context.Context, service.ProductInput) (*domain.Product, error)
	updateFn func(context.Context, int64, service.ProductInput) (*domain.Product, error)
	deleteFn func(context.Context, int64) error
}

func (f fakeProductService) List(ctx context.Context, l, o int) ([]domain.Product, error) {
	return f.listFn(ctx, l, o)
}
func (f fakeProductService) Get(ctx context.Context, id int64) (*domain.Product, error) {
	return f.getFn(ctx, id)
}
func (f fakeProductService) Create(ctx context.Context, in service.ProductInput) (*domain.Product, error) {
	return f.createFn(ctx, in)
}
func (f fakeProductService) Update(ctx context.Context, id int64, in service.ProductInput) (*domain.Product, error) {
	return f.updateFn(ctx, id, in)
}
func (f fakeProductService) Delete(ctx context.Context, id int64) error {
	return f.deleteFn(ctx, id)
}

type fakeOrderService struct {
	createFn func(context.Context, int64, service.CreateOrderInput) (*domain.Order, error)
	getFn    func(context.Context, int64, domain.Role, int64) (*domain.Order, error)
	listFn   func(context.Context, int64, int, int) ([]domain.Order, error)
	statusFn func(context.Context, int64, domain.OrderStatus) (*domain.Order, error)
}

func (f fakeOrderService) Create(ctx context.Context, uid int64, in service.CreateOrderInput) (*domain.Order, error) {
	return f.createFn(ctx, uid, in)
}
func (f fakeOrderService) Get(ctx context.Context, aid int64, role domain.Role, oid int64) (*domain.Order, error) {
	return f.getFn(ctx, aid, role, oid)
}
func (f fakeOrderService) ListByUser(ctx context.Context, uid int64, l, o int) ([]domain.Order, error) {
	return f.listFn(ctx, uid, l, o)
}
func (f fakeOrderService) UpdateStatus(ctx context.Context, oid int64, st domain.OrderStatus) (*domain.Order, error) {
	return f.statusFn(ctx, oid, st)
}

// --- helpers ----------------------------------------------------------------

func newTestServer(a AuthService, p ProductService, o OrderService) (*Server, *auth.TokenManager) {
	tm, _ := auth.NewTokenManager("test-secret-1234567890", time.Hour)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewServer(a, p, o, tm, logger), tm
}

func doRequest(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// --- tests ------------------------------------------------------------------

func TestHealth(t *testing.T) {
	srv, _ := newTestServer(fakeAuthService{}, fakeProductService{}, fakeOrderService{})
	rec := doRequest(t, srv.Routes(), http.MethodGet, "/healthz", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestRegisterCreated(t *testing.T) {
	authSvc := fakeAuthService{
		registerFn: func(_ context.Context, in service.RegisterInput) (*domain.User, error) {
			return &domain.User{ID: 1, Email: in.Email, Role: domain.RoleUser}, nil
		},
	}
	srv, _ := newTestServer(authSvc, fakeProductService{}, fakeOrderService{})

	rec := doRequest(t, srv.Routes(), http.MethodPost, "/auth/register", "", map[string]string{
		"email": "new@example.com", "password": "supersecret",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	// The password hash must never be serialized.
	if bytes.Contains(rec.Body.Bytes(), []byte("password_hash")) {
		t.Error("response leaked password_hash")
	}
}

func TestRegisterValidationReturns422(t *testing.T) {
	authSvc := fakeAuthService{
		registerFn: func(_ context.Context, _ service.RegisterInput) (*domain.User, error) {
			fe := domain.FieldErrors{}
			fe.Add("email", "must be a valid email address")
			return nil, fe
		},
	}
	srv, _ := newTestServer(authSvc, fakeProductService{}, fakeOrderService{})

	rec := doRequest(t, srv.Routes(), http.MethodPost, "/auth/register", "", map[string]string{
		"email": "bad", "password": "x",
	})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	var resp errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Fields["email"] == "" {
		t.Error("expected an email field error in the response")
	}
}

func TestLoginReturnsToken(t *testing.T) {
	authSvc := fakeAuthService{
		loginFn: func(_ context.Context, _, _ string) (string, *domain.User, error) {
			return "tok-123", &domain.User{ID: 1, Email: "a@b.com", Role: domain.RoleUser}, nil
		},
	}
	srv, _ := newTestServer(authSvc, fakeProductService{}, fakeOrderService{})

	rec := doRequest(t, srv.Routes(), http.MethodPost, "/auth/login", "", map[string]string{
		"email": "a@b.com", "password": "supersecret",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["token"] != "tok-123" {
		t.Errorf("token = %v, want tok-123", resp["token"])
	}
}

func TestCreateProductRequiresAdmin(t *testing.T) {
	products := fakeProductService{
		createFn: func(_ context.Context, in service.ProductInput) (*domain.Product, error) {
			return &domain.Product{ID: 1, Name: in.Name}, nil
		},
	}
	srv, tm := newTestServer(fakeAuthService{}, products, fakeOrderService{})
	handler := srv.Routes()
	payload := map[string]any{"name": "Widget", "price_cents": 1000, "stock": 5}

	// No token -> 401.
	if rec := doRequest(t, handler, http.MethodPost, "/products", "", payload); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", rec.Code)
	}

	// Authenticated non-admin -> 403.
	userTok, _ := tm.Generate(2, domain.RoleUser)
	if rec := doRequest(t, handler, http.MethodPost, "/products", userTok, payload); rec.Code != http.StatusForbidden {
		t.Errorf("user token: status = %d, want 403", rec.Code)
	}

	// Admin -> 201.
	adminTok, _ := tm.Generate(1, domain.RoleAdmin)
	if rec := doRequest(t, handler, http.MethodPost, "/products", adminTok, payload); rec.Code != http.StatusCreated {
		t.Errorf("admin token: status = %d, want 201", rec.Code)
	}
}

func TestGetProductNotFound(t *testing.T) {
	products := fakeProductService{
		getFn: func(_ context.Context, _ int64) (*domain.Product, error) {
			return nil, domain.ErrNotFound
		},
	}
	srv, _ := newTestServer(fakeAuthService{}, products, fakeOrderService{})

	rec := doRequest(t, srv.Routes(), http.MethodGet, "/products/999", "", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestCreateOrderFlow(t *testing.T) {
	orders := fakeOrderService{
		createFn: func(_ context.Context, uid int64, in service.CreateOrderInput) (*domain.Order, error) {
			if uid != 5 {
				t.Errorf("user id = %d, want 5 (from token subject)", uid)
			}
			return &domain.Order{ID: 1, UserID: uid, Status: domain.StatusPending, TotalCents: 2000}, nil
		},
	}
	srv, tm := newTestServer(fakeAuthService{}, fakeProductService{}, orders)
	handler := srv.Routes()
	payload := map[string]any{"items": []map[string]any{{"product_id": 1, "quantity": 2}}}

	// Unauthenticated -> 401.
	if rec := doRequest(t, handler, http.MethodPost, "/orders", "", payload); rec.Code != http.StatusUnauthorized {
		t.Errorf("no token: status = %d, want 401", rec.Code)
	}

	// Authenticated -> 201, and the handler must pass the token's user id through.
	tok, _ := tm.Generate(5, domain.RoleUser)
	if rec := doRequest(t, handler, http.MethodPost, "/orders", tok, payload); rec.Code != http.StatusCreated {
		t.Errorf("authenticated: status = %d, want 201", rec.Code)
	}
}

func TestCreateOrderInsufficientStockReturns409(t *testing.T) {
	orders := fakeOrderService{
		createFn: func(_ context.Context, _ int64, _ service.CreateOrderInput) (*domain.Order, error) {
			return nil, domain.ErrInsufficientStock
		},
	}
	srv, tm := newTestServer(fakeAuthService{}, fakeProductService{}, orders)
	tok, _ := tm.Generate(5, domain.RoleUser)

	rec := doRequest(t, srv.Routes(), http.MethodPost, "/orders", tok,
		map[string]any{"items": []map[string]any{{"product_id": 1, "quantity": 99}}})
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}
