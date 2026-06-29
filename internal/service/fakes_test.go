package service

import (
	"context"
	"sort"
	"time"

	"github.com/yourusername/orderflow/internal/domain"
)

// These in-memory fakes implement the repository interfaces so the service
// layer can be unit tested without a database. They are deliberately simple but
// honor the documented contracts (e.g. returning domain.ErrConflict /
// domain.ErrNotFound) that the services rely on.

type fakeUserRepo struct {
	byEmail map[string]*domain.User
	nextID  int64
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byEmail: map[string]*domain.User{}}
}

func (r *fakeUserRepo) Create(_ context.Context, u *domain.User) error {
	if _, exists := r.byEmail[u.Email]; exists {
		return domain.ErrConflict
	}
	r.nextID++
	u.ID = r.nextID
	u.CreatedAt = time.Now()
	stored := *u
	r.byEmail[u.Email] = &stored
	return nil
}

func (r *fakeUserRepo) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	u, ok := r.byEmail[email]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *u
	return &cp, nil
}

type fakeProductRepo struct {
	items  map[int64]*domain.Product
	nextID int64
}

func newFakeProductRepo() *fakeProductRepo {
	return &fakeProductRepo{items: map[int64]*domain.Product{}}
}

func (r *fakeProductRepo) Create(_ context.Context, p *domain.Product) error {
	r.nextID++
	p.ID = r.nextID
	now := time.Now()
	p.CreatedAt, p.UpdatedAt = now, now
	cp := *p
	r.items[p.ID] = &cp
	return nil
}

func (r *fakeProductRepo) GetByID(_ context.Context, id int64) (*domain.Product, error) {
	p, ok := r.items[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *fakeProductRepo) ListByIDs(_ context.Context, ids []int64) ([]domain.Product, error) {
	var out []domain.Product
	for _, id := range ids {
		if p, ok := r.items[id]; ok {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (r *fakeProductRepo) List(_ context.Context, limit, offset int) ([]domain.Product, error) {
	ids := make([]int64, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var out []domain.Product
	for i, id := range ids {
		if i < offset {
			continue
		}
		if len(out) >= limit {
			break
		}
		out = append(out, *r.items[id])
	}
	return out, nil
}

func (r *fakeProductRepo) Update(_ context.Context, p *domain.Product) error {
	existing, ok := r.items[p.ID]
	if !ok {
		return domain.ErrNotFound
	}
	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = time.Now()
	cp := *p
	r.items[p.ID] = &cp
	return nil
}

func (r *fakeProductRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.items[id]; !ok {
		return domain.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

type fakeOrderRepo struct {
	orders map[int64]*domain.Order
	nextID int64

	// createErr, when set, is returned by Create to simulate repository-level
	// failures (e.g. domain.ErrInsufficientStock from the conditional UPDATE).
	createErr error
	// lastCreated captures the order most recently passed to Create so tests can
	// assert on computed totals and snapshotted prices.
	lastCreated *domain.Order
}

func newFakeOrderRepo() *fakeOrderRepo {
	return &fakeOrderRepo{orders: map[int64]*domain.Order{}}
}

func (r *fakeOrderRepo) Create(_ context.Context, o *domain.Order) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.nextID++
	o.ID = r.nextID
	o.CreatedAt = time.Now()
	for i := range o.Items {
		o.Items[i].ID = int64(i + 1)
	}
	cp := *o
	r.orders[o.ID] = &cp
	r.lastCreated = &cp
	return nil
}

func (r *fakeOrderRepo) GetByID(_ context.Context, id int64) (*domain.Order, error) {
	o, ok := r.orders[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	cp := *o
	return &cp, nil
}

func (r *fakeOrderRepo) ListByUser(_ context.Context, userID int64, limit, offset int) ([]domain.Order, error) {
	var out []domain.Order
	for _, o := range r.orders {
		if o.UserID == userID {
			out = append(out, *o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (r *fakeOrderRepo) UpdateStatus(_ context.Context, id int64, status domain.OrderStatus) error {
	o, ok := r.orders[id]
	if !ok {
		return domain.ErrNotFound
	}
	o.Status = status
	return nil
}
