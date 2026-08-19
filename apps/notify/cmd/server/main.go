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

	"github.com/portfolio/pf-commerce/apps/notify/internal/notify"
	"github.com/portfolio/pf-commerce/apps/notify/internal/web"
	"github.com/portfolio/pf-commerce/packages/clock"
)

func main() {
	port := strings.TrimSpace(os.Getenv("COMMERCE_HTTP_PORT"))
	if port == "" {
		port = "8105"
	}
	clk := clock.System{}
	svc := notify.NewService(clk.Now)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           web.New(svc, func() error { return svc.Ping(context.Background()) }).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("notify listening on %s (in-process log, no SMTP)", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
}
