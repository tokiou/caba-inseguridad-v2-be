package crimes

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
	GetNearby(ctx context.Context, query NearbyCrimesQuery) (NearbyCrimesResponse, error)
}

type Handler struct {
	service service
	log     *slog.Logger
}

func NewHandler(svc service, log *slog.Logger) *Handler {
	return &Handler{service: svc, log: log}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/crimes/nearby", h.GetNearby)
}

func (h *Handler) GetNearby(w http.ResponseWriter, r *http.Request) {
	query, err := parseNearbyCrimesQuery(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	response, err := h.service.GetNearby(r.Context(), query)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}

func parseNearbyCrimesQuery(r *http.Request) (NearbyCrimesQuery, error) {
	values := r.URL.Query()

	latRaw := values.Get("lat")
	lngRaw := values.Get("lng")
	radiusRaw := values.Get("radius")
	limitRaw := values.Get("limit")
	cursorRaw := values.Get("cursor")

	if latRaw == "" || lngRaw == "" {
		return NearbyCrimesQuery{}, ErrInvalidCoordinates
	}

	lat, err := strconv.ParseFloat(latRaw, 64)
	if err != nil {
		return NearbyCrimesQuery{}, ErrInvalidCoordinates
	}

	lng, err := strconv.ParseFloat(lngRaw, 64)
	if err != nil {
		return NearbyCrimesQuery{}, ErrInvalidCoordinates
	}

	radius := 0
	if radiusRaw != "" {
		parsedRadius, err := strconv.Atoi(radiusRaw)
		if err != nil {
			return NearbyCrimesQuery{}, ErrInvalidRadius
		}
		radius = parsedRadius
	}

	limit := 0
	if limitRaw != "" {
		parsedLimit, err := strconv.Atoi(limitRaw)
		if err != nil {
			return NearbyCrimesQuery{}, ErrInvalidLimit
		}
		limit = parsedLimit
	}

	var cursor *Cursor
	if cursorRaw != "" {
		decoded, err := DecodeCursor(cursorRaw)
		if err != nil {
			return NearbyCrimesQuery{}, err
		}
		cursor = &decoded
	}

	return NearbyCrimesQuery{Lat: lat, Lng: lng, RadiusMeters: radius, Limit: limit, Cursor: cursor}, nil
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrInvalidCoordinates):
		httpx.WriteInvalidRequest(w, "lat and lng are required and must be valid CABA coordinates")
	case errors.Is(err, ErrInvalidRadius):
		httpx.WriteInvalidRequest(w, "radius must be between 1 and 2000 meters")
	case errors.Is(err, ErrInvalidLimit):
		httpx.WriteInvalidRequest(w, "limit must be between 1 and 500")
	case errors.Is(err, ErrInvalidCursor):
		httpx.WriteInvalidRequest(w, "cursor is not a valid pagination token")
	default:
		httpx.LogWith(h.log, r).Error("nearby crimes internal error", "err", err)
		httpx.WriteInternalError(w, "could not fetch nearby crimes")
	}
}
