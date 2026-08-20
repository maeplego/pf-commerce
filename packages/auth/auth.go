package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ctxKey struct{}

type Role string

const (
	RoleBuyer Role = "buyer"
	RoleOps   Role = "ops"
)

type User struct {
	Sub  string
	Role Role
}

func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, ctxKey{}, u)
}

func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(ctxKey{}).(User)
	return u, ok
}

type Middleware struct {
	devAuth      bool
	issuer       string
	internalBase string
}

func New(devAuth bool, issuer, internalBase string) *Middleware {
	if internalBase == "" {
		internalBase = issuer
	}
	return &Middleware{devAuth: devAuth, issuer: strings.TrimRight(strings.TrimSpace(issuer), "/"), internalBase: strings.TrimRight(strings.TrimSpace(internalBase), "/")}
}

func (m *Middleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := m.authenticate(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), u)))
	})
}

func (m *Middleware) authenticate(r *http.Request) (User, error) {
	if m.devAuth {
		sub := strings.TrimSpace(r.Header.Get("X-Dev-User-Sub"))
		if sub != "" {
			role := Role(strings.ToLower(strings.TrimSpace(r.Header.Get("X-Dev-Role"))))
			if role == "" {
				role = RoleBuyer
			}
			if role != RoleBuyer && role != RoleOps {
				return User{}, fmt.Errorf("invalid role")
			}
			return User{Sub: sub, Role: role}, nil
		}
	}
	authz := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authz, "Bearer ") {
		return User{}, fmt.Errorf("missing bearer")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
	if token == "" || m.internalBase == "" {
		return User{}, fmt.Errorf("oidc not configured")
	}
	sub, err := m.userinfoSub(r.Context(), token)
	if err != nil {
		return User{}, err
	}
	role := RoleBuyer
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Dev-Role")), "ops") {
		role = RoleOps
	}
	return User{Sub: sub, Role: role}, nil
}

func (m *Middleware) userinfoSub(ctx context.Context, token string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.internalBase+"/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("userinfo %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	if err != nil {
		return "", err
	}
	var ui struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(body, &ui); err != nil {
		return "", err
	}
	sub := strings.TrimSpace(ui.Sub)
	if sub == "" {
		return "", fmt.Errorf("empty sub")
	}
	return sub, nil
}
