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

	"github.com/portfolio/pf-commerce/apps/payment/internal/payment"
	"github.com/portfolio/pf-commerce/apps/payment/internal/web"
	"github.com/portfolio/pf-commerce/packages/clock"
)

func main() {
	port := strings.TrimSpace(os.Getenv("COMMERCE_HTTP_PORT"))
	if port == "" {
		port = "8104"
	}
	clk := clock.System{}
	st := payment.NewMemory()
	svc := payment.NewService(st, clk.Now)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           web.New(svc, func() error { return st.Ping(context.Background()) }).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("payment listening on %s (mock, no cards)", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}
