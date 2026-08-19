package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/portfolio/pf-commerce/apps/inventory/internal/hub"
	"github.com/portfolio/pf-commerce/apps/inventory/internal/inventory"
	"github.com/portfolio/pf-commerce/apps/inventory/internal/seed"
	"github.com/portfolio/pf-commerce/apps/inventory/internal/store/memory"
	"github.com/portfolio/pf-commerce/apps/inventory/internal/store/postgres"
	"github.com/portfolio/pf-commerce/apps/inventory/internal/web"
	"github.com/portfolio/pf-commerce/packages/clock"
)

func main() {
	port := strings.TrimSpace(os.Getenv("COMMERCE_HTTP_PORT"))
	if port == "" {
		port = "8102"
	}
	dbURL := strings.TrimSpace(os.Getenv("COMMERCE_DATABASE_URL"))
	catalogURL := strings.TrimSpace(os.Getenv("COMMERCE_CATALOG_URL"))
	ctx := context.Background()
	clk := clock.System{}

	var (
		repo   inventory.Repository
		ready  func() error
		closer func()
	)
	if dbURL != "" {
		pg, err := postgres.Open(ctx, dbURL)
		if err != nil {
			log.Fatal(err)
		}
		closer = pg.Close
		repo = pg
		ready = func() error { return pg.Ping(context.Background()) }
		log.Printf("inventory store=postgres")
	} else {
		st := memory.New()
		repo = st
		ready = func() error { return st.Ping(context.Background()) }
		log.Printf("inventory store=memory")
	}

	svc := inventory.NewService(repo, 15*time.Minute, clk.Now)
	site, err := seed.Ensure(ctx, svc, catalogURL)
	if err != nil {
		log.Fatal(err)
	}
	h := hub.New()

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           web.New(svc, h, ready).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("inventory listening on %s site=%s", srv.Addr, site.ID)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("inventory shutting down")
	if closer != nil {
		closer()
	}
}
