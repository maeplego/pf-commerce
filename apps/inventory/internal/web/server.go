package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/portfolio/pf-commerce/apps/inventory/internal/hub"
	"github.com/portfolio/pf-commerce/apps/inventory/internal/inventory"
	"github.com/portfolio/pf-commerce/packages/httpjson"
)

type Server struct {
	inv   *inventory.Service
	hub   *hub.Hub
	ready func() error
}

func New(inv *inventory.Service, h *hub.Hub, ready func() error) *Server {
	if ready == nil {
		ready = func() error { return nil }
	}
	if h == nil {
		h = hub.New()
	}
	return &Server{inv: inv, hub: h, ready: ready}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	httpjson.MountHealth(mux, s.ready)
	mux.HandleFunc("GET /v1/sites/code/{code}", s.siteByCode)
	mux.HandleFunc("GET /v1/available", s.available)
	mux.HandleFunc("GET /v1/stock", s.stock)
	mux.HandleFunc("GET /v1/stock/stream", s.stream)
	mux.HandleFunc("POST /v1/inbound", s.inbound)
	mux.HandleFunc("POST /v1/reserve", s.reserve)
	mux.HandleFunc("POST /v1/release", s.release)
	mux.HandleFunc("POST /v1/consume", s.consume)
	return mux
}

func (s *Server) siteByCode(w http.ResponseWriter, r *http.Request) {
	site, err := s.inv.SiteByCode(r.Context(), r.PathValue("code"))
	if err != nil {
		writeErr(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"id": site.ID, "code": site.Code, "name": site.Name})
}

func (s *Server) available(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	siteID := q.Get("siteId")
	ids := strings.Split(q.Get("productIds"), ",")
	out := map[string]int{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		n, err := s.inv.Available(r.Context(), siteID, id)
		if err != nil {
			writeErr(w, err)
			return
		}
		out[id] = n
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"available": out})
}

func (s *Server) stock(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	rows, next, err := s.inv.ListStock(r.Context(), q.Get("siteId"), q.Get("cursor"), limit)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(rows))
	for _, b := range rows {
		out = append(out, map[string]any{
			"productId": b.ProductID, "qty": b.Qty, "reservedQty": b.ReservedQty,
			"availableQty": b.Available(), "updatedAt": b.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"items": out, "nextCursor": next})
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError, "error", "stream unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := s.hub.Subscribe()
	defer s.hub.Unsubscribe(ch)
	fl.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case raw, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write([]byte("event: stock.updated\ndata: "))
			_, _ = w.Write(raw)
			_, _ = w.Write([]byte("\n\n"))
			fl.Flush()
		}
	}
}

func (s *Server) inbound(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SiteID    string `json:"siteId"`
		ProductID string `json:"productId"`
		Qty       int    `json:"qty"`
		Reason    string `json:"reason"`
		ActorID   string `json:"actorId"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	b, err := s.inv.Inbound(r.Context(), body.SiteID, body.ProductID, body.ActorID, body.Reason, body.Qty)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.hub.Publish(hub.Event{
		ProductID: b.ProductID, Qty: b.Qty, ReservedQty: b.ReservedQty,
		AvailableQty: b.Available(), Reason: "inbound",
	})
	httpjson.Write(w, http.StatusCreated, map[string]any{
		"productId": b.ProductID, "qty": b.Qty, "reservedQty": b.ReservedQty, "availableQty": b.Available(),
	})
}

func (s *Server) reserve(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SiteID    string `json:"siteId"`
		ProductID string `json:"productId"`
		OrderID   string `json:"orderId"`
		ActorID   string `json:"actorId"`
		Qty       int    `json:"qty"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	res, err := s.inv.Reserve(r.Context(), body.SiteID, body.ProductID, body.OrderID, body.ActorID, body.Qty)
	if err != nil {
		writeErr(w, err)
		return
	}
	s.publishBalance(r, body.SiteID, body.ProductID, "reserve")
	httpjson.Write(w, http.StatusCreated, map[string]any{
		"id": res.ID, "orderId": res.OrderID, "qty": res.Qty, "status": res.Status,
		"expiresAt": res.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z"),
	})
}

func (s *Server) release(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrderID string `json:"orderId"`
		ActorID string `json:"actorId"`
		Reason  string `json:"reason"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	if err := s.inv.ReleaseOrder(r.Context(), body.OrderID, body.ActorID, body.Reason); err != nil {
		writeErr(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) consume(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrderID string `json:"orderId"`
		ActorID string `json:"actorId"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	if err := s.inv.ConsumeOrder(r.Context(), body.OrderID, body.ActorID); err != nil {
		writeErr(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) publishBalance(r *http.Request, siteID, productID, reason string) {
	b, err := s.inv.GetBalance(r.Context(), siteID, productID)
	if err != nil {
		return
	}
	s.hub.Publish(hub.Event{
		ProductID: b.ProductID, Qty: b.Qty, ReservedQty: b.ReservedQty,
		AvailableQty: b.Available(), Reason: reason,
	})
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, inventory.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, inventory.ErrShortage):
		httpjson.WriteError(w, http.StatusConflict, "inventory_shortage", "not enough stock")
	case errors.Is(err, inventory.ErrConflict):
		httpjson.WriteError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, inventory.ErrInvalid), errors.Is(err, inventory.ErrExpired):
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", err.Error())
	default:
		httpjson.WriteError(w, http.StatusInternalServerError, "error", err.Error())
	}
}
