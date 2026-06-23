package auth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type fakeAuthenticator struct {
	user User
	err  error
}

func (f fakeAuthenticator) Authenticate(context.Context, string) (User, error) {
	return f.user, f.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMiddleware(t *testing.T) {
	user := User{ID: uuid.New(), Email: "u@example.com", IsActive: true}

	tests := []struct {
		name       string
		header     string
		auth       fakeAuthenticator
		wantStatus int
		wantUser   bool
	}{
		{"missing header", "", fakeAuthenticator{}, http.StatusUnauthorized, false},
		{"invalid token", "Bearer bad", fakeAuthenticator{err: ErrInvalidToken}, http.StatusUnauthorized, false},
		{"internal error", "Bearer good", fakeAuthenticator{err: errors.New("db down")}, http.StatusInternalServerError, false},
		{"valid token", "Bearer good", fakeAuthenticator{user: user}, http.StatusOK, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var injected bool
			next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				_, injected = UserFromContext(r.Context())
			})

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()

			Middleware(tt.auth, discardLogger())(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if injected != tt.wantUser {
				t.Errorf("user injected = %v, want %v", injected, tt.wantUser)
			}
		})
	}
}

func TestBearerToken(t *testing.T) {
	cases := map[string]string{
		"Bearer abc":  "abc",
		"bearer abc":  "abc",
		"Bearer  abc": "abc",
		"Basic abc":   "",
		"abc":         "",
		"":            "",
	}
	for header, want := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		if got := bearerToken(req); got != want {
			t.Errorf("bearerToken(%q) = %q, want %q", header, got, want)
		}
	}
}
