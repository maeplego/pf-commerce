package catalog

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/portfolio/pf-commerce/packages/auth"
	"github.com/portfolio/pf-commerce/packages/id"
	"github.com/portfolio/pf-commerce/packages/money"
)

var (
	ErrNotFound = errors.New("product not found")
	ErrInvalid  = errors.New("invalid product")
	ErrConflict = errors.New("product conflict")
)

type Product struct {
	ID          string
	OrgID       string
	SKU         string
	Name        string
	Description string
	Price       money.Amount
	ImageURL    string
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Review struct {
	ID        string
	ProductID string
	Author    string
	Body      string
	CreatedAt time.Time
}

type Repository interface {
	Create(ctx context.Context, p Product) error
	Get(ctx context.Context, id string) (Product, error)
	GetBySKU(ctx context.Context, sku string) (Product, error)
	List(ctx context.Context, orgID string) ([]Product, error)
	AddReview(ctx context.Context, r Review) error
	ListReviews(ctx context.Context, productIDs []string) ([]Review, error)
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

type CreateInput struct {
	OrgID       string
	SKU         string
	Name        string
	Description string
	PriceMinor  int64
	Currency    string
	ImageURL    string
}

func normalizeOrg(orgID string) string {
	if strings.TrimSpace(orgID) == "" {
		return auth.DefaultOrgID
	}
	return strings.TrimSpace(orgID)
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Product, error) {
	sku := strings.ToUpper(strings.TrimSpace(in.SKU))
	name := strings.TrimSpace(in.Name)
	if sku == "" || name == "" {
		return Product{}, ErrInvalid
	}
	price, err := money.New(in.PriceMinor, in.Currency)
	if err != nil {
		return Product{}, err
	}
	if _, err := s.repo.GetBySKU(ctx, sku); err == nil {
		return Product{}, ErrConflict
	} else if err != ErrNotFound {
		return Product{}, err
	}
	now := s.now()
	p := Product{
		ID:          id.New(),
		OrgID:       normalizeOrg(in.OrgID),
		SKU:         sku,
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		Price:       price,
		ImageURL:    strings.TrimSpace(in.ImageURL),
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return Product{}, err
	}
	return p, nil
}

func (s *Service) Get(ctx context.Context, productID string) (Product, error) {
	if err := id.Parse(productID); err != nil {
		return Product{}, ErrInvalid
	}
	return s.repo.Get(ctx, productID)
}

func (s *Service) GetBySKU(ctx context.Context, sku string) (Product, error) {
	sku = strings.ToUpper(strings.TrimSpace(sku))
	if sku == "" {
		return Product{}, ErrInvalid
	}
	return s.repo.GetBySKU(ctx, sku)
}

func (s *Service) List(ctx context.Context, orgID string) ([]Product, error) {
	return s.repo.List(ctx, normalizeOrg(orgID))
}

func (s *Service) AddReview(ctx context.Context, productID, author, body string) (Review, error) {
	if _, err := s.Get(ctx, productID); err != nil {
		return Review{}, err
	}
	author = strings.TrimSpace(author)
	body = strings.TrimSpace(body)
	if author == "" || body == "" {
		return Review{}, ErrInvalid
	}
	r := Review{ID: id.New(), ProductID: productID, Author: author, Body: body, CreatedAt: s.now()}
	if err := s.repo.AddReview(ctx, r); err != nil {
		return Review{}, err
	}
	return r, nil
}

func (s *Service) ListReviews(ctx context.Context, productIDs []string) ([]Review, error) {
	return s.repo.ListReviews(ctx, productIDs)
}
