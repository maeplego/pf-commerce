package order_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portfolio/pf-commerce/apps/order/internal/order"
	"github.com/portfolio/pf-commerce/apps/order/internal/store/memory"
	"github.com/portfolio/pf-commerce/packages/clock"
	"github.com/portfolio/pf-commerce/packages/id"
)

type fakeCatalog struct {
	p order.Product
}

func (f fakeCatalog) Get(context.Context, string) (order.Product, error) { return f.p, nil }

type fakeStock struct {
	mu    sync.Mutex
	avail int
	held  map[string]int
}

func newFakeStock(n int) *fakeStock {
	return &fakeStock{avail: n, held: map[string]int{}}
}

func (s *fakeStock) Reserve(_ context.Context, _, _, orderID, _ string, qty int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.avail < qty {
		return order.ErrShortage
	}
	s.avail -= qty
	s.held[orderID] += qty
	return nil
}

func (s *fakeStock) ReleaseOrder(_ context.Context, orderID, _, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	qty := s.held[orderID]
	s.avail += qty
	delete(s.held, orderID)
	return nil
}

func (s *fakeStock) ConsumeOrder(_ context.Context, orderID, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.held, orderID)
	return nil
}

type world struct {
	ctx    context.Context
	orders *order.Service
	pay    *order.Mock
	stock  *fakeStock
	siteID string
	mugID  string
}

func newWorld(t *testing.T, stock int) *world {
	t.Helper()
	clk := &clock.Fixed{T: time.Date(2026, 8, 19, 4, 0, 0, 0, time.UTC)}
	mugID := id.New()
	cat := fakeCatalog{p: order.Product{ID: mugID, SKU: "MUG-1", Name: "Mug", PriceMinor: 1200, Currency: "JPY", Active: true}}
	st := newFakeStock(stock)
	pay := order.NewMock()
	svc := order.NewService(memory.New(), cat, st, pay, clk.Now)
	return &world{ctx: context.Background(), orders: svc, pay: pay, stock: st, siteID: id.New(), mugID: mugID}
}

func (w *world) checkout(buyer, key string, qty int) (order.Order, error) {
	o, _, err := w.orders.Checkout(w.ctx, order.CheckoutInput{
		BuyerSub: buyer, IdempotencyKey: key, SiteID: w.siteID,
		Lines: []order.CheckoutLine{{ProductID: w.mugID, Qty: qty}},
	})
	return o, err
}

func TestCheckoutSuccessDecrementsStock(t *testing.T) {
	w := newWorld(t, 5)
	o, err := w.checkout("alice", id.New(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if o.Status != order.StatusPaid || o.Amount.Minor != 1200 || o.PaymentID == "" {
		t.Fatalf("%+v", o)
	}
	if w.stock.avail != 4 {
		t.Fatalf("avail %d", w.stock.avail)
	}
}

func TestCheckoutShortageCompensates(t *testing.T) {
	w := newWorld(t, 1)
	o, err := w.checkout("alice", id.New(), 2)
	if !errors.Is(err, order.ErrShortage) {
		t.Fatalf("got %v", err)
	}
	if o.Status != order.StatusCancelled || o.CancelReason != order.ReasonShortage {
		t.Fatalf("%+v", o)
	}
	if w.stock.avail != 1 {
		t.Fatalf("stock must stay 1 after shortage, got %d", w.stock.avail)
	}
}

func TestConcurrentCheckoutLastUnit(t *testing.T) {
	w := newWorld(t, 1)
	type res struct {
		o   order.Order
		err error
	}
	ch := make(chan res, 2)
	var wg sync.WaitGroup
	for _, buyer := range []string{"alice", "bob"} {
		wg.Add(1)
		go func(b string) {
			defer wg.Done()
			o, err := w.checkout(b, id.New(), 1)
			ch <- res{o, err}
		}(buyer)
	}
	wg.Wait()
	close(ch)
	var paid, short int
	for r := range ch {
		if r.err == nil && r.o.Status == order.StatusPaid {
			paid++
			continue
		}
		if errors.Is(r.err, order.ErrShortage) && r.o.Status == order.StatusCancelled {
			short++
			continue
		}
		t.Fatalf("unexpected %+v %v", r.o, r.err)
	}
	if paid != 1 || short != 1 {
		t.Fatalf("paid=%d short=%d", paid, short)
	}
	if w.stock.avail != 0 {
		t.Fatalf("avail %d", w.stock.avail)
	}
}

func TestCheckoutIdempotent(t *testing.T) {
	w := newWorld(t, 2)
	key := id.New()
	first, err := w.checkout("alice", key, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := w.checkout("alice", key, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatal("idempotency must return same order")
	}
	if w.stock.avail != 1 {
		t.Fatalf("second checkout must not reserve again, avail %d", w.stock.avail)
	}
}

func TestPaymentDeclineReleasesStock(t *testing.T) {
	w := newWorld(t, 1)
	w.pay.FailNext(1)
	o, err := w.checkout("alice", id.New(), 1)
	if !errors.Is(err, order.ErrDeclined) {
		t.Fatalf("got %v", err)
	}
	if o.Status != order.StatusCancelled || o.CancelReason != order.ReasonPayment {
		t.Fatalf("%+v", o)
	}
	if w.stock.avail != 1 {
		t.Fatalf("compensated stock %d", w.stock.avail)
	}
}

func TestBuyerCannotReadOthersOrder(t *testing.T) {
	w := newWorld(t, 1)
	o, err := w.checkout("alice", id.New(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.orders.Get(w.ctx, o.ID, "bob", false); err != order.ErrForbidden {
		t.Fatalf("got %v", err)
	}
	if _, err := w.orders.Get(w.ctx, o.ID, "bob", true); err != nil {
		t.Fatal(err)
	}
}
