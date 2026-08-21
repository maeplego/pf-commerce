package cart

import (
	"context"
	"errors"
	"time"

	"github.com/portfolio/pf-commerce/packages/auth"
	"github.com/portfolio/pf-commerce/packages/id"
)

var ErrInvalid = errors.New("invalid cart")

type Item struct {
	ProductID string
	Qty       int
	UpdatedAt time.Time
}

type Cart struct {
	BuyerSub string
	OrgID    string
	Items    []Item
}

type Repository interface {
	Get(ctx context.Context, buyerSub, orgID string) (Cart, error)
	Replace(ctx context.Context, c Cart) error
	Clear(ctx context.Context, buyerSub, orgID string) error
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, now: now}
}

func normalizeOrg(orgID string) string {
	if orgID == "" {
		return auth.DefaultOrgID
	}
	return orgID
}

func (s *Service) Get(ctx context.Context, buyerSub, orgID string) (Cart, error) {
	if buyerSub == "" {
		return Cart{}, ErrInvalid
	}
	orgID = normalizeOrg(orgID)
	c, err := s.repo.Get(ctx, buyerSub, orgID)
	if err != nil {
		return Cart{}, err
	}
	if c.BuyerSub == "" {
		c.BuyerSub = buyerSub
	}
	if c.OrgID == "" {
		c.OrgID = orgID
	}
	if c.Items == nil {
		c.Items = []Item{}
	}
	return c, nil
}

func (s *Service) Add(ctx context.Context, buyerSub, orgID, productID string, qty int) (Cart, error) {
	if qty <= 0 || buyerSub == "" {
		return Cart{}, ErrInvalid
	}
	if err := id.Parse(productID); err != nil {
		return Cart{}, ErrInvalid
	}
	orgID = normalizeOrg(orgID)
	c, err := s.Get(ctx, buyerSub, orgID)
	if err != nil {
		return Cart{}, err
	}
	now := s.now()
	found := false
	for i := range c.Items {
		if c.Items[i].ProductID == productID {
			c.Items[i].Qty += qty
			c.Items[i].UpdatedAt = now
			found = true
			break
		}
	}
	if !found {
		c.Items = append(c.Items, Item{ProductID: productID, Qty: qty, UpdatedAt: now})
	}
	c.BuyerSub = buyerSub
	c.OrgID = orgID
	if err := s.repo.Replace(ctx, c); err != nil {
		return Cart{}, err
	}
	return c, nil
}

func (s *Service) Replace(ctx context.Context, buyerSub, orgID string, items []Item) (Cart, error) {
	if buyerSub == "" {
		return Cart{}, ErrInvalid
	}
	orgID = normalizeOrg(orgID)
	now := s.now()
	out := make([]Item, 0, len(items))
	for _, it := range items {
		if it.Qty <= 0 {
			return Cart{}, ErrInvalid
		}
		if err := id.Parse(it.ProductID); err != nil {
			return Cart{}, ErrInvalid
		}
		it.UpdatedAt = now
		out = append(out, it)
	}
	c := Cart{BuyerSub: buyerSub, OrgID: orgID, Items: out}
	if err := s.repo.Replace(ctx, c); err != nil {
		return Cart{}, err
	}
	return c, nil
}

func (s *Service) Clear(ctx context.Context, buyerSub, orgID string) error {
	if buyerSub == "" {
		return ErrInvalid
	}
	return s.repo.Clear(ctx, buyerSub, normalizeOrg(orgID))
}
