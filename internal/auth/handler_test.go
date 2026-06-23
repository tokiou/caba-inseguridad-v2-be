package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type fakeAuthService struct {
	registerResp  RegisterResponse
	loginResult   authResult
	refreshResult authResult
	err           error
}

func (f fakeAuthService) Register(context.Context, RegisterRequest, RequestMeta) (RegisterResponse, error) {
	return f.registerResp, f.err
}
func (f fakeAuthService) Login(context.Context, LoginRequest, RequestMeta) (authResult, error) {
	return f.loginResult, f.err
}
func (f fakeAuthService) Refresh(context.Context, string, RequestMeta) (authResult, error) {
	return f.refreshResult, f.err
}
func (f fakeAuthService) Logout(context.Context, string, RequestMeta) error {
	return f.err
}

func testCookieConfig() CookieConfig {
	return CookieConfig{Name: "refresh_token", Path: "/api/v1/auth", Secure: false, SameSite: http.SameSiteLaxMode, MaxAge: 604800}
}

func passthrough(next http.Handler) http.Handler { return next }

func newTestRouter(svc authService) http.Handler {
	h := NewHandler(svc, passthrough, testCookieConfig(), discardLogger())
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) { h.Register(r) })
	return r
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestHandlerRegister(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		svc        fakeAuthService
		wantStatus int
		wantBody   string
	}{
		{"created", `{"email":"a@b.com","password":"password123"}`,
			fakeAuthService{registerResp: RegisterResponse{ID: uuid.New(), Email: "a@b.com"}}, http.StatusCreated, "a@b.com"},
		{"email taken", `{"email":"a@b.com","password":"password123"}`,
			fakeAuthService{err: ErrEmailTaken}, http.StatusConflict, "email_taken"},
		{"short password", `{"email":"a@b.com","password":"x"}`,
			fakeAuthService{err: ErrPasswordTooShort}, http.StatusBadRequest, "at least 8"},
		{"malformed json", `not json`,
			fakeAuthService{}, http.StatusBadRequest, "invalid_request"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			newTestRouter(tt.svc).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHandlerLogin(t *testing.T) {
	t.Run("success sets refresh cookie and returns access token", func(t *testing.T) {
		svc := fakeAuthService{loginResult: authResult{
			Response:     LoginResponse{AccessToken: "jwt-token", TokenType: "bearer", ExpiresIn: 900},
			RefreshToken: "refresh-xyz",
		}}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"a@b.com","password":"password123"}`))
		rec := httptest.NewRecorder()
		newTestRouter(svc).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "jwt-token") {
			t.Error("response missing access token")
		}
		cookie := findCookie(rec.Result().Cookies(), "refresh_token")
		if cookie == nil {
			t.Fatal("no refresh_token cookie set")
		}
		if cookie.Value != "refresh-xyz" || !cookie.HttpOnly || cookie.Path != "/api/v1/auth" {
			t.Errorf("unexpected cookie %+v", cookie)
		}
	})

	t.Run("invalid credentials", func(t *testing.T) {
		svc := fakeAuthService{err: ErrInvalidCredentials}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"a@b.com","password":"nope12345"}`))
		rec := httptest.NewRecorder()
		newTestRouter(svc).ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "invalid_credentials") {
			t.Errorf("body missing invalid_credentials: %s", rec.Body.String())
		}
		if findCookie(rec.Result().Cookies(), "refresh_token") != nil {
			t.Error("no cookie should be set on failed login")
		}
	})
}

func TestHandlerLogoutClearsCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec := httptest.NewRecorder()
	newTestRouter(fakeAuthService{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	cookie := findCookie(rec.Result().Cookies(), "refresh_token")
	if cookie == nil || cookie.MaxAge >= 0 || cookie.Value != "" {
		t.Errorf("logout should clear the cookie, got %+v", cookie)
	}
}

func TestHandlerMe(t *testing.T) {
	user := User{ID: uuid.New(), Email: "me@example.com", IsActive: true}
	inject := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithUser(r.Context(), user)))
		})
	}
	h := NewHandler(fakeAuthService{}, inject, testCookieConfig(), discardLogger())
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) { h.Register(r) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "me@example.com") {
		t.Errorf("body missing email: %s", rec.Body.String())
	}
}
