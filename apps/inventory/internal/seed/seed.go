package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/portfolio/pf-commerce/apps/inventory/internal/inventory"
	"github.com/portfolio/pf-commerce/packages/demoseed"
)

func Ensure(ctx context.Context, inv *inventory.Service, catalogURL string) (inventory.Site, error) {
	site, err := inv.CreateSite(ctx, inventory.DefaultSiteCode, "Main warehouse")
	if err != nil {
		return site, err
	}
	if catalogURL == "" {
		return site, nil
	}
	products, err := waitProducts(ctx, catalogURL)
	if err != nil {
		return site, err
	}
	bySKU := map[string]string{}
	for _, p := range products {
		bySKU[p.SKU] = p.ID
	}
	for _, it := range demoseed.Items {
		pid, ok := bySKU[it.SKU]
		if !ok {
			return site, fmt.Errorf("catalog missing sku %s", it.SKU)
		}
		avail, err := inv.Available(ctx, site.ID, pid)
		if err != nil {
			return site, err
		}
		if it.Stock > 0 && avail == 0 {
			if _, err := inv.Inbound(ctx, site.ID, pid, "seed", "demo", it.Stock); err != nil {
				return site, err
			}
		}
	}
	return site, nil
}

type product struct {
	ID  string `json:"id"`
	SKU string `json:"sku"`
}

func waitProducts(ctx context.Context, catalogURL string) ([]product, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	var last error
	for i := 0; i < 90; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL+"/v1/products", nil)
		if err != nil {
			return nil, err
		}
		res, err := client.Do(req)
		if err != nil {
			last = err
			time.Sleep(time.Second)
			continue
		}
		var body struct {
			Products []product `json:"products"`
		}
		decErr := json.NewDecoder(res.Body).Decode(&body)
		_ = res.Body.Close()
		if decErr != nil || res.StatusCode != 200 || len(body.Products) == 0 {
			last = fmt.Errorf("catalog not ready status=%d", res.StatusCode)
			time.Sleep(time.Second)
			continue
		}
		return body.Products, nil
	}
	if last == nil {
		last = fmt.Errorf("catalog not ready")
	}
	return nil, last
}
