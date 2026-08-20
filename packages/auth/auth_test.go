package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/portfolio/pf-commerce/packages/auth"
)

func TestDevAuthHeader(t *testing.T) {
	mw := auth.New(true, "", "")
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.UserFrom(r.Context())
		if !ok || u.Sub != "alice" {
			t.Fatalf("%+v", u)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Dev-User-Sub", "alice")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestOidcRequiresBearerWhenDevOff(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/userinfo" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"demo@example.test"}`))
	}))
	defer up.Close()

	mw := auth.New(false, up.URL, up.URL)
	h := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.UserFrom(r.Context())
		if !ok || u.Sub != "demo@example.test" {
			t.Fatalf("%+v", u)
		}
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer fake-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d", rec.Code)
	}
}
