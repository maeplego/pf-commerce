package memory

import (
	"context"
	"sync"

	"github.com/portfolio/pf-commerce/apps/api/internal/cart"
)

type Store struct {
	mu    sync.Mutex
	carts map[string]cart.Cart
}

func New() *Store {
	return &Store{carts: map[string]cart.Cart{}}
}

func (s *Store) Ping(context.Context) error { return nil }

func (s *Store) Get(_ context.Context, buyerSub string) (cart.Cart, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.carts[buyerSub]
	if !ok {
		return cart.Cart{BuyerSub: buyerSub, Items: []cart.Item{}}, nil
	}
	return c, nil
}

func (s *Store) Replace(_ context.Context, c cart.Cart) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := append([]cart.Item{}, c.Items...)
	s.carts[c.BuyerSub] = cart.Cart{BuyerSub: c.BuyerSub, Items: items}
	return nil
}

func (s *Store) Clear(_ context.Context, buyerSub string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.carts, buyerSub)
	return nil
}
