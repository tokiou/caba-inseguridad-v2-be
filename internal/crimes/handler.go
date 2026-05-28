package crimes

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/tokiou/caba-inseguridad-routes-go/internal/httpx"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) GetNearby(w http.ResponseWriter, r *http.Request) {
	query, err := parseNearbyCrimesQuery(r)
	if err != nil {
		writeNearbyError(w, err)
		return
	}

	response, err := h.service.GetNearby(r.Context(), query)
	if err != nil {
		writeNearbyError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, response)
}

func parseNearbyCrimesQuery(r *http.Request) (NearbyCrimesQuery, error) {
	values := r.URL.Query()

	latRaw := values.Get("lat")
	lngRaw := values.Get("lng")
	radiusRaw := values.Get("radius")

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

	return NearbyCrimesQuery{
		Lat:          lat,
		Lng:          lng,
		RadiusMeters: radius,
	}, nil
}

func writeNearbyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidCoordinates):
		httpx.WriteInvalidRequest(
			w,
			"lat and lng are required and must be valid CABA coordinates",
		)

	case errors.Is(err, ErrInvalidRadius):
		httpx.WriteInvalidRequest(
			w,
			"radius must be between 1 and 2000 meters",
		)

	default:
		log.Printf("nearby crimes internal error: %v", err)
		httpx.WriteInternalError(
			w,
			"could not fetch nearby crimes",
		)
	}
}
