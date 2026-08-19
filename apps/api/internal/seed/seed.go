package seed

import (
	"context"

	"github.com/portfolio/pf-commerce/api/internal/catalog"
	"github.com/portfolio/pf-commerce/api/internal/inventory"
)

func Ensure(ctx context.Context, cat *catalog.Service, inv *inventory.Service) (site inventory.Site, err error) {
	site, err = inv.CreateSite(ctx, inventory.DefaultSiteCode, "Main warehouse")
	if err != nil {
		return site, err
	}
	type item struct {
		in    catalog.CreateInput
		stock int
	}
	items := []item{
		{catalog.CreateInput{
			SKU: "MUG-1", Name: "Demo Mug", Description: "Last-unit demo. Stock starts at 1.",
			PriceMinor: 1200, Currency: "JPY", ImageURL: "https://placehold.co/400x400?text=Mug",
		}, 1},
		{catalog.CreateInput{
			SKU: "TEE-1", Name: "Demo Tee", Description: "Plenty in stock.",
			PriceMinor: 3500, Currency: "JPY", ImageURL: "https://placehold.co/400x400?text=Tee",
		}, 20},
		{catalog.CreateInput{
			SKU: "STK-1", Name: "Demo Sticker", Description: "Already sold out.",
			PriceMinor: 300, Currency: "JPY", ImageURL: "https://placehold.co/400x400?text=Sticker",
		}, 0},
	}
	for _, it := range items {
		existing, err := cat.GetBySKU(ctx, it.in.SKU)
		if err == catalog.ErrNotFound {
			existing, err = cat.Create(ctx, it.in)
		}
		if err != nil {
			return site, err
		}
		avail, err := inv.Available(ctx, site.ID, existing.ID)
		if err != nil {
			return site, err
		}
		if it.stock > 0 && avail == 0 {
			if _, err := inv.Inbound(ctx, site.ID, existing.ID, "seed", "demo", it.stock); err != nil {
				return site, err
			}
		}
	}
	return site, nil
}
