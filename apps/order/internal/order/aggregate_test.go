package order_test

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/portfolio/pf-commerce/apps/order/internal/order"
	"github.com/portfolio/pf-commerce/packages/id"
)

func ev(t EventHelper, typ order.EventType, data any) order.RecordedEvent {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	return order.RecordedEvent{StreamID: t.stream, Type: typ, Time: t.at, Data: raw}
}

type EventHelper struct {
	*testing.T
	stream string
	at     time.Time
}

func TestGivenWhenThenCreateThenPayThenShip(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	h := EventHelper{T: t, stream: id.New(), at: now}
	in := order.CheckoutInput{BuyerSub: "alice", IdempotencyKey: "k1"}
	lines := []order.Line{{ProductID: "p1", SKU: "MUG-1", Name: "Mug", Qty: 1, UnitPriceMinor: 1200, Currency: "JPY"}}
	created, err := order.DecideCreate(order.Order{}, in, lines, 1200, "JPY", now)
	if err != nil {
		t.Fatal(err)
	}
	o, err := order.Fold([]order.RecordedEvent{ev(h, created[0].Type, created[0].Data)})
	if err != nil || o.Status != order.StatusPending {
		t.Fatalf("%+v %v", o, err)
	}
	pay, err := order.DecidePaymentOK(o, "pay-1", now)
	if err != nil {
		t.Fatal(err)
	}
	o, err = order.Apply(o, ev(h, pay[0].Type, pay[0].Data))
	if err != nil || o.Status != order.StatusPaid || o.PaymentID != "pay-1" {
		t.Fatalf("%+v %v", o, err)
	}
	ship, err := order.DecideShip(o, now)
	if err != nil {
		t.Fatal(err)
	}
	o, err = order.Apply(o, ev(h, ship[0].Type, ship[0].Data))
	if err != nil || o.Status != order.StatusShipped {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestDecideRejectsPaidThenPayAgain(t *testing.T) {
	now := time.Date(2026, 8, 19, 6, 0, 0, 0, time.UTC)
	o := order.Order{ID: "o1", Status: order.StatusPaid}
	if _, err := order.DecidePaymentOK(o, "pay-2", now); !errors.Is(err, order.ErrInvalidTransition) {
		t.Fatalf("got %v", err)
	}
	if _, err := order.DecideShip(order.Order{ID: "o1", Status: order.StatusPending}, now); !errors.Is(err, order.ErrInvalidTransition) {
		t.Fatalf("got %v", err)
	}
	if _, err := order.DecideCancel(order.Order{ID: "o1", Status: order.StatusShipped}, "x", now); !errors.Is(err, order.ErrInvalidTransition) {
		t.Fatalf("got %v", err)
	}
}

func TestCheckoutWritesTimelineEvents(t *testing.T) {
	w := newWorld(t, 5)
	o, err := w.checkout("alice", id.New(), 1)
	if err != nil {
		t.Fatal(err)
	}
	evs, err := w.orders.Events(w.ctx, o.ID, "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	var types []order.EventType
	for _, e := range evs {
		types = append(types, e.Type)
	}
	want := []order.EventType{order.EventOrderCreated, order.EventInventoryReserved, order.EventPaymentRecorded}
	if len(types) != len(want) {
		t.Fatalf("types %v", types)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("types %v", types)
		}
	}
	if err := w.orders.Rebuild(w.ctx); err != nil {
		t.Fatal(err)
	}
	again, err := w.orders.Get(w.ctx, o.ID, "alice", false)
	if err != nil || again.Status != order.StatusPaid {
		t.Fatalf("%+v %v", again, err)
	}
}

func TestShipAfterPaid(t *testing.T) {
	w := newWorld(t, 1)
	o, err := w.checkout("alice", id.New(), 1)
	if err != nil {
		t.Fatal(err)
	}
	shipped, err := w.orders.Ship(w.ctx, o.ID, "alice", false)
	if err != nil || shipped.Status != order.StatusShipped {
		t.Fatalf("%+v %v", shipped, err)
	}
}
