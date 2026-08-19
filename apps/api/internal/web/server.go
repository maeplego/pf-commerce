package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/portfolio/pf-commerce/api/internal/auth"
	"github.com/portfolio/pf-commerce/api/internal/cart"
	"github.com/portfolio/pf-commerce/api/internal/catalog"
	"github.com/portfolio/pf-commerce/api/internal/inventory"
	"github.com/portfolio/pf-commerce/api/internal/money"
	"github.com/portfolio/pf-commerce/api/internal/order"
	"github.com/portfolio/pf-commerce/api/internal/payment"
)

type Server struct {
	cat    *catalog.Service
	inv    *inventory.Service
	carts  *cart.Service
	orders *order.Service
	siteID string
	cors   string
	auth   *auth.Middleware
	ready  func() error
}

func New(cat *catalog.Service, inv *inventory.Service, carts *cart.Service, orders *order.Service, siteID, cors string, mw *auth.Middleware, ready func() error) *Server {
	if ready == nil {
		ready = func() error { return nil }
	}
	return &Server{cat: cat, inv: inv, carts: carts, orders: orders, siteID: siteID, cors: cors, auth: mw, ready: ready}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		if err := s.ready(); err != nil {
			writeError(w, http.StatusServiceUnavailable, "not_ready", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /v1/products", s.listProducts)
	mux.HandleFunc("GET /v1/products/{id}", s.getProduct)

	mux.Handle("GET /v1/cart", s.auth.Handler(http.HandlerFunc(s.getCart)))
	mux.Handle("POST /v1/cart/items", s.auth.Handler(http.HandlerFunc(s.addCartItem)))
	mux.Handle("PUT /v1/cart", s.auth.Handler(http.HandlerFunc(s.replaceCart)))
	mux.Handle("POST /v1/checkout", s.auth.Handler(http.HandlerFunc(s.checkout)))
	mux.Handle("GET /v1/orders", s.auth.Handler(http.HandlerFunc(s.listOrders)))
	mux.Handle("GET /v1/orders/{id}", s.auth.Handler(http.HandlerFunc(s.getOrder)))
	mux.Handle("POST /v1/ops/products", s.auth.Handler(http.HandlerFunc(s.opsCreateProduct)))
	mux.Handle("POST /v1/ops/stock-inbound", s.auth.Handler(http.HandlerFunc(s.opsInbound)))
	return s.withCORS(mux)
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cors != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.cors)
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, X-Dev-User-Sub, X-Dev-Role")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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

func (s *Server) toProductJSON(r *http.Request, p catalog.Product) productJSON {
	qty, _ := s.inv.Available(r.Context(), s.siteID, p.ID)
	return productJSON{
		ID: p.ID, SKU: p.SKU, Name: p.Name, Description: p.Description,
		PriceMinor: p.Price.Minor, Currency: p.Price.Currency, ImageURL: p.ImageURL,
		Active: p.Active, AvailableQty: qty,
	}
}

func (s *Server) listProducts(w http.ResponseWriter, r *http.Request) {
	list, err := s.cat.List(r.Context())
	if err != nil {
		writeDomainError(w, err)
		return
	}
	out := make([]productJSON, 0, len(list))
	for _, p := range list {
		out = append(out, s.toProductJSON(r, p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": out})
}

func (s *Server) getProduct(w http.ResponseWriter, r *http.Request) {
	p, err := s.cat.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.toProductJSON(r, p))
}

func (s *Server) getCart(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	c, err := s.carts.Get(r.Context(), u.Sub)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cartJSON(c))
}

func (s *Server) addCartItem(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var body struct {
		ProductID string `json:"productId"`
		Qty       int    `json:"qty"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	if _, err := s.cat.Get(r.Context(), body.ProductID); err != nil {
		writeDomainError(w, err)
		return
	}
	c, err := s.carts.Add(r.Context(), u.Sub, body.ProductID, body.Qty)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cartJSON(c))
}

func (s *Server) replaceCart(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var body struct {
		Items []struct {
			ProductID string `json:"productId"`
			Qty       int    `json:"qty"`
		} `json:"items"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	items := make([]cart.Item, 0, len(body.Items))
	for _, it := range body.Items {
		items = append(items, cart.Item{ProductID: it.ProductID, Qty: it.Qty})
	}
	c, err := s.carts.Replace(r.Context(), u.Sub, items)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cartJSON(c))
}

func (s *Server) checkout(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var body struct {
		IdempotencyKey string `json:"idempotencyKey"`
		Lines          []struct {
			ProductID string `json:"productId"`
			Qty       int    `json:"qty"`
		} `json:"lines"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(body.IdempotencyKey)
	}
	in := order.CheckoutInput{BuyerSub: u.Sub, IdempotencyKey: key, SiteID: s.siteID}
	if len(body.Lines) > 0 {
		for _, ln := range body.Lines {
			in.Lines = append(in.Lines, order.CheckoutLine{ProductID: ln.ProductID, Qty: ln.Qty})
		}
	} else {
		in.UseCart = true
	}
	o, created, err := s.orders.Checkout(r.Context(), in)
	if err != nil {
		if o.ID != "" {
			writeJSON(w, statusFor(err), map[string]any{"order": orderJSON(o), "error": errorBody(err)})
			return
		}
		writeDomainError(w, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, orderJSON(o))
}

func (s *Server) listOrders(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	list, err := s.orders.ListMine(r.Context(), u.Sub)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	out := make([]any, 0, len(list))
	for _, o := range list {
		out = append(out, orderJSON(o))
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": out})
}

func (s *Server) getOrder(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	o, err := s.orders.Get(r.Context(), r.PathValue("id"), u.Sub, u.Role == auth.RoleOps)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orderJSON(o))
}

func (s *Server) opsCreateProduct(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if u.Role != auth.RoleOps {
		writeError(w, http.StatusForbidden, "forbidden", "ops role required")
		return
	}
	var body struct {
		SKU         string `json:"sku"`
		Name        string `json:"name"`
		Description string `json:"description"`
		PriceMinor  int64  `json:"priceMinor"`
		Currency    string `json:"currency"`
		ImageURL    string `json:"imageUrl"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	p, err := s.cat.Create(r.Context(), catalog.CreateInput{
		SKU: body.SKU, Name: body.Name, Description: body.Description,
		PriceMinor: body.PriceMinor, Currency: body.Currency, ImageURL: body.ImageURL,
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.toProductJSON(r, p))
}

func (s *Server) opsInbound(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if u.Role != auth.RoleOps {
		writeError(w, http.StatusForbidden, "forbidden", "ops role required")
		return
	}
	var body struct {
		ProductID string `json:"productId"`
		Qty       int    `json:"qty"`
		Reason    string `json:"reason"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	b, err := s.inv.Inbound(r.Context(), s.siteID, body.ProductID, u.Sub, body.Reason, body.Qty)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"productId": b.ProductID, "qty": b.Qty, "reservedQty": b.ReservedQty, "availableQty": b.Available(),
	})
}

func cartJSON(c cart.Cart) map[string]any {
	items := make([]map[string]any, 0, len(c.Items))
	for _, it := range c.Items {
		items = append(items, map[string]any{"productId": it.ProductID, "qty": it.Qty, "updatedAt": it.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")})
	}
	return map[string]any{"buyerSub": c.BuyerSub, "items": items}
}

func orderJSON(o order.Order) map[string]any {
	lines := make([]map[string]any, 0, len(o.Lines))
	for _, ln := range o.Lines {
		lines = append(lines, map[string]any{
			"productId": ln.ProductID, "sku": ln.SKU, "name": ln.Name, "qty": ln.Qty,
			"unitPriceMinor": ln.UnitPriceMinor, "currency": ln.Currency,
		})
	}
	return map[string]any{
		"id": o.ID, "buyerSub": o.BuyerSub, "status": o.Status, "cancelReason": o.CancelReason,
		"amountMinor": o.Amount.Minor, "currency": o.Amount.Currency,
		"idempotencyKey": o.IdempotencyKey, "paymentId": o.PaymentID,
		"lines":     lines,
		"createdAt": o.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		"updatedAt": o.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func decodeJSON(r *http.Request, dest any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func errorBody(err error) map[string]string {
	code, msg := codeOf(err)
	return map[string]string{"code": code, "message": msg}
}

func writeDomainError(w http.ResponseWriter, err error) {
	st := statusFor(err)
	code, msg := codeOf(err)
	writeError(w, st, code, msg)
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, catalog.ErrNotFound), errors.Is(err, order.ErrNotFound), errors.Is(err, inventory.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, order.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, inventory.ErrShortage):
		return http.StatusConflict
	case errors.Is(err, payment.ErrDeclined):
		return http.StatusConflict
	case errors.Is(err, catalog.ErrConflict), errors.Is(err, order.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, catalog.ErrInvalid), errors.Is(err, order.ErrInvalid), errors.Is(err, inventory.ErrInvalid), errors.Is(err, cart.ErrInvalid), errors.Is(err, money.ErrInvalid), errors.Is(err, money.ErrCurrency):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func codeOf(err error) (string, string) {
	switch {
	case errors.Is(err, inventory.ErrShortage):
		return "inventory_shortage", "not enough stock"
	case errors.Is(err, payment.ErrDeclined):
		return "payment_failed", "payment declined"
	case errors.Is(err, order.ErrForbidden):
		return "forbidden", "forbidden"
	case errors.Is(err, catalog.ErrNotFound), errors.Is(err, order.ErrNotFound):
		return "not_found", "not found"
	case errors.Is(err, catalog.ErrConflict), errors.Is(err, order.ErrConflict):
		return "conflict", err.Error()
	default:
		if err == nil {
			return "error", "error"
		}
		return "invalid", err.Error()
	}
}
