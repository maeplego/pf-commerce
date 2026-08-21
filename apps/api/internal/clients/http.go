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
	OrgID       string `json:"orgId"`
}

type HTTP struct {
	Catalog   string
	Inventory string
	Order     string
	Notify    string
	Client    *http.Client
}

func New(catalog, inventory, orderURL, notifyURL string) *HTTP {
	return &HTTP{
		Catalog:   strings.TrimRight(catalog, "/"),
		Inventory: strings.TrimRight(inventory, "/"),
		Order:     strings.TrimRight(orderURL, "/"),
		Notify:    strings.TrimRight(notifyURL, "/"),
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

func (h *HTTP) ListProducts(ctx context.Context, orgID string) ([]Product, error) {
	var body struct {
		Products []Product `json:"products"`
	}
	u := h.Catalog + "/v1/products"
	if orgID != "" {
		u += "?orgId=" + orgID
	}
	if err := h.getJSON(ctx, u, &body); err != nil {
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

type StockRow struct {
	ProductID    string `json:"productId"`
	Qty          int    `json:"qty"`
	ReservedQty  int    `json:"reservedQty"`
	AvailableQty int    `json:"availableQty"`
	UpdatedAt    string `json:"updatedAt"`
}

func (h *HTTP) Stock(ctx context.Context, siteID, cursor string, limit int) ([]StockRow, string, error) {
	u := fmt.Sprintf("%s/v1/stock?siteId=%s&cursor=%s&limit=%d", h.Inventory, siteID, cursor, limit)
	var body struct {
		Items      []StockRow `json:"items"`
		NextCursor string     `json:"nextCursor"`
	}
	if err := h.getJSON(ctx, u, &body); err != nil {
		return nil, "", err
	}
	return body.Items, body.NextCursor, nil
}

func (h *HTTP) Reviews(ctx context.Context, ids []string) ([]map[string]any, error) {
	var body struct {
		Reviews []map[string]any `json:"reviews"`
	}
	q := h.Catalog + "/v1/reviews?productIds=" + strings.Join(ids, ",")
	if err := h.getJSON(ctx, q, &body); err != nil {
		return nil, err
	}
	return body.Reviews, nil
}

func (h *HTTP) Notifications(ctx context.Context) ([]byte, int, error) {
	if h.Notify == "" {
		return []byte(`{"notifications":[]}`), 200, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.Notify+"/v1/notifications", nil)
	if err != nil {
		return nil, 0, err
	}
	res, err := h.Client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	return b, res.StatusCode, err
}

func (h *HTTP) StockStreamURL() string {
	return h.Inventory + "/v1/stock/stream"
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

func (h *HTTP) PostOrder(ctx context.Context, path string, header http.Header) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.Order+path, bytes.NewReader([]byte("{}")))
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

func copyAuth(req *http.Request, header http.Header) {
	for _, k := range []string{"Authorization", "X-Dev-User-Sub", "X-Dev-User-Org", "X-Dev-Role", "Idempotency-Key", "Content-Type"} {
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
