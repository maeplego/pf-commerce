package web

import (
	"errors"
	"net/http"

	"github.com/portfolio/pf-commerce/apps/notify/internal/notify"
	"github.com/portfolio/pf-commerce/packages/httpjson"
)

type Server struct {
	n     *notify.Service
	ready func() error
}

func New(n *notify.Service, ready func() error) *Server {
	if ready == nil {
		ready = func() error { return nil }
	}
	return &Server{n: n, ready: ready}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	httpjson.MountHealth(mux, s.ready)
	mux.HandleFunc("POST /v1/notifications", s.deliver)
	mux.HandleFunc("GET /v1/notifications", s.list)
	return mux
}

func (s *Server) deliver(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		OrderID  string `json:"orderId"`
		BuyerSub string `json:"buyerSub"`
		Payload  string `json:"payload"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	msg, created, err := s.n.Deliver(r.Context(), notify.Message{
		ID: body.ID, Type: body.Type, OrderID: body.OrderID, BuyerSub: body.BuyerSub, Payload: body.Payload,
	})
	if errors.Is(err, notify.ErrInvalid) {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, "error", err.Error())
		return
	}
	st := http.StatusCreated
	if !created {
		st = http.StatusOK
	}
	httpjson.Write(w, st, map[string]any{
		"id": msg.ID, "type": msg.Type, "orderId": msg.OrderID, "buyerSub": msg.BuyerSub,
	})
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	list := s.n.List(r.Context())
	out := make([]map[string]any, 0, len(list))
	for _, m := range list {
		out = append(out, map[string]any{
			"id": m.ID, "type": m.Type, "orderId": m.OrderID, "buyerSub": m.BuyerSub,
			"payload": m.Payload, "createdAt": m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"notifications": out})
}
