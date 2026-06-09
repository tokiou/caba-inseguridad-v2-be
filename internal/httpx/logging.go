package httpx

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// LogWith returns log enriched with the request's correlation id, so that
// error logs emitted by handlers carry the same request_id as the request
// log line produced by the logging middleware.
func LogWith(log *slog.Logger, r *http.Request) *slog.Logger {
	if id := middleware.GetReqID(r.Context()); id != "" {
		return log.With("request_id", id)
	}
	return log
}
