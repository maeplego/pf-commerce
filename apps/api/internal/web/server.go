package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/portfolio/pf-commerce/apps/api/internal/cart"
	"github.com/portfolio/pf-commerce/apps/api/internal/clients"
	"github.com/portfolio/pf-commerce/packages/auth"
	"github.com/portfolio/pf-commerce/packages/httpjson"
	"github.com/portfolio/pf-commerce/packages/recommendevents"
)

type Server struct {
	be          *clients.HTTP
	carts       *cart.Service
	siteID      string
	cors        string
	auth        *auth.Middleware
	recommendURL string
	ready       func() error
}

func New(be *clients.HTTP, carts *cart.Service, siteID, cors, recommendURL string, mw *auth.Middleware, ready func() error) *Server {
	if ready == nil {
		ready = func() error { return nil }
	}
	return &Server{
		be: be, carts: carts, siteID: siteID, cors: cors, auth: mw,
		recommendURL: strings.TrimSpace(recommendURL), ready: ready,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	httpjson.MountHealth(mux, s.ready)
	mux.HandleFunc("GET /v1/products", s.listProducts)
	mux.HandleFunc("GET /v1/products/{id}", s.getProduct)
	mux.Handle("GET /v1/cart", s.auth.Handler(http.HandlerFunc(s.getCart)))
	mux.Handle("POST /v1/cart/items", s.auth.Handler(http.HandlerFunc(s.addCartItem)))
	mux.Handle("PUT /v1/cart", s.auth.Handler(http.HandlerFunc(s.replaceCart)))
	mux.Handle("POST /v1/checkout", s.auth.Handler(http.HandlerFunc(s.checkout)))
	mux.Handle("GET /v1/orders", s.auth.Handler(http.HandlerFunc(s.proxyOrderGET)))
	mux.Handle("GET /v1/orders/{id}/events", s.auth.Handler(http.HandlerFunc(s.proxyOrderGET)))
	mux.Handle("POST /v1/orders/{id}/ship", s.auth.Handler(http.HandlerFunc(s.proxyOrderPOST)))
	mux.Handle("GET /v1/orders/{id}", s.auth.Handler(http.HandlerFunc(s.proxyOrderGET)))
	mux.Handle("POST /v1/admin/projections/rebuild", s.auth.Handler(http.HandlerFunc(s.proxyOrderPOST)))
	mux.Handle("POST /v1/ops/products", s.auth.Handler(http.HandlerFunc(s.opsCreateProduct)))
	mux.Handle("POST /v1/ops/stock-inbound", s.auth.Handler(http.HandlerFunc(s.opsInbound)))
	mux.Handle("GET /v1/ops/stock", s.auth.Handler(http.HandlerFunc(s.opsStock)))
	mux.Handle("GET /v1/ops/notifications", s.auth.Handler(http.HandlerFunc(s.opsNotifications)))
	mux.HandleFunc("GET /v1/ops/stock/stream", s.opsStockStream)
	return httpjson.CORS(s.cors, mux)
}

type productJSON struct {
	ID           string `json:"id"`
	SKU          string `json:"sku"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	PriceMinor   int64  `json:"priceMinor"`
	Currency     string `json:"currency"`
	ImageURL     string `json:"imageUrl"`
	Active       bool   `json:"active"`
	AvailableQty int    `json:"availableQty"`
}

func withQty(p clients.Product, qty int) productJSON {
	return productJSON{
		ID: p.ID, SKU: p.SKU, Name: p.Name, Description: p.Description,
		PriceMinor: p.PriceMinor, Currency: p.Currency, ImageURL: p.ImageURL,
		Active: p.Active, AvailableQty: qty,
	}
}

func (s *Server) listProducts(w http.ResponseWriter, r *http.Request) {
	list, err := s.be.ListProducts(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	ids := make([]string, 0, len(list))
	for _, p := range list {
		ids = append(ids, p.ID)
	}
	avail, err := s.be.Available(r.Context(), s.siteID, ids)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	out := make([]productJSON, 0, len(list))
	for _, p := range list {
		out = append(out, withQty(p, avail[p.ID]))
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"products": out})
}

func (s *Server) getProduct(w http.ResponseWriter, r *http.Request) {
	p, st, err := s.be.GetProduct(r.Context(), r.PathValue("id"))
	if st == http.StatusNotFound {
		httpjson.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if st == http.StatusBadRequest {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid")
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	avail, err := s.be.Available(r.Context(), s.siteID, []string{p.ID})
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, withQty(p, avail[p.ID]))
}

func (s *Server) getCart(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	c, err := s.carts.Get(r.Context(), u.Sub)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, cartJSON(c))
}

func (s *Server) addCartItem(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var body struct {
		ProductID string `json:"productId"`
		Qty       int    `json:"qty"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	if _, st, err := s.be.GetProduct(r.Context(), body.ProductID); st == http.StatusNotFound || st == http.StatusBadRequest {
		httpjson.WriteError(w, st, "invalid", "product")
		return
	} else if err != nil && st == 0 {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	c, err := s.carts.Add(r.Context(), u.Sub, body.ProductID, body.Qty)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, cartJSON(c))
}

func (s *Server) replaceCart(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var body struct {
		Items []struct {
			ProductID string `json:"productId"`
			Qty       int    `json:"qty"`
		} `json:"items"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	items := make([]cart.Item, 0, len(body.Items))
	for _, it := range body.Items {
		items = append(items, cart.Item{ProductID: it.ProductID, Qty: it.Qty})
	}
	c, err := s.carts.Replace(r.Context(), u.Sub, items)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, cartJSON(c))
}

func (s *Server) checkout(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	var body struct {
		IdempotencyKey string `json:"idempotencyKey"`
		Lines          []struct {
			ProductID string `json:"productId"`
			Qty       int    `json:"qty"`
		} `json:"lines"`
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
			return
		}
	}
	useCart := len(body.Lines) == 0
	if useCart {
		c, err := s.carts.Get(r.Context(), u.Sub)
		if err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, "invalid", err.Error())
			return
		}
		if len(c.Items) == 0 {
			httpjson.WriteError(w, http.StatusBadRequest, "invalid", "empty cart")
			return
		}
		for _, it := range c.Items {
			body.Lines = append(body.Lines, struct {
				ProductID string `json:"productId"`
				Qty       int    `json:"qty"`
			}{it.ProductID, it.Qty})
		}
	}
	payload, err := json.Marshal(map[string]any{
		"idempotencyKey": body.IdempotencyKey,
		"siteId":         s.siteID,
		"lines":          body.Lines,
	})
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, "error", err.Error())
		return
	}
	out, st, err := s.be.Checkout(r.Context(), payload, r.Header)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	if st == http.StatusCreated || st == http.StatusOK {
		var o struct {
			Status string `json:"status"`
			Lines  []struct {
				SKU string `json:"sku"`
				Qty int    `json:"qty"`
			} `json:"lines"`
		}
		if json.Unmarshal(out, &o) == nil && o.Status == "paid" {
			if useCart {
				_ = s.carts.Clear(r.Context(), u.Sub)
			}
			if s.recommendURL != "" && len(o.Lines) > 0 {
				lines := make([]recommendevents.Line, 0, len(o.Lines))
				for _, ln := range o.Lines {
					lines = append(lines, recommendevents.Line{SKU: ln.SKU, Qty: ln.Qty})
				}
				go recommendevents.PostPurchase(context.Background(), s.recommendURL, u.Sub, lines, nil)
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(st)
	_, _ = w.Write(out)
}

func (s *Server) proxyOrderGET(w http.ResponseWriter, r *http.Request) {
	out, st, err := s.be.GetOrders(r.Context(), r.URL.Path, r.Header)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(st)
	_, _ = w.Write(out)
}

func (s *Server) proxyOrderPOST(w http.ResponseWriter, r *http.Request) {
	out, st, err := s.be.PostOrder(r.Context(), r.URL.Path, r.Header)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(st)
	_, _ = w.Write(out)
}

func (s *Server) opsCreateProduct(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if u.Role != auth.RoleOps {
		httpjson.WriteError(w, http.StatusForbidden, "forbidden", "ops role required")
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	p, st, b, err := s.be.CreateProduct(r.Context(), raw)
	if err != nil && st == 0 {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	if st >= 400 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		_, _ = w.Write(b)
		return
	}
	avail, _ := s.be.Available(r.Context(), s.siteID, []string{p.ID})
	httpjson.Write(w, http.StatusCreated, withQty(p, avail[p.ID]))
}

func (s *Server) opsInbound(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if u.Role != auth.RoleOps {
		httpjson.WriteError(w, http.StatusForbidden, "forbidden", "ops role required")
		return
	}
	var body struct {
		ProductID string `json:"productId"`
		Qty       int    `json:"qty"`
		Reason    string `json:"reason"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"siteId": s.siteID, "productId": body.ProductID, "qty": body.Qty, "reason": body.Reason, "actorId": u.Sub,
	})
	out, st, b, err := s.be.Inbound(r.Context(), payload)
	if err != nil && st == 0 {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	if st >= 400 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(st)
		_, _ = w.Write(b)
		return
	}
	httpjson.Write(w, st, out)
}

