package routes

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// roundTripFunc lets a test stand in for the ORS HTTP transport.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newTestClient(status int, body string) *ORSClient {
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
	return NewORSClient("test-key", "https://ors.example", &http.Client{Transport: transport})
}

func TestORSClientGetRoute_StatusMapping(t *testing.T) {
	const okBody = `{"features":[{"geometry":{"type":"LineString","coordinates":[[-58.38,-34.60],[-58.42,-34.59]]},"properties":{"summary":{"distance":3241.5,"duration":478.2}}}]}`

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr error // nil means success
	}{
		{name: "200 with route succeeds", status: http.StatusOK, body: okBody, wantErr: nil},
		{name: "200 with no features is not found", status: http.StatusOK, body: `{"features":[]}`, wantErr: ErrRouteNotFound},
		{name: "404 is route not found", status: http.StatusNotFound, body: `{"error":"no route"}`, wantErr: ErrRouteNotFound},
		{name: "401 is external service", status: http.StatusUnauthorized, body: `{"error":"bad key"}`, wantErr: ErrExternalService},
		{name: "403 is external service", status: http.StatusForbidden, body: `{"error":"Access to this API has been disallowed"}`, wantErr: ErrExternalService},
		{name: "429 is external service", status: http.StatusTooManyRequests, body: `{"error":"rate limited"}`, wantErr: ErrExternalService},
		{name: "400 is external service", status: http.StatusBadRequest, body: `{"error":"bad request"}`, wantErr: ErrExternalService},
		{name: "500 is external service", status: http.StatusInternalServerError, body: `oops`, wantErr: ErrExternalService},
	}

	query := RouteQuery{OriginLat: -34.6037, OriginLng: -58.3816, DestLat: -34.5895, DestLng: -58.4201, Profile: "driving-car"}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(tt.status, tt.body)

			route, err := client.GetRoute(context.Background(), query)

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected success, got error: %v", err)
				}
				if route.Distance != 3241.5 {
					t.Errorf("expected distance 3241.5, got %v", route.Distance)
				}
				return
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}
