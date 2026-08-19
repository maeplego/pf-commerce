package web

import (
	"errors"
	"net/http"

	"github.com/portfolio/pf-commerce/apps/payment/internal/payment"
	"github.com/portfolio/pf-commerce/packages/httpjson"
	"github.com/portfolio/pf-commerce/packages/money"
)

type Server struct {
	pay   *payment.Service
	ready func() error
}

func New(pay *payment.Service, ready func() error) *Server {
	if ready == nil {
		ready = func() error { return nil }
	}
	return &Server{pay: pay, ready: ready}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	httpjson.MountHealth(mux, s.ready)
	mux.HandleFunc("POST /v1/charges", s.charge)
	mux.HandleFunc("POST /v1/test/fail-next", s.failNext)
	return mux
}

func (s *Server) charge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IdempotencyKey string `json:"idempotencyKey"`
		OrderID        string `json:"orderId"`
		BuyerSub       string `json:"buyerSub"`
		AmountMinor    int64  `json:"amountMinor"`
		Currency       string `json:"currency"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	amt, err := money.New(body.AmountMinor, body.Currency)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	ch, err := s.pay.Charge(r.Context(), payment.ChargeRequest{
		IdempotencyKey: body.IdempotencyKey,
		OrderID:        body.OrderID,
		BuyerSub:       body.BuyerSub,
		Amount:         amt,
	})
	if errors.Is(err, payment.ErrDeclined) {
		httpjson.WriteError(w, http.StatusConflict, "payment_failed", "payment declined")
		return
	}
	if errors.Is(err, payment.ErrInvalid) {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, "error", err.Error())
		return
	}
	httpjson.Write(w, http.StatusCreated, map[string]any{
		"id": ch.ID, "idempotencyKey": ch.IdempotencyKey, "orderId": ch.OrderID,
		"amountMinor": ch.Amount.Minor, "currency": ch.Amount.Currency,
	})
}

func (s *Server) failNext(w http.ResponseWriter, r *http.Request) {
	var body struct {
		N int `json:"n"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	s.pay.FailNext(body.N)
	httpjson.Write(w, http.StatusOK, map[string]any{"ok": true})
}
