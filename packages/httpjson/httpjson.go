package httpjson

import (
	"encoding/json"
	"net/http"
)

func Decode(r *http.Request, dest any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dest)
}

func Write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func WriteError(w http.ResponseWriter, status int, code, msg string) {
	Write(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func MountHealth(mux *http.ServeMux, ready func() error) {
	if ready == nil {
		ready = func() error { return nil }
	}
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		Write(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, _ *http.Request) {
		if err := ready(); err != nil {
			WriteError(w, http.StatusServiceUnavailable, "not_ready", err.Error())
			return
		}
		Write(w, http.StatusOK, map[string]any{"ok": true})
	})
}

func CORS(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
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
