package health

import (
	"net/http"

	"github.com/tokiou/caba-inseguridad-routes-go/internal/httpx"
)

type Handler struct{}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	response := map[string]string{
		"status": "ok",
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}
