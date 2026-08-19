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
)

type Product struct {
	ID          string `json:"id"`
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PriceMinor  int64  `json:"priceMinor"`
	Currency    string `json:"currency"`
	ImageURL    string `json:"imageUrl"`
	Active      bool   `json:"active"`
}

type HTTP struct {
	Catalog   string
	Inventory string
	Order     string
	Client    *http.Client
}

func New(catalog, inventory, orderURL string) *HTTP {
	return &HTTP{
		Catalog:   strings.TrimRight(catalog, "/"),
		Inventory: strings.TrimRight(inventory, "/"),
		Order:     strings.TrimRight(orderURL, "/"),
		Client:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *HTTP) Ping(ctx context.Context) error {
	for _, u := range []string{h.Catalog, h.Inventory, h.Order} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u+"/ready", nil)
		if err != nil {
			return err
		}
		res, err := h.Client.Do(req)
		if err != nil {
			return err
		}
		_ = res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("%s not ready", u)
		}
	}
	return nil
}

func (h *HTTP) ListProducts(ctx context.Context) ([]Product, error) {
	var body struct {
		Products []Product `json:"products"`
	}
	if err := h.getJSON(ctx, h.Catalog+"/v1/products", &body); err != nil {
		return nil, err
	}
	return body.Products, nil
}

func (h *HTTP) GetProduct(ctx context.Context, id string) (Product, int, error) {
	var p Product
	st, err := h.getJSONStatus(ctx, h.Catalog+"/v1/products/"+id, &p)
	return p, st, err
}

func (h *HTTP) CreateProduct(ctx context.Context, raw []byte) (Product, int, []byte, error) {
	return doJSON[Product](ctx, h.Client, http.MethodPost, h.Catalog+"/v1/products", raw, nil)
}

func (h *HTTP) Available(ctx context.Context, siteID string, ids []string) (map[string]int, error) {
	if len(ids) == 0 {
		return map[string]int{}, nil
	}
	q := h.Inventory + "/v1/available?siteId=" + siteID + "&productIds=" + strings.Join(ids, ",")
	var body struct {
		Available map[string]int `json:"available"`
	}
	if err := h.getJSON(ctx, q, &body); err != nil {
		return nil, err
	}
	if body.Available == nil {
		body.Available = map[string]int{}
	}
	return body.Available, nil
}

func (h *HTTP) SiteID(ctx context.Context) (string, error) {
	var body struct {
		ID string `json:"id"`
	}
	if err := h.getJSON(ctx, h.Inventory+"/v1/sites/code/MAIN", &body); err != nil {
		return "", err
	}
	return body.ID, nil
}

func (h *HTTP) Inbound(ctx context.Context, raw []byte) (map[string]any, int, []byte, error) {
	return doJSON[map[string]any](ctx, h.Client, http.MethodPost, h.Inventory+"/v1/inbound", raw, nil)
}

func (h *HTTP) Checkout(ctx context.Context, raw []byte, header http.Header) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.Order+"/v1/checkout", bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	copyAuth(req, header)
	res, err := h.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return b, res.StatusCode, err
}

func (h *HTTP) GetOrders(ctx context.Context, path string, header http.Header) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Order+path, nil)
	if err != nil {
		return nil, 0, err
	}
	copyAuth(req, header)
	res, err := h.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return b, res.StatusCode, err
}

func copyAuth(req *http.Request, header http.Header) {
	for _, k := range []string{"X-Dev-User-Sub", "X-Dev-Role", "Idempotency-Key", "Content-Type"} {
		if v := header.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
}

func (h *HTTP) getJSON(ctx context.Context, url string, dest any) error {
	_, err := h.getJSONStatus(ctx, url, dest)
	return err
}

func (h *HTTP) getJSONStatus(ctx context.Context, url string, dest any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	res, err := h.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, err
	}
	if res.StatusCode >= 400 {
		return res.StatusCode, fmt.Errorf("%s", b)
	}
	if err := json.Unmarshal(b, dest); err != nil {
		return res.StatusCode, err
	}
	return res.StatusCode, nil
}

func doJSON[T any](ctx context.Context, client *http.Client, method, url string, raw []byte, header http.Header) (T, int, []byte, error) {
	var zero T
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(raw))
	if err != nil {
		return zero, 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if header != nil {
		copyAuth(req, header)
	}
	res, err := client.Do(req)
	if err != nil {
		return zero, 0, nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return zero, res.StatusCode, nil, err
	}
	if res.StatusCode >= 400 {
		return zero, res.StatusCode, b, nil
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, res.StatusCode, b, err
	}
	return out, res.StatusCode, b, nil
}
