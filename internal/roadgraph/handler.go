package roadgraph

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/httpx"
)

type service interface {
	GetStats(ctx context.Context) (GraphStats, error)
}

type Handler struct {
	service service
	log     *slog.Logger
}

func NewHandler(svc service, log *slog.Logger) *Handler {
	return &Handler{service: svc, log: log}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/roadgraph/stats", h.GetStats)
}

// GetStats is a read-only dev/admin probe of the walkable graph import state.
func (h *Handler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.GetStats(r.Context())
	if err != nil {
		httpx.LogWith(h.log, r).Error("roadgraph stats internal error", "err", err)
		httpx.WriteInternalError(w, "could not fetch road graph stats")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, stats)
}
