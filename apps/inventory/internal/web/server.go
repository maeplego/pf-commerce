package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/portfolio/pf-commerce/apps/inventory/internal/inventory"
	"github.com/portfolio/pf-commerce/packages/httpjson"
)

type Server struct {
	inv   *inventory.Service
	ready func() error
}

func New(inv *inventory.Service, ready func() error) *Server {
	if ready == nil {
		ready = func() error { return nil }
	}
	return &Server{inv: inv, ready: ready}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	httpjson.MountHealth(mux, s.ready)
	mux.HandleFunc("GET /v1/sites/code/{code}", s.siteByCode)
	mux.HandleFunc("GET /v1/available", s.available)
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
