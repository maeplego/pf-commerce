package boot

import (
	"net/http"
	"time"

	"github.com/portfolio/pf-commerce/apps/order/internal/clients"
	"github.com/portfolio/pf-commerce/apps/order/internal/order"
	"github.com/portfolio/pf-commerce/apps/order/internal/store/memory"
	"github.com/portfolio/pf-commerce/apps/order/internal/web"
	"github.com/portfolio/pf-commerce/packages/auth"
)

func MemoryHandler(now func() time.Time, catalogURL, inventoryURL, paymentURL, notifyURL, siteID string) http.Handler {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	var pay order.Gateway = order.NewMock()
	if paymentURL != "" {
		pay = clients.NewPayment(paymentURL)
	}
	var n order.Notifier
	if notifyURL != "" {
		n = clients.NewNotify(notifyURL)
	}
	svc := order.NewService(memory.New(), clients.NewCatalog(catalogURL), clients.NewStock(inventoryURL), pay, n, now)
	return web.New(svc, siteID, auth.New(true), nil).Routes()
}
