package roadgraph

import (
	"context"
	"errors"
	"testing"
)

func TestServiceWalkRoute(t *testing.T) {
	validRoute := WalkRoute{
		From:            RoutePoint{Lat: -34.6037, Lng: -58.3816, SnappedNodeID: 10},
		To:              RoutePoint{Lat: -34.5895, Lng: -58.4201, SnappedNodeID: 20},
		DistanceMeters:  1234.5,
		DurationSeconds: 882.0,
		EdgeCount:       7,
		Geometry:        GeoJSONLineString{Type: "LineString", Coordinates: [][]float64{{-58.3816, -34.6037}, {-58.4201, -34.5895}}},
	}

	t.Run("returns repository route for valid distinct CABA endpoints", func(t *testing.T) {
		repo := &fakeRepository{route: validRoute}
		svc := NewService(repo)

		query := WalkRouteQuery{FromLat: -34.6037, FromLng: -58.3816, ToLat: -34.5895, ToLng: -58.4201}
		got, err := svc.WalkRoute(context.Background(), query)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.EdgeCount != validRoute.EdgeCount || got.DistanceMeters != validRoute.DistanceMeters {
			t.Errorf("want %+v, got %+v", validRoute, got)
		}
		if repo.gotQuery != query {
			t.Errorf("repository got query %+v, want %+v", repo.gotQuery, query)
		}
	})

	t.Run("rejects out-of-CABA endpoints without hitting the repository", func(t *testing.T) {
		repo := &fakeRepository{route: validRoute}
		svc := NewService(repo)

		// Origin in Montevideo, destination in CABA.
		query := WalkRouteQuery{FromLat: -34.9011, FromLng: -56.1645, ToLat: -34.5895, ToLng: -58.4201}
		_, err := svc.WalkRoute(context.Background(), query)
		if !errors.Is(err, ErrInvalidCoordinates) {
			t.Fatalf("want ErrInvalidCoordinates, got %v", err)
		}
		if repo.routeCalls != 0 {
			t.Errorf("repository should not be called on invalid input, got %d calls", repo.routeCalls)
		}
	})

	t.Run("rejects identical origin and destination", func(t *testing.T) {
		svc := NewService(&fakeRepository{route: validRoute})

		query := WalkRouteQuery{FromLat: -34.6037, FromLng: -58.3816, ToLat: -34.6037, ToLng: -58.3816}
		if _, err := svc.WalkRoute(context.Background(), query); !errors.Is(err, ErrInvalidCoordinates) {
			t.Fatalf("want ErrInvalidCoordinates, got %v", err)
		}
	})

	t.Run("propagates ErrNoRoute from the repository", func(t *testing.T) {
		svc := NewService(&fakeRepository{routeErr: ErrNoRoute})

		query := WalkRouteQuery{FromLat: -34.6037, FromLng: -58.3816, ToLat: -34.5895, ToLng: -58.4201}
		if _, err := svc.WalkRoute(context.Background(), query); !errors.Is(err, ErrNoRoute) {
			t.Fatalf("want ErrNoRoute, got %v", err)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		svc := NewService(&fakeRepository{routeErr: errors.New("db unavailable")})

		query := WalkRouteQuery{FromLat: -34.6037, FromLng: -58.3816, ToLat: -34.5895, ToLng: -58.4201}
		if _, err := svc.WalkRoute(context.Background(), query); err == nil {
			t.Fatal("want error, got nil")
		}
	})
}
