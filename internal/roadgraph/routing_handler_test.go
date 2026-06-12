package roadgraph

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerGetWalkRoute(t *testing.T) {
	validRoute := WalkRoute{
		From:            RoutePoint{Lat: -34.6037, Lng: -58.3816, SnappedNodeID: 10},
		To:              RoutePoint{Lat: -34.5895, Lng: -58.4201, SnappedNodeID: 20},
		DistanceMeters:  1234.5,
		DurationSeconds: 882.0,
		EdgeCount:       7,
		Geometry:        GeoJSONLineString{Type: "LineString", Coordinates: [][]float64{{-58.3816, -34.6037}, {-58.4201, -34.5895}}},
	}
	validURL := "/api/v1/roadgraph/route?from_lat=-34.6037&from_lng=-58.3816&to_lat=-34.5895&to_lng=-58.4201"

	tests := []struct {
		name       string
		url        string
		svc        *fakeService
		wantStatus int
		wantBody   string
	}{
		{
			name:       "200 with route geometry",
			url:        validURL,
			svc:        &fakeService{route: validRoute},
			wantStatus: http.StatusOK,
			wantBody:   `"type":"LineString"`,
		},
		{
			name:       "200 exposes snapped node ids",
			url:        validURL,
			svc:        &fakeService{route: validRoute},
			wantStatus: http.StatusOK,
			wantBody:   `"snapped_node_id":10`,
		},
		{
			name:       "400 on missing param",
			url:        "/api/v1/roadgraph/route?from_lat=-34.6037&from_lng=-58.3816&to_lng=-58.4201",
			svc:        &fakeService{route: validRoute},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid_request",
		},
		{
			name:       "400 on non-numeric coordinate",
			url:        "/api/v1/roadgraph/route?from_lat=abc&from_lng=-58.3816&to_lat=-34.5895&to_lng=-58.4201",
			svc:        &fakeService{route: validRoute},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid_request",
		},
		{
			name:       "400 when service rejects coordinates",
			url:        validURL,
			svc:        &fakeService{routeErr: ErrInvalidCoordinates},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid_request",
		},
		{
			name:       "404 when no route exists",
			url:        validURL,
			svc:        &fakeService{routeErr: ErrNoRoute},
			wantStatus: http.StatusNotFound,
			wantBody:   "route_not_found",
		},
		{
			name:       "500 when service errors",
			url:        validURL,
			svc:        &fakeService{routeErr: errors.New("db unavailable")},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "could not compute walkable route",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(tt.svc, discardLogger())
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			handler.GetWalkRoute(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("want status %d, got %d — body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("want body to contain %q, got: %s", tt.wantBody, rec.Body.String())
			}
		})
	}
}
