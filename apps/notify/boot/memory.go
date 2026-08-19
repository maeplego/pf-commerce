package boot

import (
	"context"
	"net/http"

	"github.com/portfolio/pf-commerce/apps/notify/internal/notify"
	"github.com/portfolio/pf-commerce/apps/notify/internal/web"
)

func MemoryHandler() http.Handler {
	svc := notify.NewService(nil)
	return web.New(svc, func() error { return svc.Ping(context.Background()) }).Routes()
}
