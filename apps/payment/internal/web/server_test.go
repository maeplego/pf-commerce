package web_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portfolio/pf-commerce/apps/payment/internal/payment"
	"github.com/portfolio/pf-commerce/apps/payment/internal/web"
)

func TestChargeHTTP(t *testing.T) {
	ts := httptest.NewServer(web.New(payment.NewService(payment.NewMemory(), nil), nil).Routes())
	t.Cleanup(ts.Close)
	payload, _ := json.Marshal(map[string]any{
		"idempotencyKey": "pay:k", "orderId": "o1", "buyerSub": "alice", "amountMinor": 1200, "currency": "JPY",
	})
	res, err := http.Post(ts.URL+"/v1/charges", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("%d", res.StatusCode)
	}
	var first struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	res2, err := http.Post(ts.URL+"/v1/charges", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	var second struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res2.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatal("idempotent charge")
	}
}
