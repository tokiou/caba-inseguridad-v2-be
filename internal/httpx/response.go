package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(data); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

func WriteError(w http.ResponseWriter, status int, code string, message string) {
	WriteJSON(w, status, ErrorResponse{Error: code, Message: message})
}

func WriteInvalidRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, "invalid_request", message)
}

func WriteInternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, "internal_error", message)
}
