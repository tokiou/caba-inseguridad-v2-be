package roadgraph

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/httpx"
)

type service interface {
	GetStats(ctx context.Context) (GraphStats, error)
	WalkRoute(ctx context.Context, query WalkRouteQuery) (WalkRoute, error)
}

type Handler struct {
	service   service
	rateLimit func(http.Handler) http.Handler
	log       *slog.Logger
}

func NewHandler(svc service, rateLimit func(http.Handler) http.Handler, log *slog.Logger) *Handler {
	return &Handler{service: svc, rateLimit: rateLimit, log: log}
}

func (h *Handler) Register(r chi.Router) {
	// Only the stats probe is rate limited; /roadgraph/route is left unbounded
	// in this iteration (it is not in the source spec's limit table).
	r.With(h.rateLimit).Get("/roadgraph/stats", h.GetStats)
	r.Get("/roadgraph/route", h.GetWalkRoute)
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

// GetWalkRoute returns the shortest walkable path between two CABA points computed
// over the local road graph.
func (h *Handler) GetWalkRoute(w http.ResponseWriter, r *http.Request) {
	query, err := parseWalkRouteQuery(r)
	if err != nil {
		httpx.WriteInvalidRequest(w, "from_lat, from_lng, to_lat and to_lng are required and must be valid CABA coordinates")
		return
	}

	route, err := h.service.WalkRoute(r.Context(), query)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCoordinates):
			httpx.WriteInvalidRequest(w, "endpoints must be distinct and within CABA")
		case errors.Is(err, ErrNoRoute):
			httpx.WriteError(w, http.StatusNotFound, "route_not_found", "no walkable route between the points")
		default:
			httpx.LogWith(h.log, r).Error("roadgraph route internal error", "err", err)
			httpx.WriteInternalError(w, "could not compute walkable route")
		}
		return
	}

	httpx.WriteJSON(w, http.StatusOK, route)
}

func parseWalkRouteQuery(r *http.Request) (WalkRouteQuery, error) {
	values := r.URL.Query()

	fromLat, err := strconv.ParseFloat(values.Get("from_lat"), 64)
	if err != nil {
		return WalkRouteQuery{}, ErrInvalidCoordinates
	}
	fromLng, err := strconv.ParseFloat(values.Get("from_lng"), 64)
	if err != nil {
		return WalkRouteQuery{}, ErrInvalidCoordinates
	}
	toLat, err := strconv.ParseFloat(values.Get("to_lat"), 64)
	if err != nil {
		return WalkRouteQuery{}, ErrInvalidCoordinates
	}
	toLng, err := strconv.ParseFloat(values.Get("to_lng"), 64)
	if err != nil {
		return WalkRouteQuery{}, ErrInvalidCoordinates
	}

	return WalkRouteQuery{FromLat: fromLat, FromLng: fromLng, ToLat: toLat, ToLng: toLng}, nil
}
