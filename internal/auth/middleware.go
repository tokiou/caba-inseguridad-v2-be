package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/tokiou/caba-inseguridad-routes-go/internal/httpx"
)

type ctxKey int

const userKey ctxKey = iota

// WithUser stores the authenticated user in the context.
func WithUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// UserFromContext returns the authenticated user injected by the middleware.
func UserFromContext(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey).(User)
	return u, ok
}

type authenticator interface {
	Authenticate(ctx context.Context, accessToken string) (User, error)
}

// Middleware validates the Authorization bearer access token, loads the active
// user, and injects it into the request context. Missing or invalid tokens yield
// 401; unexpected failures yield 500.
func Middleware(a authenticator, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}

			user, err := a.Authenticate(r.Context(), token)
			if err != nil {
				if errors.Is(err, ErrInvalidToken) {
					httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or expired access token")
					return
				}
				httpx.LogWith(log, r).Error("auth middleware error", "err", err)
				httpx.WriteInternalError(w, "could not authenticate request")
				return
			}

			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user)))
		})
	}
}

// bearerToken extracts the token from an `Authorization: Bearer <token>` header.
func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}
