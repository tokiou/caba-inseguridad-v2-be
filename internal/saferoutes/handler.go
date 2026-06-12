package saferoutes

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/httpx"
)

type service interface {
	SafeRoutes(ctx context.Context, query SafeRoutesQuery) (SafeRoutesResponse, error)
}

type Handler struct {
	service service
	log     *slog.Logger
}

func NewHandler(svc service, log *slog.Logger) *Handler {
	return &Handler{service: svc, log: log}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/routes/safe", h.GetSafeRoutes)
}

// GetSafeRoutes returns the fastest / balanced / safest / least-safe-candidate
// walking routes between two CABA points for a given moment.
func (h *Handler) GetSafeRoutes(w http.ResponseWriter, r *http.Request) {
	query, err := parseSafeRoutesQuery(r)
	if err != nil {
		httpx.WriteInvalidRequest(w,
			"origin_lat, origin_lng, dest_lat and dest_lng are required CABA coordinates; datetime, if present, must be RFC3339")
		return
	}

	response, err := h.service.SafeRoutes(r.Context(), query)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCoordinates):
			httpx.WriteInvalidRequest(w, "endpoints must be distinct and within CABA")
		case errors.Is(err, ErrPointOutsideGraph):
			httpx.WriteError(w, http.StatusBadRequest,
				"origin_or_destination_outside_walkable_graph",
				"origin or destination is too far from the walkable road network")
		case errors.Is(err, ErrNoRoute):
			httpx.WriteError(w, http.StatusNotFound, "route_not_found",
				"no walkable route between the points")
		case errors.Is(err, ErrNoActiveModel):
			httpx.WriteError(w, http.StatusServiceUnavailable, "risk_model_unavailable",
				"no active risk model is available")
		default:
			httpx.LogWith(h.log, r).Error("safe routes internal error", "err", err)
			httpx.WriteInternalError(w, "could not compute safe routes")
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}

func parseSafeRoutesQuery(r *http.Request) (SafeRoutesQuery, error) {
	values := r.URL.Query()

	originLat, err := strconv.ParseFloat(values.Get("origin_lat"), 64)
	if err != nil {
		return SafeRoutesQuery{}, ErrInvalidCoordinates
	}
	originLng, err := strconv.ParseFloat(values.Get("origin_lng"), 64)
	if err != nil {
		return SafeRoutesQuery{}, ErrInvalidCoordinates
	}
	destLat, err := strconv.ParseFloat(values.Get("dest_lat"), 64)
	if err != nil {
		return SafeRoutesQuery{}, ErrInvalidCoordinates
	}
	destLng, err := strconv.ParseFloat(values.Get("dest_lng"), 64)
	if err != nil {
		return SafeRoutesQuery{}, ErrInvalidCoordinates
	}

	query := SafeRoutesQuery{
		OriginLat: originLat,
		OriginLng: originLng,
		DestLat:   destLat,
		DestLng:   destLng,
	}

	if raw := values.Get("datetime"); raw != "" {
		at, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return SafeRoutesQuery{}, ErrInvalidCoordinates
		}
		query.At = at
	}
	return query, nil
}
