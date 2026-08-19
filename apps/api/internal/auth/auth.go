package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
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
	devAuth bool
}

func New(devAuth bool) *Middleware {
	return &Middleware{devAuth: devAuth}
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
	if !m.devAuth {
		return User{}, fmt.Errorf("dev auth disabled")
	}
	sub := strings.TrimSpace(r.Header.Get("X-Dev-User-Sub"))
	if sub == "" {
		return User{}, fmt.Errorf("missing sub")
	}
	role := Role(strings.ToLower(strings.TrimSpace(r.Header.Get("X-Dev-Role"))))
	if role == "" {
		role = RoleBuyer
	}
	if role != RoleBuyer && role != RoleOps {
		return User{}, fmt.Errorf("invalid role")
	}
	return User{Sub: sub, Role: role}, nil
}
