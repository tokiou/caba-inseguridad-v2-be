package routes

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeService struct {
	resp RouteResponse
	err  error
}

func (s *fakeService) GetRoute(_ context.Context, _ RouteQuery) (RouteResponse, error) {
	return s.resp, s.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHandlerGetRoute(t *testing.T) {
	validURL := "/api/v1/routes?origin_lat=-34.6037&origin_lng=-58.3816&dest_lat=-34.5895&dest_lng=-58.4201"

	fakeResp := RouteResponse{
		Origin:      Waypoint{Lat: -34.6037, Lng: -58.3816},
		Destination: Waypoint{Lat: -34.5895, Lng: -58.4201},
		Profile:     "driving-car",
		Distance:    3241.5,
		Duration:    478.2,
		Geometry:    GeoJSONLineString{Type: "LineString", Coordinates: [][]float64{{-58.3816, -34.6037}}},
	}

	tests := []struct {
		name       string
		url        string
		svc        *fakeService
		wantStatus int
		wantBody   string
	}{
		{
			name:       "400 when origin_lat is missing",
			url:        "/api/v1/routes?origin_lng=-58.3816&dest_lat=-34.5895&dest_lng=-58.4201",
			svc:        &fakeService{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid_request",
		},
		{
			name:       "400 when origin_lng is unparseable",
			url:        "/api/v1/routes?origin_lat=-34.6037&origin_lng=abc&dest_lat=-34.5895&dest_lng=-58.4201",
			svc:        &fakeService{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid_request",
		},
		{
			name:       "400 when dest_lat is missing",
			url:        "/api/v1/routes?origin_lat=-34.6037&origin_lng=-58.3816&dest_lng=-58.4201",
			svc:        &fakeService{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid_request",
		},
		{
			name:       "400 when service returns ErrInvalidCoordinates",
			url:        validURL,
			svc:        &fakeService{err: ErrInvalidCoordinates},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid_request",
		},
		{
			name:       "400 when service returns ErrSamePoint",
			url:        validURL,
			svc:        &fakeService{err: ErrSamePoint},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid_request",
		},
		{
			name:       "400 when service returns ErrInvalidProfile",
			url:        validURL + "&profile=rocket",
			svc:        &fakeService{err: ErrInvalidProfile},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid_request",
		},
		{
			name:       "404 when service returns ErrRouteNotFound",
			url:        validURL,
			svc:        &fakeService{err: ErrRouteNotFound},
			wantStatus: http.StatusNotFound,
			wantBody:   "route_not_found",
		},
		{
			name:       "502 when service returns ErrExternalService",
			url:        validURL,
			svc:        &fakeService{err: ErrExternalService},
			wantStatus: http.StatusBadGateway,
			wantBody:   "external_service_error",
		},
		{
			name:       "500 when service returns unexpected error",
			url:        validURL,
			svc:        &fakeService{err: io.ErrUnexpectedEOF},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "could not calculate route",
		},
		{
			name:       "200 for valid request",
			url:        validURL,
			svc:        &fakeService{resp: fakeResp},
			wantStatus: http.StatusOK,
			wantBody:   `"profile":"driving-car"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(tt.svc, discardLogger())
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			handler.GetRoute(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("want status %d, got %d — body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("want body to contain %q, got: %s", tt.wantBody, rec.Body.String())
			}
		})
	}
}
