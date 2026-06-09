package routes

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
	GetRoute(ctx context.Context, query RouteQuery) (RouteResponse, error)
}

type Handler struct {
	service service
	log     *slog.Logger
}

func NewHandler(svc service, log *slog.Logger) *Handler {
	return &Handler{service: svc, log: log}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/routes", h.GetRoute)
}

func (h *Handler) GetRoute(w http.ResponseWriter, r *http.Request) {
	query, err := parseRouteQuery(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	response, err := h.service.GetRoute(r.Context(), query)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}

func parseRouteQuery(r *http.Request) (RouteQuery, error) {
	values := r.URL.Query()

	originLat, err := parseRequiredFloat(values.Get("origin_lat"))
	if err != nil {
		return RouteQuery{}, ErrInvalidCoordinates
	}
	originLng, err := parseRequiredFloat(values.Get("origin_lng"))
	if err != nil {
		return RouteQuery{}, ErrInvalidCoordinates
	}
	destLat, err := parseRequiredFloat(values.Get("dest_lat"))
	if err != nil {
		return RouteQuery{}, ErrInvalidCoordinates
	}
	destLng, err := parseRequiredFloat(values.Get("dest_lng"))
	if err != nil {
		return RouteQuery{}, ErrInvalidCoordinates
	}

	return RouteQuery{
		OriginLat: originLat,
		OriginLng: originLng,
		DestLat:   destLat,
		DestLng:   destLng,
		Profile:   values.Get("profile"),
	}, nil
}

func parseRequiredFloat(s string) (float64, error) {
	if s == "" {
		return 0, ErrInvalidCoordinates
	}
	return strconv.ParseFloat(s, 64)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidCoordinates):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "origin and destination must be valid CABA coordinates")
	case errors.Is(err, ErrSamePoint):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "origin and destination must not be the same point")
	case errors.Is(err, ErrInvalidProfile):
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "profile must be one of: driving-car, foot-walking, cycling-regular")
	case errors.Is(err, ErrRouteNotFound):
		httpx.WriteError(w, http.StatusNotFound, "route_not_found", "no route found between the given points")
	case errors.Is(err, ErrExternalService):
		httpx.LogWith(h.log, r).Error("routes external service error", "err", err)
		httpx.WriteError(w, http.StatusBadGateway, "external_service_error", "route service is temporarily unavailable")
	default:
		httpx.LogWith(h.log, r).Error("routes internal error", "err", err)
		httpx.WriteInternalError(w, "could not calculate route")
	}
}
