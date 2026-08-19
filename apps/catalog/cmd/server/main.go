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

	"github.com/portfolio/pf-commerce/apps/catalog/internal/catalog"
	"github.com/portfolio/pf-commerce/apps/catalog/internal/seed"
	"github.com/portfolio/pf-commerce/apps/catalog/internal/store/memory"
	"github.com/portfolio/pf-commerce/apps/catalog/internal/store/postgres"
	"github.com/portfolio/pf-commerce/apps/catalog/internal/web"
	"github.com/portfolio/pf-commerce/packages/clock"
)

func main() {
	port := strings.TrimSpace(os.Getenv("COMMERCE_HTTP_PORT"))
	if port == "" {
		port = "8101"
	}
	dbURL := strings.TrimSpace(os.Getenv("COMMERCE_DATABASE_URL"))
	ctx := context.Background()
	clk := clock.System{}

	var (
		repo   catalog.Repository
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
		log.Printf("catalog store=postgres")
	} else {
		st := memory.New()
		repo = st
		ready = func() error { return st.Ping(context.Background()) }
		log.Printf("catalog store=memory")
	}

	svc := catalog.NewService(repo, clk.Now)
	if err := seed.Ensure(ctx, svc); err != nil {
		log.Fatal(err)
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           web.New(svc, ready).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("catalog listening on %s", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("catalog shutting down")
	if closer != nil {
		closer()
	}
}
