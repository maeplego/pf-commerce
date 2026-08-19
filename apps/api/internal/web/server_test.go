package web_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	gwcart "github.com/portfolio/pf-commerce/apps/api/internal/cart"
	gwclients "github.com/portfolio/pf-commerce/apps/api/internal/clients"
	gwmem "github.com/portfolio/pf-commerce/apps/api/internal/store/memory"
	"github.com/portfolio/pf-commerce/apps/api/internal/web"
	catboot "github.com/portfolio/pf-commerce/apps/catalog/boot"
	invboot "github.com/portfolio/pf-commerce/apps/inventory/boot"
	ordboot "github.com/portfolio/pf-commerce/apps/order/boot"
	"github.com/portfolio/pf-commerce/packages/auth"
	"github.com/portfolio/pf-commerce/packages/clock"
	"github.com/portfolio/pf-commerce/packages/id"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	clk := &clock.Fixed{T: time.Date(2026, 8, 19, 4, 0, 0, 0, time.UTC)}

	catH, err := catboot.MemoryHandler(clk.Now)
	if err != nil {
		t.Fatal(err)
	}
	catHTTP := httptest.NewServer(catH)

	invH, siteID, err := invboot.MemoryHandler(clk.Now, catHTTP.URL)
	if err != nil {
		t.Fatal(err)
	}
	invHTTP := httptest.NewServer(invH)

	ordHTTP := httptest.NewServer(ordboot.MemoryHandler(clk.Now, catHTTP.URL, invHTTP.URL, siteID))

	be := gwclients.New(catHTTP.URL, invHTTP.URL, ordHTTP.URL)
	gw := web.New(be, gwcart.NewService(gwmem.New(), clk.Now), siteID, "", auth.New(true), nil)
	ts := httptest.NewServer(gw.Routes())
	t.Cleanup(func() {
		ts.Close()
		ordHTTP.Close()
		invHTTP.Close()
		catHTTP.Close()
	})
	return ts
}

func doJSON(t *testing.T, method, url string, body any, sub, role string) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if sub != "" {
		req.Header.Set("X-Dev-User-Sub", sub)
	}
	if role != "" {
		req.Header.Set("X-Dev-Role", role)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func decode(t *testing.T, res *http.Response, dest any) {
	t.Helper()
	defer res.Body.Close()
	if err := json.NewDecoder(res.Body).Decode(dest); err != nil {
		t.Fatal(err)
	}
}

func mugID(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	res, err := http.Get(ts.URL + "/v1/products")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Products []struct {
			ID  string `json:"id"`
			SKU string `json:"sku"`
		} `json:"products"`
	}
	decode(t, res, &body)
	for _, p := range body.Products {
		if p.SKU == "MUG-1" {
			return p.ID
		}
	}
	t.Fatal("MUG-1 missing")
	return ""
}

func TestHealthReady(t *testing.T) {
	ts := testServer(t)
	for _, path := range []string{"/health", "/ready"} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != 200 {
			t.Fatalf("%s %d", path, res.StatusCode)
		}
		_ = res.Body.Close()
	}
}

func TestListProductsPublic(t *testing.T) {
	ts := testServer(t)
	res, err := http.Get(ts.URL + "/v1/products")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("%d", res.StatusCode)
	}
	var body struct {
		Products []struct {
			SKU          string `json:"sku"`
			PriceMinor   int64  `json:"priceMinor"`
			AvailableQty int    `json:"availableQty"`
		} `json:"products"`
	}
	decode(t, res, &body)
	if len(body.Products) != 3 {
		t.Fatalf("%d products", len(body.Products))
	}
	var mugQty int
	for _, p := range body.Products {
		if p.PriceMinor < 0 {
			t.Fatal("negative money")
		}
		if p.SKU == "MUG-1" {
			mugQty = p.AvailableQty
		}
	}
	if mugQty != 1 {
		t.Fatalf("mug qty %d", mugQty)
	}
}

func TestCheckoutUnauthorized(t *testing.T) {
	ts := testServer(t)
	res := doJSON(t, http.MethodPost, ts.URL+"/v1/checkout", map[string]any{"idempotencyKey": id.New()}, "", "")
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("%d", res.StatusCode)
	}
}

func TestCheckoutPaidAndIdempotent(t *testing.T) {
	ts := testServer(t)
	pid := mugID(t, ts)
	key := id.New()
	body := map[string]any{
		"idempotencyKey": key,
		"lines":          []map[string]any{{"productId": pid, "qty": 1}},
	}
	res := doJSON(t, http.MethodPost, ts.URL+"/v1/checkout", body, "alice", "buyer")
	if res.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("first %d %s", res.StatusCode, b)
	}
	var o struct {
		ID           string `json:"id"`
		Status       string `json:"status"`
		AmountMinor  int64  `json:"amountMinor"`
		CancelReason string `json:"cancelReason"`
	}
	decode(t, res, &o)
	if o.Status != "paid" || o.AmountMinor != 1200 {
		t.Fatalf("%+v", o)
	}
	res2 := doJSON(t, http.MethodPost, ts.URL+"/v1/checkout", body, "alice", "buyer")
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("replay %d", res2.StatusCode)
	}
	var o2 struct {
		ID string `json:"id"`
	}
	decode(t, res2, &o2)
	if o2.ID != o.ID {
		t.Fatal("idempotency changed id")
	}
}

