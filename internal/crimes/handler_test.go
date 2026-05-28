package crimes

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
	resp NearbyCrimesResponse
	err  error
}

func (s *fakeService) GetNearby(_ context.Context, _ NearbyCrimesQuery) (NearbyCrimesResponse, error) {
	return s.resp, s.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestHandlerGetNearby(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		svc        *fakeService
		wantStatus int
		wantBody   string
	}{
		{
			name:       "400 when lat is missing",
			url:        "/api/v1/crimes/nearby?lng=-58.4201&radius=300",
			svc:        &fakeService{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "lat and lng are required",
		},
		{
			name:       "400 when lng is missing",
			url:        "/api/v1/crimes/nearby?lat=-34.5895&radius=300",
			svc:        &fakeService{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "lat and lng are required",
		},
		{
			name:       "400 when radius is not a number",
			url:        "/api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=abc",
			svc:        &fakeService{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "radius must be between 1 and 2000 meters",
		},
		{
			name:       "200 for valid request",
			url:        "/api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=300",
			svc:        &fakeService{resp: NearbyCrimesResponse{RadiusMeters: 300, Items: []Crime{}}},
			wantStatus: http.StatusOK,
			wantBody:   `"radius_meters":300`,
		},
		{
			name:       "500 when service returns unexpected error",
			url:        "/api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=300",
			svc:        &fakeService{err: io.ErrUnexpectedEOF},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "could not fetch nearby crimes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewHandler(tt.svc, discardLogger())
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			handler.GetNearby(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("want status %d, got %d — body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("want body to contain %q, got: %s", tt.wantBody, rec.Body.String())
			}
		})
	}
}
