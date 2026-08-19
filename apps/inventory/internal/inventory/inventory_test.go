package inventory_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/portfolio/pf-commerce/apps/inventory/internal/inventory"
	"github.com/portfolio/pf-commerce/apps/inventory/internal/store/memory"
	"github.com/portfolio/pf-commerce/packages/clock"
	"github.com/portfolio/pf-commerce/packages/id"
)

func setupInv(t *testing.T) (context.Context, *inventory.Service, string, string, *clock.Fixed) {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	clk := &clock.Fixed{T: time.Date(2026, 8, 19, 4, 0, 0, 0, time.UTC)}
	inv := inventory.NewService(st, 15*time.Minute, clk.Now)
	productID := id.New()
	site, err := inv.CreateSite(ctx, inventory.DefaultSiteCode, "Main warehouse")
	if err != nil {
		t.Fatal(err)
	}
	return ctx, inv, site.ID, productID, clk
}

func TestInboundReserveConsume(t *testing.T) {
	ctx, inv, siteID, productID, _ := setupInv(t)
	if _, err := inv.Inbound(ctx, siteID, productID, "ops", "seed", 5); err != nil {
		t.Fatal(err)
	}
	avail, err := inv.Available(ctx, siteID, productID)
	if err != nil || avail != 5 {
		t.Fatalf("avail %d %v", avail, err)
	}
	if _, err := inv.Reserve(ctx, siteID, productID, id.New(), "buyer", 3); err != nil {
		t.Fatal(err)
	}
	avail, _ = inv.Available(ctx, siteID, productID)
	if avail != 2 {
		t.Fatalf("after reserve %d", avail)
	}
	if _, err := inv.Reserve(ctx, siteID, productID, id.New(), "buyer", 3); err != inventory.ErrShortage {
		t.Fatalf("expected shortage, got %v", err)
	}
	avail, _ = inv.Available(ctx, siteID, productID)
	if avail != 2 {
		t.Fatalf("shortage must not consume remaining: %d", avail)
	}
}

func TestShortageDoesNotLeavePartialReserveOnReleaseOrder(t *testing.T) {
	ctx, inv, siteID, productID, _ := setupInv(t)
	if _, err := inv.Inbound(ctx, siteID, productID, "ops", "seed", 1); err != nil {
		t.Fatal(err)
	}
	orderID := id.New()
	if _, err := inv.Reserve(ctx, siteID, productID, orderID, "a", 1); err != nil {
		t.Fatal(err)
	}
	if err := inv.ReleaseOrder(ctx, orderID, "a", "inventory_shortage"); err != nil {
		t.Fatal(err)
	}
	avail, _ := inv.Available(ctx, siteID, productID)
	if avail != 1 {
		t.Fatalf("released back to %d", avail)
	}
}

func TestConsumeDecrementsOnHand(t *testing.T) {
	ctx, inv, siteID, productID, _ := setupInv(t)
	if _, err := inv.Inbound(ctx, siteID, productID, "ops", "seed", 1); err != nil {
		t.Fatal(err)
	}
	orderID := id.New()
	if _, err := inv.Reserve(ctx, siteID, productID, orderID, "a", 1); err != nil {
		t.Fatal(err)
	}
	if err := inv.ConsumeOrder(ctx, orderID, "a"); err != nil {
		t.Fatal(err)
	}
	avail, _ := inv.Available(ctx, siteID, productID)
	if avail != 0 {
		t.Fatalf("on-hand after consume %d", avail)
	}
	if _, err := inv.Reserve(ctx, siteID, productID, id.New(), "b", 1); err != inventory.ErrShortage {
		t.Fatalf("expected shortage after consume, got %v", err)
	}
}

func TestConcurrentReserveOneUnit(t *testing.T) {
	ctx, inv, siteID, productID, _ := setupInv(t)
	if _, err := inv.Inbound(ctx, siteID, productID, "ops", "seed", 1); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := inv.Reserve(ctx, siteID, productID, id.New(), "buyer", 1)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	var ok, shortage int
	for err := range errs {
		switch err {
		case nil:
			ok++
		case inventory.ErrShortage:
			shortage++
		default:
			t.Fatalf("unexpected %v", err)
		}
	}
	if ok != 1 || shortage != 1 {
		t.Fatalf("ok=%d shortage=%d", ok, shortage)
	}
}

func TestReservationTTLRestoresAvailability(t *testing.T) {
	ctx, inv, siteID, productID, clk := setupInv(t)
	if _, err := inv.Inbound(ctx, siteID, productID, "ops", "seed", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := inv.Reserve(ctx, siteID, productID, id.New(), "a", 1); err != nil {
		t.Fatal(err)
	}
	clk.Advance(16 * time.Minute)
	if err := inv.ExpireDue(ctx); err != nil {
		t.Fatal(err)
	}
	avail, _ := inv.Available(ctx, siteID, productID)
	if avail != 1 {
		t.Fatalf("ttl restore %d", avail)
	}
}