func TestCheckoutShortageHTTP(t *testing.T) {
	ts := testServer(t)
	pid := mugID(t, ts)
	res := doJSON(t, http.MethodPost, ts.URL+"/v1/checkout", map[string]any{
		"idempotencyKey": id.New(),
		"lines":          []map[string]any{{"productId": pid, "qty": 2}},
	}, "alice", "buyer")
	if res.StatusCode != http.StatusConflict {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("%d %s", res.StatusCode, b)
	}
	var body struct {
		Order struct {
			Status       string `json:"status"`
			CancelReason string `json:"cancelReason"`
		} `json:"order"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decode(t, res, &body)
	if body.Error.Code != "inventory_shortage" || body.Order.Status != "cancelled" {
		t.Fatalf("%+v", body)
	}
	res2, err := http.Get(ts.URL + "/v1/products/" + pid)
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		AvailableQty int `json:"availableQty"`
	}
	decode(t, res2, &p)
	if p.AvailableQty != 1 {
		t.Fatalf("compensated qty %d", p.AvailableQty)
	}
}

func TestConcurrentCheckoutHTTP(t *testing.T) {
	ts := testServer(t)
	pid := mugID(t, ts)
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for _, user := range []string{"alice", "bob"} {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			res := doJSON(t, http.MethodPost, ts.URL+"/v1/checkout", map[string]any{
				"idempotencyKey": id.New(),
				"lines":          []map[string]any{{"productId": pid, "qty": 1}},
			}, u, "buyer")
			codes <- res.StatusCode
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
		}(user)
	}
	wg.Wait()
	close(codes)
	var created, conflict int
	for c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("status %d", c)
		}
	}
	if created != 1 || conflict != 1 {
		t.Fatalf("created=%d conflict=%d", created, conflict)
	}
}

func TestOpsInboundForbiddenToBuyer(t *testing.T) {
	ts := testServer(t)
	res := doJSON(t, http.MethodPost, ts.URL+"/v1/ops/stock-inbound", map[string]any{
		"productId": mugID(t, ts), "qty": 1,
	}, "alice", "buyer")
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("%d", res.StatusCode)
	}
}

func TestRejectFloatPriceOnOpsCreate(t *testing.T) {
	ts := testServer(t)
	raw := bytes.NewBufferString(`{"sku":"BAD-1","name":"Bad","priceMinor":12.5,"currency":"JPY"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/ops/products", raw)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dev-User-Sub", "ops-1")
	req.Header.Set("X-Dev-Role", "ops")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("float money must be 400, got %d", res.StatusCode)
	}
}

func TestOtherBuyerForbidden(t *testing.T) {
	ts := testServer(t)
	pid := mugID(t, ts)
	res := doJSON(t, http.MethodPost, ts.URL+"/v1/checkout", map[string]any{
		"idempotencyKey": id.New(),
		"lines":          []map[string]any{{"productId": pid, "qty": 1}},
	}, "alice", "buyer")
	var o struct {
		ID string `json:"id"`
	}
	decode(t, res, &o)
	res2 := doJSON(t, http.MethodGet, ts.URL+"/v1/orders/"+o.ID, nil, "bob", "buyer")
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusForbidden {
		t.Fatalf("%d", res2.StatusCode)
	}
}

func TestCheckoutFromCart(t *testing.T) {
	ts := testServer(t)
	pid := mugID(t, ts)
	res := doJSON(t, http.MethodPost, ts.URL+"/v1/cart/items", map[string]any{"productId": pid, "qty": 1}, "alice", "buyer")
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("cart %d %s", res.StatusCode, b)
	}
	_ = res.Body.Close()
	res2 := doJSON(t, http.MethodPost, ts.URL+"/v1/checkout", map[string]any{"idempotencyKey": id.New()}, "alice", "buyer")
	if res2.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(res2.Body)
		t.Fatalf("checkout %d %s", res2.StatusCode, b)
	}
	var o struct {
		Status      string `json:"status"`
		AmountMinor int64  `json:"amountMinor"`
	}
	decode(t, res2, &o)
	if o.Status != "paid" || o.AmountMinor != 1200 {
		t.Fatalf("%+v", o)
	}
	res3 := doJSON(t, http.MethodGet, ts.URL+"/v1/cart", nil, "alice", "buyer")
	var c struct {
		Items []any `json:"items"`
	}
	decode(t, res3, &c)
	if len(c.Items) != 0 {
		t.Fatalf("cart should clear: %+v", c)
	}
}
