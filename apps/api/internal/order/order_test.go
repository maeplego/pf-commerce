package order_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portfolio/pf-commerce/api/internal/cart"
	"github.com/portfolio/pf-commerce/api/internal/catalog"
	"github.com/portfolio/pf-commerce/api/internal/clock"
	"github.com/portfolio/pf-commerce/api/internal/id"
	"github.com/portfolio/pf-commerce/api/internal/inventory"
	"github.com/portfolio/pf-commerce/api/internal/order"
	"github.com/portfolio/pf-commerce/api/internal/payment"
	"github.com/portfolio/pf-commerce/api/internal/store/memory"
)

type world struct {
	ctx    context.Context
	cat    *catalog.Service
	inv    *inventory.Service
	carts  *cart.Service
	orders *order.Service
	pay    *payment.Mock
	siteID string
	mugID  string
	clk    *clock.Fixed
}

func newWorld(t *testing.T, stock int) *world {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	clk := &clock.Fixed{T: time.Date(2026, 8, 19, 4, 0, 0, 0, time.UTC)}
	cat := catalog.NewService(memory.Catalog{Store: st}, clk.Now)
	inv := inventory.NewService(memory.Inventory{Store: st}, 15*time.Minute, clk.Now)
	carts := cart.NewService(memory.Cart{Store: st}, clk.Now)
	pay := payment.NewMock()
	orders := order.NewService(memory.Orders{Store: st}, cat, inv, carts, pay, clk.Now)
	p, err := cat.Create(ctx, catalog.CreateInput{SKU: "MUG-1", Name: "Mug", PriceMinor: 1200, Currency: "JPY"})
	if err != nil {
		t.Fatal(err)
	}
	site, err := inv.CreateSite(ctx, inventory.DefaultSiteCode, "Main")
	if err != nil {
		t.Fatal(err)
	}
	if stock > 0 {
		if _, err := inv.Inbound(ctx, site.ID, p.ID, "ops", "seed", stock); err != nil {
			t.Fatal(err)
		}
	}
	return &world{ctx: ctx, cat: cat, inv: inv, carts: carts, orders: orders, pay: pay, siteID: site.ID, mugID: p.ID, clk: clk}
}

func (w *world) checkout(buyer, key string, qty int) (order.Order, error) {
	o, _, err := w.orders.Checkout(w.ctx, order.CheckoutInput{
		BuyerSub:       buyer,
		IdempotencyKey: key,
		SiteID:         w.siteID,
		Lines:          []order.CheckoutLine{{ProductID: w.mugID, Qty: qty}},
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
	avail, _ := w.inv.Available(w.ctx, w.siteID, w.mugID)
	if avail != 4 {
		t.Fatalf("avail %d", avail)
	}
}

func TestCheckoutShortageCompensates(t *testing.T) {
	w := newWorld(t, 1)
	o, err := w.checkout("alice", id.New(), 2)
	if !errors.Is(err, inventory.ErrShortage) {
		t.Fatalf("got %v", err)
	}
	if o.Status != order.StatusCancelled || o.CancelReason != order.ReasonShortage {
		t.Fatalf("%+v", o)
	}
	avail, _ := w.inv.Available(w.ctx, w.siteID, w.mugID)
	if avail != 1 {
		t.Fatalf("stock must stay 1 after shortage, got %d", avail)
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
		if errors.Is(r.err, inventory.ErrShortage) && r.o.Status == order.StatusCancelled {
			short++
			continue
		}
		t.Fatalf("unexpected %+v %v", r.o, r.err)
	}
	if paid != 1 || short != 1 {
		t.Fatalf("paid=%d short=%d", paid, short)
	}
	avail, _ := w.inv.Available(w.ctx, w.siteID, w.mugID)
	if avail != 0 {
		t.Fatalf("avail %d", avail)
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
	avail, _ := w.inv.Available(w.ctx, w.siteID, w.mugID)
	if avail != 1 {
		t.Fatalf("second checkout must not reserve again, avail %d", avail)
	}
}

func TestPaymentDeclineReleasesStock(t *testing.T) {
	w := newWorld(t, 1)
	w.pay.FailNext(1)
	o, err := w.checkout("alice", id.New(), 1)
	if !errors.Is(err, payment.ErrDeclined) {
		t.Fatalf("got %v", err)
	}
	if o.Status != order.StatusCancelled || o.CancelReason != order.ReasonPayment {
		t.Fatalf("%+v", o)
	}
	avail, _ := w.inv.Available(w.ctx, w.siteID, w.mugID)
	if avail != 1 {
		t.Fatalf("compensated stock %d", avail)
	}
}

func TestCheckoutFromCart(t *testing.T) {
	w := newWorld(t, 3)
	if _, err := w.carts.Add(w.ctx, "alice", w.mugID, 2); err != nil {
		t.Fatal(err)
	}
	o, created, err := w.orders.Checkout(w.ctx, order.CheckoutInput{
		BuyerSub: "alice", IdempotencyKey: id.New(), SiteID: w.siteID, UseCart: true,
	})
	if !created {
		t.Fatal("expected new order")
	}
	if err != nil {
		t.Fatal(err)
	}
	if o.Amount.Minor != 2400 || len(o.Lines) != 1 || o.Lines[0].Qty != 2 {
		t.Fatalf("%+v", o)
	}
	c, _ := w.carts.Get(w.ctx, "alice")
	if len(c.Items) != 0 {
		t.Fatalf("cart should clear: %+v", c)
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
