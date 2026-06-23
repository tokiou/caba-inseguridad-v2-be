package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"

	"github.com/go-chi/chi/v5"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/httpx"
)

type authService interface {
	Register(ctx context.Context, req RegisterRequest, meta RequestMeta) (RegisterResponse, error)
	Login(ctx context.Context, req LoginRequest, meta RequestMeta) (authResult, error)
	Refresh(ctx context.Context, rawToken string, meta RequestMeta) (authResult, error)
	Logout(ctx context.Context, rawToken string, meta RequestMeta) error
}

// CookieConfig controls how the refresh-token cookie is written.
type CookieConfig struct {
	Name     string
	Path     string
	Secure   bool
	SameSite http.SameSite
	MaxAge   int // seconds
}

type Handler struct {
	service    authService
	middleware func(http.Handler) http.Handler
	cookie     CookieConfig
	log        *slog.Logger
}

func NewHandler(service authService, middleware func(http.Handler) http.Handler, cookie CookieConfig, log *slog.Logger) *Handler {
	return &Handler{service: service, middleware: middleware, cookie: cookie, log: log}
}

func (h *Handler) Register(r chi.Router) {
	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)
	r.Post("/auth/refresh", h.refresh)
	r.Post("/auth/logout", h.logout)
	// /auth/me is the one auth route that requires an access token.
	r.With(h.middleware).Get("/auth/me", h.me)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.WriteInvalidRequest(w, "request body must be JSON with email and password")
		return
	}

	resp, err := h.service.Register(r.Context(), req, requestMeta(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		httpx.WriteInvalidRequest(w, "request body must be JSON with email and password")
		return
	}

	result, err := h.service.Login(r.Context(), req, requestMeta(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.setRefreshCookie(w, result.RefreshToken)
	httpx.WriteJSON(w, http.StatusOK, result.Response)
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.Refresh(r.Context(), h.readRefreshCookie(r), requestMeta(r))
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	h.setRefreshCookie(w, result.RefreshToken)
	httpx.WriteJSON(w, http.StatusOK, result.Response)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if err := h.service.Logout(r.Context(), h.readRefreshCookie(r), requestMeta(r)); err != nil {
		h.writeError(w, r, err)
		return
	}

	h.clearRefreshCookie(w)
	httpx.WriteJSON(w, http.StatusOK, LogoutResponse{Message: "logged out"})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, MeResponse{ID: user.ID, Email: user.Email})
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrEmailRequired):
		httpx.WriteInvalidRequest(w, "email and password are required")
	case errors.Is(err, ErrPasswordTooShort):
		httpx.WriteInvalidRequest(w, "password must be at least 8 characters")
	case errors.Is(err, ErrEmailTaken):
		httpx.WriteError(w, http.StatusConflict, "email_taken", "email is already registered")
	case errors.Is(err, ErrInvalidCredentials):
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
	case errors.Is(err, ErrInactiveUser):
		httpx.WriteError(w, http.StatusForbidden, "account_inactive", "account is inactive")
	case errors.Is(err, ErrInvalidRefresh):
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_refresh",
			"refresh token is missing, invalid, or expired")
	default:
		httpx.LogWith(h.log, r).Error("auth handler internal error", "err", err)
		httpx.WriteInternalError(w, "authentication failed")
	}
}

func (h *Handler) setRefreshCookie(w http.ResponseWriter, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookie.Name,
		Value:    value,
		Path:     h.cookie.Path,
		HttpOnly: true,
		Secure:   h.cookie.Secure,
		SameSite: h.cookie.SameSite,
		MaxAge:   h.cookie.MaxAge,
	})
}

func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.cookie.Name,
		Value:    "",
		Path:     h.cookie.Path,
		HttpOnly: true,
		Secure:   h.cookie.Secure,
		SameSite: h.cookie.SameSite,
		MaxAge:   -1,
	})
}

func (h *Handler) readRefreshCookie(r *http.Request) string {
	c, err := r.Cookie(h.cookie.Name)
	if err != nil {
		return ""
	}
	return c.Value
}

func decodeJSON(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func requestMeta(r *http.Request) RequestMeta {
	meta := RequestMeta{}
	if ua := r.UserAgent(); ua != "" {
		meta.UserAgent = &ua
	}
	if addr := clientIP(r); addr != nil {
		meta.IP = addr
	}
	return meta
}

func clientIP(r *http.Request) *netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	return &addr
}
