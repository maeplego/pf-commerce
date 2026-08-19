package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/portfolio/pf-commerce/apps/api/internal/cart"
	"github.com/portfolio/pf-commerce/apps/api/internal/clients"
	"github.com/portfolio/pf-commerce/apps/api/internal/config"
	"github.com/portfolio/pf-commerce/apps/api/internal/store/memory"
	"github.com/portfolio/pf-commerce/apps/api/internal/store/postgres"
	"github.com/portfolio/pf-commerce/apps/api/internal/web"
	"github.com/portfolio/pf-commerce/packages/auth"
	"github.com/portfolio/pf-commerce/packages/clock"
)

func main() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	clk := clock.System{}
	be := clients.New(cfg.CatalogURL, cfg.InventoryURL, cfg.OrderURL, cfg.NotifyURL)

	var (
		carts  cart.Repository
		ready  func() error
		closer func()
	)
	if cfg.DatabaseURL != "" {
		pg, err := postgres.Open(ctx, cfg.DatabaseURL)
		if err != nil {
			log.Fatal(err)
		}
		closer = pg.Close
		carts = pg
		ready = func() error {
			if err := pg.Ping(context.Background()); err != nil {
				return err
			}
			return be.Ping(context.Background())
		}
		log.Printf("gateway cart store=postgres")
	} else {
		st := memory.New()
		carts = st
		ready = func() error {
			if err := st.Ping(context.Background()); err != nil {
				return err
			}
			return be.Ping(context.Background())
		}
		log.Printf("gateway cart store=memory")
	}

	siteID, err := waitSite(ctx, be)
	if err != nil {
		log.Fatal(err)
	}

	mw := auth.New(cfg.DevAuth)
	handler := web.New(be, cart.NewService(carts, clk.Now), siteID, cfg.CORSOrigin, mw, ready).Routes()
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("commerce gateway listening on %s site=%s", cfg.HTTPAddr, siteID)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("gateway shutting down")
	if closer != nil {
		closer()
	}
}

func waitSite(ctx context.Context, be *clients.HTTP) (string, error) {
	var last error
	for i := 0; i < 40; i++ {
		id, err := be.SiteID(ctx)
		if err == nil && id != "" {
			return id, nil
		}
		last = err
		time.Sleep(250 * time.Millisecond)
	}
	return "", last
}
