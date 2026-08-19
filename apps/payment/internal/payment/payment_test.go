package payment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/portfolio/pf-commerce/apps/payment/internal/payment"
	"github.com/portfolio/pf-commerce/packages/money"
)

func TestChargeIdempotentAndNoCardFields(t *testing.T) {
	svc := payment.NewService(payment.NewMemory(), func() time.Time {
		return time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	})
	amt, err := money.New(1200, "JPY")
	if err != nil {
		t.Fatal(err)
	}
	req := payment.ChargeRequest{IdempotencyKey: "pay:k1", OrderID: "ord-1", BuyerSub: "alice", Amount: amt}
	first, err := svc.Charge(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Charge(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatal("same idempotency key must not create a second charge")
	}
}

func TestChargeDecline(t *testing.T) {
	svc := payment.NewService(payment.NewMemory(), nil)
	svc.FailNext(1)
	amt, _ := money.New(300, "JPY")
	_, err := svc.Charge(context.Background(), payment.ChargeRequest{
		IdempotencyKey: "pay:fail", OrderID: "ord-2", BuyerSub: "alice", Amount: amt,
	})
	if !errors.Is(err, payment.ErrDeclined) {
		t.Fatalf("got %v", err)
	}
	ok, err := svc.Charge(context.Background(), payment.ChargeRequest{
		IdempotencyKey: "pay:ok", OrderID: "ord-3", BuyerSub: "alice", Amount: amt,
	})
	if err != nil || ok.ID == "" {
		t.Fatalf("%+v %v", ok, err)
	}
}

func TestChargeRejectsEmptyKey(t *testing.T) {
	svc := payment.NewService(nil, nil)
	_, err := svc.Charge(context.Background(), payment.ChargeRequest{OrderID: "x"})
	if !errors.Is(err, payment.ErrInvalid) {
		t.Fatalf("got %v", err)
	}
}
