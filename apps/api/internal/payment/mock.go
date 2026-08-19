package payment

import (
	"context"
	"errors"
	"sync"

	"github.com/portfolio/pf-commerce/api/internal/id"
	"github.com/portfolio/pf-commerce/api/internal/money"
)

var ErrDeclined = errors.New("payment declined")

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
	Amount         money.Amount
}

type Gateway interface {
	Charge(ctx context.Context, req ChargeRequest) (Charge, error)
}

type Mock struct {
	mu       sync.Mutex
	failNext int
	byKey    map[string]Charge
}

func NewMock() *Mock {
	return &Mock{byKey: map[string]Charge{}}
}

func (m *Mock) FailNext(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failNext = n
}

func (m *Mock) Charge(_ context.Context, req ChargeRequest) (Charge, error) {
	if req.IdempotencyKey == "" || req.OrderID == "" {
		return Charge{}, errors.New("invalid charge")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.byKey[req.IdempotencyKey]; ok {
		return existing, nil
	}
	if m.failNext > 0 {
		m.failNext--
		return Charge{}, ErrDeclined
	}
	ch := Charge{
		ID:             id.New(),
		IdempotencyKey: req.IdempotencyKey,
		OrderID:        req.OrderID,
		Amount:         req.Amount,
	}
	m.byKey[req.IdempotencyKey] = ch
	return ch, nil
}
