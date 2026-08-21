package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/portfolio/pf-commerce/apps/catalog/internal/catalog"
)

type Store struct {
	mu            sync.Mutex
	products      map[string]catalog.Product
	productsBySKU map[string]string
	reviews       []catalog.Review
}

func New() *Store {
	return &Store{products: map[string]catalog.Product{}, productsBySKU: map[string]string{}}
}

func (s *Store) Ping(context.Context) error { return nil }

func (s *Store) Create(_ context.Context, p catalog.Product) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.productsBySKU[p.SKU]; ok {
		return catalog.ErrConflict
	}
	s.products[p.ID] = p
	s.productsBySKU[p.SKU] = p.ID
	return nil
}

func (s *Store) Get(_ context.Context, id string) (catalog.Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.products[id]
	if !ok {
		return catalog.Product{}, catalog.ErrNotFound
	}
	return p, nil
}

func (s *Store) GetBySKU(_ context.Context, sku string) (catalog.Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.productsBySKU[sku]
	if !ok {
		return catalog.Product{}, catalog.ErrNotFound
	}
	return s.products[id], nil
}

func (s *Store) List(_ context.Context, orgID string) ([]catalog.Product, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]catalog.Product, 0, len(s.products))
	for _, p := range s.products {
		if p.OrgID == orgID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SKU < out[j].SKU })
	return out, nil
}

func (s *Store) AddReview(_ context.Context, r catalog.Review) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reviews = append(s.reviews, r)
	return nil
}

func (s *Store) ListReviews(_ context.Context, productIDs []string) ([]catalog.Review, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]struct{}{}
	for _, id := range productIDs {
		want[id] = struct{}{}
	}
	var out []catalog.Review
	for _, r := range s.reviews {
		if _, ok := want[r.ProductID]; ok || len(want) == 0 {
			if len(want) == 0 {
				continue
			}
			out = append(out, r)
		}
	}
	return out, nil
}
