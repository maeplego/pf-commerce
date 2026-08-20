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
	ErrShortage          = errors.New("inventory shortage")
	ErrInvalidTransition = errors.New("invalid order transition")
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusPaid      Status = "paid"
	StatusCancelled Status = "cancelled"
	StatusShipped   Status = "shipped"
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
	ListAll(ctx context.Context) ([]Order, error)
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

type Notifier interface {
	Send(ctx context.Context, mail Mail) error
}

type Mail struct {
	ID       string
	Type     string
	OrderID  string
	BuyerSub string
	Payload  string
}

type Service struct {
	store   Persistence
	catalog CatalogClient
	stock   StockClient
	pay     Gateway
	notify  Notifier
	now     func() time.Time
}

func NewService(store Persistence, cat CatalogClient, stock StockClient, pay Gateway, notify Notifier, now func() time.Time) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, catalog: cat, stock: stock, pay: pay, notify: notify, now: now}
}

func (s *Service) PublishDue(ctx context.Context) error {
	if s.notify == nil {
		return nil
	}
	msgs, err := s.store.ListUnpublished(ctx, 50)
	if err != nil {
		return err
	}
	var done []string
	for _, m := range msgs {
		o, err := s.store.Get(ctx, m.AggregateID)
		if err != nil {
			return err
		}
		if err := s.notify.Send(ctx, Mail{
			ID: m.ID, Type: m.Type, OrderID: o.ID, BuyerSub: o.BuyerSub, Payload: string(m.Payload),
		}); err != nil {
			if markErr := s.store.MarkPublished(ctx, done, s.now()); markErr != nil {
				return markErr
			}
			return err
		}
		done = append(done, m.ID)
	}
	return s.store.MarkPublished(ctx, done, s.now())
}

func (s *Service) Get(ctx context.Context, orderID, actorSub string, ops bool) (Order, error) {
	o, err := s.store.Get(ctx, orderID)
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
	return s.store.ListByBuyer(ctx, buyerSub)
}

func (s *Service) Events(ctx context.Context, orderID, actorSub string, ops bool) ([]RecordedEvent, error) {
	if _, err := s.Get(ctx, orderID, actorSub, ops); err != nil {
		return nil, err
	}
	return s.store.Load(ctx, orderID)
}

func (s *Service) Rebuild(ctx context.Context) error {
	return s.store.RebuildProjections(ctx)
}

func (s *Service) Ship(ctx context.Context, orderID, actorSub string, ops bool) (Order, error) {
	o, err := s.Get(ctx, orderID, actorSub, ops)
	if err != nil {
		return Order{}, err
	}
	evs, err := s.store.Load(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	cmds, err := DecideShip(o, s.now())
	if err != nil {
		return Order{}, err
	}
	if err := s.store.Append(ctx, orderID, len(evs), cmds); err != nil {
		return Order{}, err
	}
	return s.store.Get(ctx, orderID)
}

func (s *Service) Checkout(ctx context.Context, in CheckoutInput) (Order, bool, error) {
	o, created, err := s.checkout(ctx, in)
	_ = s.PublishDue(ctx)
	return o, created, err
}

func (s *Service) checkout(ctx context.Context, in CheckoutInput) (Order, bool, error) {
	if in.BuyerSub == "" || in.IdempotencyKey == "" || in.SiteID == "" {
		return Order{}, false, ErrInvalid
	}
	if existing, err := s.store.GetByIdempotency(ctx, in.BuyerSub, in.IdempotencyKey); err == nil {
		return existing, false, nil
	} else if err != ErrNotFound {
		return Order{}, false, err
	}

	lines, total, err := s.priceLines(ctx, in.Lines)
	if err != nil {
		return Order{}, false, err
	}

	now := s.now()
	orderID := id.New()
	created, err := DecideCreate(Order{}, in, lines, total.Minor, total.Currency, now)
	if err != nil {
		return Order{}, false, err
	}
	if err := s.store.Append(ctx, orderID, 0, created); err != nil {
		if errors.Is(err, ErrConflict) {
			existing, gerr := s.store.GetByIdempotency(ctx, in.BuyerSub, in.IdempotencyKey)
			return existing, false, gerr
		}
		return Order{}, false, err
	}
	o, err := s.store.Get(ctx, orderID)
	if err != nil {
		return Order{}, false, err
	}
	version := 1

	for _, ln := range lines {
		if err := s.stock.Reserve(ctx, in.SiteID, ln.ProductID, o.ID, in.BuyerSub, ln.Qty); err != nil {
			_ = s.stock.ReleaseOrder(ctx, o.ID, in.BuyerSub, ReasonShortage)
			evs, derr := DecideShortage(o, s.now())
			if derr != nil {
				return Order{}, true, derr
			}
			if aerr := s.store.Append(ctx, o.ID, version, evs); aerr != nil {
				return Order{}, true, aerr
			}
			o, _ = s.store.Get(ctx, o.ID)
			if errors.Is(err, ErrShortage) {
				return o, true, ErrShortage
			}
			return o, true, err
		}
	}
	reserved, err := DecideReserveOK(o, s.now())
	if err != nil {
		return Order{}, true, err
	}
	if err := s.store.Append(ctx, o.ID, version, reserved); err != nil {
		return Order{}, true, err
	}
	version++
	o, _ = s.store.Get(ctx, o.ID)

	ch, err := s.pay.Charge(ctx, ChargeRequest{
		IdempotencyKey: "pay:" + in.IdempotencyKey,
		OrderID:        o.ID,
		BuyerSub:       in.BuyerSub,
		Amount:         total,
	})
	if err != nil {
		_ = s.stock.ReleaseOrder(ctx, o.ID, in.BuyerSub, ReasonPayment)
		evs, derr := DecidePaymentFail(o, s.now())
		if derr != nil {
			return Order{}, true, derr
		}
		if aerr := s.store.Append(ctx, o.ID, version, evs); aerr != nil {
			return Order{}, true, aerr
		}
		o, _ = s.store.Get(ctx, o.ID)
		return o, true, err
	}

	if err := s.stock.ConsumeOrder(ctx, o.ID, in.BuyerSub); err != nil {
		_ = s.stock.ReleaseOrder(ctx, o.ID, in.BuyerSub, "consume_failed")
		evs, derr := DecideCancel(o, "consume_failed", s.now())
		if derr != nil {
			return Order{}, true, derr
		}
		if aerr := s.store.Append(ctx, o.ID, version, evs); aerr != nil {
			return Order{}, true, aerr
		}
		o, _ = s.store.Get(ctx, o.ID)
		return o, true, err
	}

	paid, err := DecidePaymentOK(o, ch.ID, s.now())
	if err != nil {
		return Order{}, true, err
	}
	if err := s.store.Append(ctx, o.ID, version, paid); err != nil {
		return Order{}, true, err
	}
	o, err = s.store.Get(ctx, o.ID)
	if err != nil {
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
