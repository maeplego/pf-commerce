package boot

import (
	"context"
	"net/http"
	"time"

	"github.com/portfolio/pf-commerce/apps/catalog/internal/catalog"
	"github.com/portfolio/pf-commerce/apps/catalog/internal/seed"
	"github.com/portfolio/pf-commerce/apps/catalog/internal/store/memory"
	"github.com/portfolio/pf-commerce/apps/catalog/internal/web"
)

func MemoryHandler(now func() time.Time) (http.Handler, error) {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	svc := catalog.NewService(memory.New(), now)
	if err := seed.Ensure(context.Background(), svc); err != nil {
		return nil, err
	}
	return web.New(svc, nil).Routes(), nil
}