func (s *Server) opsStock(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if u.Role != auth.RoleOps {
		httpjson.WriteError(w, http.StatusForbidden, "forbidden", "ops role required")
		return
	}
	products, err := s.be.ListProducts(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	byID := map[string]clients.Product{}
	for _, p := range products {
		byID[p.ID] = p
	}
	cursor := r.URL.Query().Get("cursor")
	rows, next, err := s.be.Stock(r.Context(), s.siteID, cursor, 50)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"productId": row.ProductID, "qty": row.Qty, "reservedQty": row.ReservedQty,
			"availableQty": row.AvailableQty, "updatedAt": row.UpdatedAt,
			"sku": "", "name": "",
		}
		if p, ok := byID[row.ProductID]; ok {
			item["sku"] = p.SKU
			item["name"] = p.Name
		}
		out = append(out, item)
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"items": out, "nextCursor": next})
}

func (s *Server) opsNotifications(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if u.Role != auth.RoleOps {
		httpjson.WriteError(w, http.StatusForbidden, "forbidden", "ops role required")
		return
	}
	out, st, err := s.be.Notifications(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(st)
	_, _ = w.Write(out)
}

func (s *Server) opsStockStream(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimSpace(r.Header.Get("X-Dev-User-Sub"))
	if sub == "" {
		sub = strings.TrimSpace(r.URL.Query().Get("devUser"))
	}
	role := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Dev-Role")))
	if role == "" {
		role = strings.ToLower(strings.TrimSpace(r.URL.Query().Get("devRole")))
	}
	if sub == "" || role != string(auth.RoleOps) {
		httpjson.WriteError(w, http.StatusUnauthorized, "unauthorized", "ops stream requires role")
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError, "error", "stream unsupported")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, s.be.StockStreamURL(), nil)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	res, err := streamClient.Do(req)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadGateway, "upstream", err.Error())
		return
	}
	defer res.Body.Close()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	buf := make([]byte, 1024)
	for {
		n, err := res.Body.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			fl.Flush()
		}
		if err != nil {
			return
		}
	}
}

var streamClient = &http.Client{Timeout: 0}

func cartJSON(c cart.Cart) map[string]any {
	items := make([]map[string]any, 0, len(c.Items))
	for _, it := range c.Items {
		items = append(items, map[string]any{"productId": it.ProductID, "qty": it.Qty, "updatedAt": it.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")})
	}
	return map[string]any{"buyerSub": c.BuyerSub, "items": items}
}
