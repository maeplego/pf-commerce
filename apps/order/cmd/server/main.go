package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/portfolio/pf-commerce/apps/order/internal/clients"
	"github.com/portfolio/pf-commerce/apps/order/internal/order"
	"github.com/portfolio/pf-commerce/apps/order/internal/store/memory"
	"github.com/portfolio/pf-commerce/apps/order/internal/store/postgres"
	"github.com/portfolio/pf-commerce/apps/order/internal/web"
	"github.com/portfolio/pf-commerce/packages/auth"
	"github.com/portfolio/pf-commerce/packages/clock"
	"github.com/portfolio/pf-commerce/packages/envprofile"
)

func main() {
	port := strings.TrimSpace(os.Getenv("COMMERCE_HTTP_PORT"))
	if port == "" {
		port = "8103"
	}
	dbURL := strings.TrimSpace(os.Getenv("COMMERCE_DATABASE_URL"))
	catalogURL := strings.TrimSpace(os.Getenv("COMMERCE_CATALOG_URL"))
	inventoryURL := strings.TrimSpace(os.Getenv("COMMERCE_INVENTORY_URL"))
	paymentURL := strings.TrimSpace(os.Getenv("COMMERCE_PAYMENT_URL"))
	notifyURL := strings.TrimSpace(os.Getenv("COMMERCE_NOTIFY_URL"))
	if catalogURL == "" || inventoryURL == "" {
		log.Fatal("COMMERCE_CATALOG_URL and COMMERCE_INVENTORY_URL are required")
	}
	devAuth := strings.EqualFold(os.Getenv("COMMERCE_DEV_AUTH"), "true") || os.Getenv("COMMERCE_DEV_AUTH") == "1"
	issuer := strings.TrimSpace(os.Getenv("OIDC_ISSUER"))
	internalBase := strings.TrimSpace(os.Getenv("OIDC_INTERNAL_BASE"))
	if internalBase == "" {
		internalBase = issuer
	}
	if !devAuth && issuer == "" {
		log.Fatal("set COMMERCE_DEV_AUTH=true or configure OIDC_ISSUER")
	}
	env := envprofile.Normalize(os.Getenv("COMMERCE_ENV"))
	if err := envprofile.ValidateCommercial(env, devAuth, issuer, "COMMERCE_ENV", "COMMERCE_DEV_AUTH"); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	clk := clock.System{}

	var (
		repo   order.Persistence
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
		log.Printf("order store=postgres")
	} else {
		st := memory.New()
		repo = st
		ready = func() error { return st.Ping(context.Background()) }
		log.Printf("order store=memory")
	}

	siteID, err := waitSite(ctx, inventoryURL)
	if err != nil {
		log.Fatal(err)
	}

	var pay order.Gateway = order.NewMock()
	if paymentURL != "" {
		pay = clients.NewPayment(paymentURL)
		log.Printf("order payment=http %s", paymentURL)
	} else {
		log.Printf("order payment=in-process mock")
	}
	var n order.Notifier
	if notifyURL != "" {
		n = clients.NewNotify(notifyURL)
	}

	svc := order.NewService(repo, clients.NewCatalog(catalogURL), clients.NewStock(inventoryURL), pay, n, clk.Now)
	mw := auth.New(devAuth, issuer, internalBase)
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           web.New(svc, siteID, mw, ready).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("order listening on %s site=%s (payment mock, no cards)", srv.Addr, siteID)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for range t.C {
			_ = svc.PublishDue(context.Background())
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("order shutting down")
	if closer != nil {
		closer()
	}
}

func waitSite(ctx context.Context, inventoryURL string) (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	url := strings.TrimRight(inventoryURL, "/") + "/v1/sites/code/MAIN"
	var last error
	for i := 0; i < 40; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}
		res, err := client.Do(req)
		if err != nil {
			last = err
			time.Sleep(250 * time.Millisecond)
			continue
		}
		var body struct {
			ID string `json:"id"`
		}
		decErr := json.NewDecoder(res.Body).Decode(&body)
		_ = res.Body.Close()
		if decErr == nil && res.StatusCode == 200 && body.ID != "" {
			return body.ID, nil
		}
		last = fmt.Errorf("inventory site status=%d", res.StatusCode)
		time.Sleep(250 * time.Millisecond)
	}
	if last == nil {
		last = fmt.Errorf("inventory site not ready")
	}
	return "", last
}
