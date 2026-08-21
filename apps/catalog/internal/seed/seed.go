package seed

import (
	"context"

	"github.com/portfolio/pf-commerce/apps/catalog/internal/catalog"
	"github.com/portfolio/pf-commerce/packages/demoseed"
)

func Ensure(ctx context.Context, cat *catalog.Service) error {
	for _, it := range demoseed.Items {
		_, err := cat.GetBySKU(ctx, it.SKU)
		if err == catalog.ErrNotFound {
			_, err = cat.Create(ctx, catalog.CreateInput{
				OrgID: "org-demo-a", SKU: it.SKU, Name: it.Name, Description: it.Description,
				PriceMinor: it.PriceMinor, Currency: it.Currency, ImageURL: it.ImageURL,
			})
		}
		if err != nil {
			return err
		}
	}
	mug, err := cat.GetBySKU(ctx, "MUG-1")
	if err != nil {
		return err
	}
	existing, err := cat.ListReviews(ctx, []string{mug.ID})
	if err != nil {
		return err
	}
	if len(existing) == 0 {
		if _, err := cat.AddReview(ctx, mug.ID, "demo-buyer", "Holds coffee. Last-unit demo SKU."); err != nil {
			return err
		}
		if _, err := cat.AddReview(ctx, mug.ID, "ops-note", "Keep stock at 1 for the concurrent checkout demo."); err != nil {
			return err
		}
	}
	return nil
}
