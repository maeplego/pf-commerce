package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/portfolio/pf-commerce/api/internal/auth"
	"github.com/portfolio/pf-commerce/api/internal/cart"
	"github.com/portfolio/pf-commerce/api/internal/catalog"
	"github.com/portfolio/pf-commerce/api/internal/clock"
	"github.com/portfolio/pf-commerce/api/internal/config"
	"github.com/portfolio/pf-commerce/api/internal/inventory"
	"github.com/portfolio/pf-commerce/api/internal/order"
	"github.com/portfolio/pf-commerce/api/internal/payment"
	"github.com/portfolio/pf-commerce/api/internal/seed"
	"github.com/portfolio/pf-commerce/api/internal/store/memory"
	"github.com/portfolio/pf-commerce/api/internal/store/postgres"
	"github.com/portfolio/pf-commerce/api/internal/web"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	clk := clock.System{}

	var (
		catRepo  catalog.Repository
		invRepo  inventory.Repository
		cartRepo cart.Repository
		ordRepo  order.Repository
		ready    func() error
		closer   func()
	)

	if cfg.DatabaseURL != "" {
		pg, err := postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatal(err)
		}
		closer = pg.Close
		catRepo = postgres.Catalog{Store: pg}
		invRepo = postgres.Inventory{Store: pg}
		cartRepo = postgres.Cart{Store: pg}
		ordRepo = postgres.Orders{Store: pg}
		ready = func() error { return pg.Ping(context.Background()) }
		log.Printf("store=postgres")
	} else {
		st := memory.New()
		catRepo = memory.Catalog{Store: st}
		invRepo = memory.Inventory{Store: st}
		cartRepo = memory.Cart{Store: st}
		ordRepo = memory.Orders{Store: st}
		ready = func() error { return st.Ping(context.Background()) }
		log.Printf("store=memory (set COMMERCE_DATABASE_URL for Postgres)")
	}

	cat := catalog.NewService(catRepo, clk.Now)
	inv := inventory.NewService(invRepo, cfg.ReservationTTL, clk.Now)
	carts := cart.NewService(cartRepo, clk.Now)
	orders := order.NewService(ordRepo, cat, inv, carts, payment.NewMock(), clk.Now)
	site, err := seed.Ensure(ctx, cat, inv)
	if err != nil {
		log.Fatal(err)
	}

	mw := auth.New(cfg.DevAuth)
	handler := web.New(cat, inv, carts, orders, site.ID, cfg.CORSOrigin, mw, ready).Routes()
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("commerce api listening on %s (devAuth=%v site=%s)", cfg.HTTPAddr, cfg.DevAuth, site.ID)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down")
	if closer != nil {
		closer()
	}
}
