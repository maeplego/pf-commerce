package payment

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/portfolio/pf-commerce/packages/id"
	"github.com/portfolio/pf-commerce/packages/money"
)

var (
	ErrInvalid  = errors.New("invalid charge")
	ErrDeclined = errors.New("payment declined")
)

// ChargeRequest never includes card numbers. PCI stays out of this service.
type ChargeRequest struct {
	IdempotencyKey string
	OrderID        string
	BuyerSub       string
	Amount         money.Amount
}

type Charge struct {
	ID             string
	IdempotencyKey string
	OrderID        string
	BuyerSub       string
	Amount         money.Amount
	CreatedAt      time.Time
}

type Repository interface {
	GetByKey(ctx context.Context, key string) (Charge, error)
	Insert(ctx context.Context, ch Charge) error
}

type Service struct {
	repo     Repository
	now      func() time.Time
	mu       sync.Mutex
	failNext int
}

func NewService(repo Repository, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	if repo == nil {
		repo = NewMemory()
	}
	return &Service{repo: repo, now: now}
}

func (s *Service) FailNext(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext = n
}

func (s *Service) Charge(ctx context.Context, req ChargeRequest) (Charge, error) {
	if req.IdempotencyKey == "" || req.OrderID == "" {
		return Charge{}, ErrInvalid
	}
	if existing, err := s.repo.GetByKey(ctx, req.IdempotencyKey); err == nil {
		return existing, nil
	} else if err != ErrNotFound {
		return Charge{}, err
	}

	s.mu.Lock()
	if s.failNext > 0 {
		s.failNext--
		s.mu.Unlock()
		return Charge{}, ErrDeclined
	}
	s.mu.Unlock()

	ch := Charge{
		ID:             id.New(),
		IdempotencyKey: req.IdempotencyKey,
		OrderID:        req.OrderID,
		BuyerSub:       req.BuyerSub,
		Amount:         req.Amount,
		CreatedAt:      s.now(),
	}
	if err := s.repo.Insert(ctx, ch); err != nil {
		if existing, gerr := s.repo.GetByKey(ctx, req.IdempotencyKey); gerr == nil {
			return existing, nil
		}
		return Charge{}, err
	}
	return ch, nil
}

var ErrNotFound = errors.New("charge not found")
