package order_test

import (
	"context"
	"testing"
	"time"

	"github.com/portfolio/pf-commerce/apps/order/internal/order"
	"github.com/portfolio/pf-commerce/apps/order/internal/store/memory"
	"github.com/portfolio/pf-commerce/packages/money"
)

func TestExportLinesUsesOpaqueBuyer(t *testing.T) {
	st := memory.New()
	svc := order.NewService(st, nil, nil, nil, nil, time.Now)
	ctx := context.Background()
	day := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	o := order.Order{
		ID: "o1", BuyerSub: "alice@example.test", Status: order.StatusPaid,
		Amount: money.Amount{Minor: 1200, Currency: "JPY"}, CreatedAt: day, UpdatedAt: day,
		Lines: []order.Line{{ProductID: "p1", SKU: "MUG-1", Qty: 1, UnitPriceMinor: 1200, Currency: "JPY"}},
	}
	if err := st.Create(ctx, o); err != nil {
		t.Fatal(err)
	}
	lines, err := svc.ExportLines(ctx, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].BuyerOpaque == "" || lines[0].BuyerOpaque == "alice@example.test" {
		t.Fatalf("%+v", lines)
	}
}
