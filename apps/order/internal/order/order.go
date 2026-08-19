package order

import (
	"context"
	"errors"
	"time"

	"github.com/portfolio/pf-commerce/packages/id"
	"github.com/portfolio/pf-commerce/packages/money"
)

var (
	ErrNotFound  = errors.New("order not found")
	ErrInvalid   = errors.New("invalid order")
	ErrConflict  = errors.New("order conflict")
	ErrForbidden = errors.New("order forbidden")
	ErrShortage  = errors.New("inventory shortage")
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusPaid      Status = "paid"
	StatusCancelled Status = "cancelled"
)

const (
	ReasonShortage = "inventory_shortage"
	ReasonPayment  = "payment_failed"
)

type Line struct {
	ProductID      string
	SKU            string
	Name           string
	Qty            int
	UnitPriceMinor int64
	Currency       string
}

type Order struct {
	ID             string
	BuyerSub       string
	Status         Status
	CancelReason   string
	Amount         money.Amount
	IdempotencyKey string
	PaymentID      string
	Lines          []Line
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Repository interface {
	Create(ctx context.Context, o Order) error
	Get(ctx context.Context, id string) (Order, error)
	GetByIdempotency(ctx context.Context, buyerSub, key string) (Order, error)
	ListByBuyer(ctx context.Context, buyerSub string) ([]Order, error)
	Update(ctx context.Context, o Order) error
}

type CheckoutInput struct {
	BuyerSub       string
	IdempotencyKey string
	SiteID         string
	Lines          []CheckoutLine
}

type CheckoutLine struct {
	ProductID string
	Qty       int
}

type Product struct {
	ID         string
	SKU        string
	Name       string
	PriceMinor int64
	Currency   string
	Active     bool
}

type CatalogClient interface {
	Get(ctx context.Context, productID string) (Product, error)
}

type StockClient interface {
	Reserve(ctx context.Context, siteID, productID, orderID, actorID string, qty int) error
	ReleaseOrder(ctx context.Context, orderID, actorID, reason string) error
	ConsumeOrder(ctx context.Context, orderID, actorID string) error
}

type Service struct {
	orders  Repository
	catalog CatalogClient
	stock   StockClient
	pay     Gateway
	now     func() time.Time
}

func NewService(orders Repository, cat CatalogClient, stock StockClient, pay Gateway, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{orders: orders, catalog: cat, stock: stock, pay: pay, now: now}
}

func (s *Service) Get(ctx context.Context, orderID, actorSub string, ops bool) (Order, error) {
	o, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	if !ops && o.BuyerSub != actorSub {
		return Order{}, ErrForbidden
	}
	return o, nil
}

func (s *Service) ListMine(ctx context.Context, buyerSub string) ([]Order, error) {
	if buyerSub == "" {
		return nil, ErrInvalid
	}
	return s.orders.ListByBuyer(ctx, buyerSub)
}

func (s *Service) Checkout(ctx context.Context, in CheckoutInput) (Order, bool, error) {
	if in.BuyerSub == "" || in.IdempotencyKey == "" || in.SiteID == "" {
		return Order{}, false, ErrInvalid
	}
	if existing, err := s.orders.GetByIdempotency(ctx, in.BuyerSub, in.IdempotencyKey); err == nil {
		return existing, false, nil
	} else if err != ErrNotFound {
		return Order{}, false, err
	}

	lines, total, err := s.priceLines(ctx, in.Lines)
	if err != nil {
		return Order{}, false, err
	}

	now := s.now()
	o := Order{
		ID:             id.New(),
		BuyerSub:       in.BuyerSub,
		Status:         StatusPending,
		Amount:         total,
		IdempotencyKey: in.IdempotencyKey,
		Lines:          lines,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.orders.Create(ctx, o); err != nil {
		if errors.Is(err, ErrConflict) {
			existing, gerr := s.orders.GetByIdempotency(ctx, in.BuyerSub, in.IdempotencyKey)
			return existing, false, gerr
		}
		return Order{}, false, err
	}

	for _, ln := range lines {
		if err := s.stock.Reserve(ctx, in.SiteID, ln.ProductID, o.ID, in.BuyerSub, ln.Qty); err != nil {
			_ = s.stock.ReleaseOrder(ctx, o.ID, in.BuyerSub, ReasonShortage)
			o.Status = StatusCancelled
			o.CancelReason = ReasonShortage
			o.UpdatedAt = s.now()
			if uerr := s.orders.Update(ctx, o); uerr != nil {
				return Order{}, true, uerr
			}
			if errors.Is(err, ErrShortage) {
				return o, true, ErrShortage
			}
			return o, true, err
		}
	}

	ch, err := s.pay.Charge(ctx, ChargeRequest{
		IdempotencyKey: "pay:" + in.IdempotencyKey,
		OrderID:        o.ID,
		BuyerSub:       in.BuyerSub,
		Amount:         total,
	})
	if err != nil {
		_ = s.stock.ReleaseOrder(ctx, o.ID, in.BuyerSub, ReasonPayment)
		o.Status = StatusCancelled
		o.CancelReason = ReasonPayment
		o.UpdatedAt = s.now()
		if uerr := s.orders.Update(ctx, o); uerr != nil {
			return Order{}, true, uerr
		}
		return o, true, err
	}

	if err := s.stock.ConsumeOrder(ctx, o.ID, in.BuyerSub); err != nil {
		_ = s.stock.ReleaseOrder(ctx, o.ID, in.BuyerSub, "consume_failed")
		o.Status = StatusCancelled
		o.CancelReason = "consume_failed"
		o.UpdatedAt = s.now()
		_ = s.orders.Update(ctx, o)
		return o, true, err
	}

	o.Status = StatusPaid
	o.PaymentID = ch.ID
	o.UpdatedAt = s.now()
	if err := s.orders.Update(ctx, o); err != nil {
		return Order{}, true, err
	}
	return o, true, nil
}

func (s *Service) priceLines(ctx context.Context, raw []CheckoutLine) ([]Line, money.Amount, error) {
	if len(raw) == 0 {
		return nil, money.Amount{}, ErrInvalid
	}
	var total money.Amount
	lines := make([]Line, 0, len(raw))
	for i, r := range raw {
		if r.Qty <= 0 {
			return nil, money.Amount{}, ErrInvalid
		}
		p, err := s.catalog.Get(ctx, r.ProductID)
		if err != nil {
			return nil, money.Amount{}, err
		}
		if !p.Active {
			return nil, money.Amount{}, ErrInvalid
		}
		unit, err := money.New(p.PriceMinor, p.Currency)
		if err != nil {
			return nil, money.Amount{}, err
		}
		lineAmt, err := unit.MulQty(r.Qty)
		if err != nil {
			return nil, money.Amount{}, err
		}
		if i == 0 {
			total = lineAmt
		} else {
			total, err = total.Add(lineAmt)
			if err != nil {
				return nil, money.Amount{}, err
			}
		}
		lines = append(lines, Line{
			ProductID:      p.ID,
			SKU:            p.SKU,
			Name:           p.Name,
			Qty:            r.Qty,
			UnitPriceMinor: p.PriceMinor,
			Currency:       p.Currency,
		})
	}
	return lines, total, nil
}
