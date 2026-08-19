package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/portfolio/pf-commerce/apps/order/internal/order"
)

type CatalogHTTP struct {
	Base   string
	Client *http.Client
}

func NewCatalog(base string) *CatalogHTTP {
	return &CatalogHTTP{Base: strings.TrimRight(base, "/"), Client: &http.Client{Timeout: 8 * time.Second}}
}

func (c *CatalogHTTP) Get(ctx context.Context, productID string) (order.Product, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/v1/products/"+productID, nil)
	if err != nil {
		return order.Product{}, err
	}
	res, err := c.Client.Do(req)
	if err != nil {
		return order.Product{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return order.Product{}, order.ErrNotFound
	}
	if res.StatusCode == http.StatusBadRequest {
		return order.Product{}, order.ErrInvalid
	}
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return order.Product{}, fmt.Errorf("catalog %d: %s", res.StatusCode, b)
	}
	var p struct {
		ID         string `json:"id"`
		SKU        string `json:"sku"`
		Name       string `json:"name"`
		PriceMinor int64  `json:"priceMinor"`
		Currency   string `json:"currency"`
		Active     bool   `json:"active"`
	}
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		return order.Product{}, err
	}
	return order.Product{ID: p.ID, SKU: p.SKU, Name: p.Name, PriceMinor: p.PriceMinor, Currency: p.Currency, Active: p.Active}, nil
}

type StockHTTP struct {
	Base   string
	Client *http.Client
}

func NewStock(base string) *StockHTTP {
	return &StockHTTP{Base: strings.TrimRight(base, "/"), Client: &http.Client{Timeout: 8 * time.Second}}
}

func (s *StockHTTP) Reserve(ctx context.Context, siteID, productID, orderID, actorID string, qty int) error {
	return s.post(ctx, "/v1/reserve", map[string]any{
		"siteId": siteID, "productId": productID, "orderId": orderID, "actorId": actorID, "qty": qty,
	}, http.StatusCreated)
}

func (s *StockHTTP) ReleaseOrder(ctx context.Context, orderID, actorID, reason string) error {
	return s.post(ctx, "/v1/release", map[string]any{"orderId": orderID, "actorId": actorID, "reason": reason}, http.StatusOK)
}

func (s *StockHTTP) ConsumeOrder(ctx context.Context, orderID, actorID string) error {
	return s.post(ctx, "/v1/consume", map[string]any{"orderId": orderID, "actorId": actorID}, http.StatusOK)
}

func (s *StockHTTP) post(ctx context.Context, path string, body any, want int) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusConflict {
		var wrap struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.NewDecoder(res.Body).Decode(&wrap)
		if wrap.Error.Code == "inventory_shortage" {
			return order.ErrShortage
		}
		return fmt.Errorf("inventory conflict")
	}
	if res.StatusCode != want {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("inventory %s %d: %s", path, res.StatusCode, b)
	}
	return nil
}

type PaymentHTTP struct {
	Base   string
	Client *http.Client
}

func NewPayment(base string) *PaymentHTTP {
	return &PaymentHTTP{Base: strings.TrimRight(base, "/"), Client: &http.Client{Timeout: 8 * time.Second}}
}

func (p *PaymentHTTP) Charge(ctx context.Context, req order.ChargeRequest) (order.Charge, error) {
	raw, err := json.Marshal(map[string]any{
		"idempotencyKey": req.IdempotencyKey, "orderId": req.OrderID, "buyerSub": req.BuyerSub,
		"amountMinor": req.Amount.Minor, "currency": req.Amount.Currency,
	})
	if err != nil {
		return order.Charge{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Base+"/v1/charges", bytes.NewReader(raw))
	if err != nil {
		return order.Charge{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	res, err := p.Client.Do(httpReq)
	if err != nil {
		return order.Charge{}, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusConflict {
		return order.Charge{}, order.ErrDeclined
	}
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return order.Charge{}, fmt.Errorf("payment %d: %s", res.StatusCode, b)
	}
	var ch struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&ch); err != nil {
		return order.Charge{}, err
	}
	return order.Charge{ID: ch.ID, IdempotencyKey: req.IdempotencyKey, OrderID: req.OrderID, Amount: req.Amount}, nil
}

type NotifyHTTP struct {
	Base   string
	Client *http.Client
}

func NewNotify(base string) *NotifyHTTP {
	return &NotifyHTTP{Base: strings.TrimRight(base, "/"), Client: &http.Client{Timeout: 8 * time.Second}}
}

func (n *NotifyHTTP) Send(ctx context.Context, mail order.Mail) error {
	raw, err := json.Marshal(map[string]any{
		"id": mail.ID, "type": mail.Type, "orderId": mail.OrderID, "buyerSub": mail.BuyerSub, "payload": mail.Payload,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.Base+"/v1/notifications", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := n.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("notify %d: %s", res.StatusCode, b)
	}
	return nil
}
