package boot

import (
	"context"
	"net/http"
	"time"

	"github.com/portfolio/pf-commerce/apps/inventory/internal/inventory"
	"github.com/portfolio/pf-commerce/apps/inventory/internal/seed"
	"github.com/portfolio/pf-commerce/apps/inventory/internal/store/memory"
	"github.com/portfolio/pf-commerce/apps/inventory/internal/web"
)

func MemoryHandler(now func() time.Time, catalogURL string) (http.Handler, string, error) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	svc := inventory.NewService(memory.New(), 15*time.Minute, now)
	site, err := seed.Ensure(context.Background(), svc, catalogURL)
	if err != nil {
		return nil, "", err
	}
	return web.New(svc, nil, nil).Routes(), site.ID, nil
}
