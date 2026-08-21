package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/portfolio/pf-commerce/apps/order/internal/order"
	"github.com/portfolio/pf-commerce/packages/auth"
	"github.com/portfolio/pf-commerce/packages/httpjson"
	"github.com/portfolio/pf-commerce/packages/money"
)

type Server struct {
	orders *order.Service
	siteID string
	auth   *auth.Middleware
	ready  func() error
}

func New(orders *order.Service, siteID string, mw *auth.Middleware, ready func() error) *Server {
	if ready == nil {
		ready = func() error { return nil }
	}
	return &Server{orders: orders, siteID: siteID, auth: mw, ready: ready}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	httpjson.MountHealth(mux, s.ready)
	mux.Handle("POST /v1/checkout", s.auth.Handler(http.HandlerFunc(s.checkout)))
	mux.Handle("GET /v1/orders", s.auth.Handler(http.HandlerFunc(s.list)))
	mux.Handle("GET /v1/orders/{id}/events", s.auth.Handler(http.HandlerFunc(s.events)))
	mux.Handle("POST /v1/orders/{id}/ship", s.auth.Handler(http.HandlerFunc(s.ship)))
	mux.Handle("GET /v1/orders/{id}", s.auth.Handler(http.HandlerFunc(s.get)))
	mux.Handle("POST /v1/admin/projections/rebuild", s.auth.Handler(http.HandlerFunc(s.rebuild)))
	mux.Handle("GET /v1/ops/exports/orders", s.auth.Handler(http.HandlerFunc(s.exportOrders)))
	return mux
}

func (s *Server) checkout(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var body struct {
		IdempotencyKey string `json:"idempotencyKey"`
		SiteID         string `json:"siteId"`
		OrgID          string `json:"orgId"`
		Lines          []struct {
			ProductID string `json:"productId"`
			Qty       int    `json:"qty"`
		} `json:"lines"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		key = strings.TrimSpace(body.IdempotencyKey)
	}
	siteID := body.SiteID
	if siteID == "" {
		siteID = s.siteID
	}
	orgID := strings.TrimSpace(body.OrgID)
	if orgID == "" {
		orgID = u.OrgID
	}
	if orgID == "" {
		orgID = auth.DefaultOrgID
	}
	in := order.CheckoutInput{BuyerSub: u.Sub, OrgID: orgID, IdempotencyKey: key, SiteID: siteID}
	for _, ln := range body.Lines {
		in.Lines = append(in.Lines, order.CheckoutLine{ProductID: ln.ProductID, Qty: ln.Qty})
	}
	o, created, err := s.orders.Checkout(r.Context(), in)
	if err != nil {
		if o.ID != "" {
			httpjson.Write(w, statusFor(err), map[string]any{"order": orderJSON(o), "error": errorBody(err)})
			return
		}
		writeErr(w, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	httpjson.Write(w, status, orderJSON(o))
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	list, err := s.orders.ListMine(r.Context(), u.Sub, u.OrgID)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]any, 0, len(list))
	for _, o := range list {
		out = append(out, orderJSON(o))
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"orders": out})
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	o, err := s.orders.Get(r.Context(), r.PathValue("id"), u.Sub, u.OrgID, u.Role == auth.RoleOps)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, orderJSON(o))
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	evs, err := s.orders.Events(r.Context(), r.PathValue("id"), u.Sub, u.OrgID, u.Role == auth.RoleOps)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(evs))
	for _, e := range evs {
		var data any
		if len(e.Data) > 0 {
			_ = json.Unmarshal(e.Data, &data)
		}
		out = append(out, map[string]any{
			"id": e.ID, "streamId": e.StreamID, "version": e.Version,
			"type": e.Type, "time": e.Time.UTC().Format("2006-01-02T15:04:05Z"),
			"data": data,
		})
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"events": out})
}

func (s *Server) ship(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	o, err := s.orders.Ship(r.Context(), r.PathValue("id"), u.Sub, u.OrgID, u.Role == auth.RoleOps)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, orderJSON(o))
}

func (s *Server) rebuild(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if u.Role != auth.RoleOps {
		httpjson.WriteError(w, http.StatusForbidden, "forbidden", "ops role required")
		return
	}
	if err := s.orders.Rebuild(r.Context()); err != nil {
		writeErr(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) exportOrders(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if u.Role != auth.RoleOps {
		httpjson.WriteError(w, http.StatusForbidden, "forbidden", "ops role required")
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("date"))
	if raw == "" {
		raw = time.Now().UTC().Format("2006-01-02")
	}
	day, err := time.Parse("2006-01-02", raw)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "date must be YYYY-MM-DD")
		return
	}
	lines, err := s.orders.ExportLines(r.Context(), day)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"date": raw, "lines": lines, "pii": "buyerOpaque only"})
}

func OrderJSON(o order.Order) map[string]any { return orderJSON(o) }

func orderJSON(o order.Order) map[string]any {
	lines := make([]map[string]any, 0, len(o.Lines))
	for _, ln := range o.Lines {
		lines = append(lines, map[string]any{
			"productId": ln.ProductID, "sku": ln.SKU, "name": ln.Name, "qty": ln.Qty,
			"unitPriceMinor": ln.UnitPriceMinor, "currency": ln.Currency,
		})
	}
	return map[string]any{
		"id": o.ID, "buyerSub": o.BuyerSub, "orgId": o.OrgID, "status": o.Status, "cancelReason": o.CancelReason,
		"amountMinor": o.Amount.Minor, "currency": o.Amount.Currency,
		"idempotencyKey": o.IdempotencyKey, "paymentId": o.PaymentID,
		"lines":     lines,
		"createdAt": o.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		"updatedAt": o.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func writeErr(w http.ResponseWriter, err error) {
	st := statusFor(err)
	code, msg := codeOf(err)
	httpjson.WriteError(w, st, code, msg)
}

func errorBody(err error) map[string]string {
	code, msg := codeOf(err)
	return map[string]string{"code": code, "message": msg}
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, order.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, order.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, order.ErrShortage), errors.Is(err, order.ErrDeclined), errors.Is(err, order.ErrConflict), errors.Is(err, order.ErrInvalidTransition):
		return http.StatusConflict
	case errors.Is(err, order.ErrInvalid), errors.Is(err, money.ErrInvalid), errors.Is(err, money.ErrCurrency):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func codeOf(err error) (string, string) {
	switch {
	case errors.Is(err, order.ErrShortage):
		return "inventory_shortage", "not enough stock"
	case errors.Is(err, order.ErrDeclined):
		return "payment_failed", "payment declined"
	case errors.Is(err, order.ErrForbidden):
		return "forbidden", "forbidden"
	case errors.Is(err, order.ErrNotFound):
		return "not_found", "not found"
	case errors.Is(err, order.ErrConflict):
		return "conflict", err.Error()
	case errors.Is(err, order.ErrInvalidTransition):
		return "invalid_transition", "illegal order state"
	default:
		if err == nil {
			return "error", "error"
		}
		return "invalid", err.Error()
	}
}
