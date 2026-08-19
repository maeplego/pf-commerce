package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/portfolio/pf-commerce/apps/order/internal/order"
)

type Store struct {
	mu     sync.Mutex
	orders map[string]order.Order
}

func New() *Store {
	return &Store{orders: map[string]order.Order{}}
}

func (s *Store) Ping(context.Context) error { return nil }

func (s *Store) Create(_ context.Context, o order.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.orders {
		if existing.BuyerSub == o.BuyerSub && existing.IdempotencyKey == o.IdempotencyKey {
			return order.ErrConflict
		}
	}
	cp := o
	cp.Lines = append([]order.Line{}, o.Lines...)
	s.orders[o.ID] = cp
	return nil
}

func (s *Store) Get(_ context.Context, id string) (order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[id]
	if !ok {
		return order.Order{}, order.ErrNotFound
	}
	return cloneOrder(o), nil
}

func (s *Store) GetByIdempotency(_ context.Context, buyerSub, key string) (order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, o := range s.orders {
		if o.BuyerSub == buyerSub && o.IdempotencyKey == key {
			return cloneOrder(o), nil
		}
	}
	return order.Order{}, order.ErrNotFound
}

func (s *Store) ListByBuyer(_ context.Context, buyerSub string) ([]order.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []order.Order
	for _, o := range s.orders {
		if o.BuyerSub == buyerSub {
			out = append(out, cloneOrder(o))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) Update(_ context.Context, o order.Order) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orders[o.ID]; !ok {
		return order.ErrNotFound
	}
	cp := o
	cp.Lines = append([]order.Line{}, o.Lines...)
	s.orders[o.ID] = cp
	return nil
}

func cloneOrder(o order.Order) order.Order {
	o.Lines = append([]order.Line{}, o.Lines...)
	return o
}
