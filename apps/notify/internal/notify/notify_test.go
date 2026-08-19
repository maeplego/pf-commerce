package notify_test

import (
	"context"
	"testing"
	"time"

	"github.com/portfolio/pf-commerce/apps/notify/internal/notify"
)

func TestDeliverIdempotent(t *testing.T) {
	svc := notify.NewService(func() time.Time { return time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC) })
	msg := notify.Message{ID: "obx-1", Type: "OrderPaid", OrderID: "ord-1", BuyerSub: "alice", Payload: `{"status":"paid"}`}
	first, created, err := svc.Deliver(context.Background(), msg)
	if err != nil || !created {
		t.Fatalf("%+v %v %v", first, created, err)
	}
	second, created, err := svc.Deliver(context.Background(), msg)
	if err != nil || created {
		t.Fatalf("duplicate must not create: %v %v", created, err)
	}
	if first.ID != second.ID {
		t.Fatal("same id")
	}
	if len(svc.List(context.Background())) != 1 {
		t.Fatal("one stored mail")
	}
}
