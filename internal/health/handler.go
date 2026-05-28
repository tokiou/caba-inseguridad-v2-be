package health

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/httpx"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/health", h.Check)
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
