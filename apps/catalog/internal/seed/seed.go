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
				SKU: it.SKU, Name: it.Name, Description: it.Description,
				PriceMinor: it.PriceMinor, Currency: it.Currency, ImageURL: it.ImageURL,
			})
		}
		if err != nil {
			return err
		}
	}
	return nil
}
