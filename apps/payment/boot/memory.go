package boot

import (
	"net/http"

	"github.com/portfolio/pf-commerce/apps/payment/internal/payment"
	"github.com/portfolio/pf-commerce/apps/payment/internal/web"
)

func MemoryHandler() http.Handler {
	return web.New(payment.NewService(payment.NewMemory(), nil), nil).Routes()
}
