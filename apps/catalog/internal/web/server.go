package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/portfolio/pf-commerce/apps/catalog/internal/catalog"
	"github.com/portfolio/pf-commerce/packages/auth"
	"github.com/portfolio/pf-commerce/packages/httpjson"
	"github.com/portfolio/pf-commerce/packages/money"
)

type Server struct {
	cat   *catalog.Service
	ready func() error
}

func New(cat *catalog.Service, ready func() error) *Server {
	if ready == nil {
		ready = func() error { return nil }
	}
	return &Server{cat: cat, ready: ready}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	httpjson.MountHealth(mux, s.ready)
	mux.HandleFunc("GET /v1/products", s.list)
	mux.HandleFunc("GET /v1/products/{id}", s.get)
	mux.HandleFunc("GET /v1/reviews", s.reviews)
	mux.HandleFunc("POST /v1/products", s.create)
	return mux
}

type productJSON struct {
	ID          string `json:"id"`
	OrgID       string `json:"orgId"`
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	Description string `json:"description"`
	PriceMinor  int64  `json:"priceMinor"`
	Currency    string `json:"currency"`
	ImageURL    string `json:"imageUrl"`
	Active      bool   `json:"active"`
}

func toJSON(p catalog.Product) productJSON {
	return productJSON{
		ID: p.ID, OrgID: p.OrgID, SKU: p.SKU, Name: p.Name, Description: p.Description,
		PriceMinor: p.Price.Minor, Currency: p.Price.Currency, ImageURL: p.ImageURL, Active: p.Active,
	}
}

func requestOrgID(r *http.Request) string {
	if org := strings.TrimSpace(r.URL.Query().Get("orgId")); org != "" {
		return org
	}
	if org := strings.TrimSpace(r.Header.Get("X-Dev-User-Org")); org != "" {
		return org
	}
	return auth.DefaultOrgID
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	list, err := s.cat.List(r.Context(), requestOrgID(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]productJSON, 0, len(list))
	for _, p := range list {
		out = append(out, toJSON(p))
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"products": out})
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	p, err := s.cat.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toJSON(p))
}

func (s *Server) reviews(w http.ResponseWriter, r *http.Request) {
	raw := strings.Split(r.URL.Query().Get("productIds"), ",")
	var ids []string
	for _, id := range raw {
		id = strings.TrimSpace(id)
		if id != "" {
			ids = append(ids, id)
		}
	}
	list, err := s.cat.ListReviews(r.Context(), ids)
	if err != nil {
		writeErr(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, rv := range list {
		out = append(out, map[string]any{
			"id": rv.ID, "productId": rv.ProductID, "author": rv.Author, "body": rv.Body,
			"createdAt": rv.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"reviews": out})
}

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		OrgID       string `json:"orgId"`
		SKU         string `json:"sku"`
		Name        string `json:"name"`
		Description string `json:"description"`
		PriceMinor  int64  `json:"priceMinor"`
		Currency    string `json:"currency"`
		ImageURL    string `json:"imageUrl"`
	}
	if err := httpjson.Decode(r, &body); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", "invalid json")
		return
	}
	orgID := body.OrgID
	if orgID == "" {
		orgID = requestOrgID(r)
	}
	p, err := s.cat.Create(r.Context(), catalog.CreateInput{
		OrgID: orgID, SKU: body.SKU, Name: body.Name, Description: body.Description,
		PriceMinor: body.PriceMinor, Currency: body.Currency, ImageURL: body.ImageURL,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, toJSON(p))
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, catalog.ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, catalog.ErrConflict):
		httpjson.WriteError(w, http.StatusConflict, "conflict", err.Error())
	case errors.Is(err, catalog.ErrInvalid), errors.Is(err, money.ErrInvalid), errors.Is(err, money.ErrCurrency):
		httpjson.WriteError(w, http.StatusBadRequest, "invalid", err.Error())
	default:
		httpjson.WriteError(w, http.StatusInternalServerError, "error", err.Error())
	}
}
