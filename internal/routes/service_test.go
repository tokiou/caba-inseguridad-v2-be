package routes

import (
	"context"
	"errors"
	"testing"
)

type fakeClient struct {
	route         Route
	err           error
	receivedQuery RouteQuery
}

func (c *fakeClient) FetchRoute(_ context.Context, query RouteQuery) (Route, error) {
	c.receivedQuery = query
	return c.route, c.err
}

func TestServiceGetRoute(t *testing.T) {
	validQuery := RouteQuery{
		OriginLat: -34.6037,
		OriginLng: -58.3816,
		DestLat:   -34.5895,
		DestLng:   -58.4201,
	}

	fakeRoute := Route{
		Distance: 3241.5,
		Duration: 478.2,
		Geometry: GeoJSONLineString{
			Type:        "LineString",
			Coordinates: [][]float64{{-58.3816, -34.6037}, {-58.4201, -34.5895}},
		},
	}

	tests := []struct {
		name        string
		client      *fakeClient
		query       RouteQuery
		wantErr     error
		wantProfile string
	}{
		{
			name:    "error for origin outside CABA",
			client:  &fakeClient{},
			query:   RouteQuery{OriginLat: -33.0, OriginLng: -58.38, DestLat: -34.5895, DestLng: -58.4201},
			wantErr: ErrInvalidCoordinates,
		},
		{
			name:    "error for destination outside CABA",
			client:  &fakeClient{},
			query:   RouteQuery{OriginLat: -34.6037, OriginLng: -58.3816, DestLat: -33.0, DestLng: -58.4201},
			wantErr: ErrInvalidCoordinates,
		},
		{
			name:    "error for same origin and destination",
			client:  &fakeClient{},
			query:   RouteQuery{OriginLat: -34.6037, OriginLng: -58.3816, DestLat: -34.6037, DestLng: -58.3816},
			wantErr: ErrSamePoint,
		},
		{
			name:    "error for invalid profile",
			client:  &fakeClient{},
			query:   RouteQuery{OriginLat: -34.6037, OriginLng: -58.3816, DestLat: -34.5895, DestLng: -58.4201, Profile: "rocket"},
			wantErr: ErrInvalidProfile,
		},
		{
			name:        "applies default profile when empty",
			client:      &fakeClient{route: fakeRoute},
			query:       validQuery,
			wantProfile: DefaultProfile,
		},
		{
			name:   "returns correct route response",
			client: &fakeClient{route: fakeRoute},
			query:  RouteQuery{OriginLat: -34.6037, OriginLng: -58.3816, DestLat: -34.5895, DestLng: -58.4201, Profile: "foot-walking"},
		},
		{
			name:    "propagates ErrRouteNotFound from client",
			client:  &fakeClient{err: ErrRouteNotFound},
			query:   validQuery,
			wantErr: ErrRouteNotFound,
		},
		{
			name:    "propagates ErrExternalService from client",
			client:  &fakeClient{err: ErrExternalService},
			query:   validQuery,
			wantErr: ErrExternalService,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(tt.client)

			resp, err := svc.GetRoute(context.Background(), tt.query)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("want error %v, got nil", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantProfile != "" && tt.client.receivedQuery.Profile != tt.wantProfile {
				t.Errorf("want profile %q sent to client, got %q", tt.wantProfile, tt.client.receivedQuery.Profile)
			}
			_ = resp
		})
	}
}
