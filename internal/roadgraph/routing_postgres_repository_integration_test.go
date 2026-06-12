//go:build integration

// Integration test for graph routing (pgr_dijkstra over routable_road_edges).
// Excluded from the default build; run against a live database with:
//
//	go test -tags=integration ./internal/roadgraph/...
//
// Uses DATABASE_URL when set, otherwise the local docker-compose Postgres.
package roadgraph

import (
	"context"
	"errors"
	"testing"
)

func TestPostgresRepositoryFindWalkRoute(t *testing.T) {
	repo := newTestRepo(t)

	// Skip when the graph is empty — routing only makes sense post-import.
	stats, err := repo.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.NodesCount == 0 {
		t.Skip("road graph is empty — run scripts/osm import before testing routing")
	}

	// Two points within CABA (Obelisco area → Palermo) that should be connected
	// on the walkable graph.
	query := WalkRouteQuery{
		FromLat: -34.6037, FromLng: -58.3816,
		ToLat: -34.5895, ToLng: -58.4201,
	}

	route, err := repo.FindWalkRoute(context.Background(), query)
	if err != nil {
		t.Fatalf("FindWalkRoute: %v", err)
	}

	if route.From.SnappedNodeID == 0 || route.To.SnappedNodeID == 0 {
		t.Errorf("endpoints not snapped to graph nodes: %+v", route)
	}
	if route.EdgeCount <= 0 {
		t.Errorf("expected a multi-edge path, got edge_count=%d", route.EdgeCount)
	}
	if route.DistanceMeters <= 0 || route.DurationSeconds <= 0 {
		t.Errorf("expected positive distance/duration, got %.1fm / %.1fs", route.DistanceMeters, route.DurationSeconds)
	}
	if route.Geometry.Type != "LineString" {
		t.Errorf("expected LineString geometry, got %q", route.Geometry.Type)
	}
	if len(route.Geometry.Coordinates) < 2 {
		t.Errorf("expected at least 2 coordinates, got %d", len(route.Geometry.Coordinates))
	}
	// Coordinates are [lng, lat] and must fall within CABA.
	for i, c := range route.Geometry.Coordinates {
		if len(c) != 2 {
			t.Fatalf("coordinate %d is not a [lng, lat] pair: %v", i, c)
		}
		lng, lat := c[0], c[1]
		if lng < -58.55 || lng > -58.33 || lat < -34.75 || lat > -34.50 {
			t.Errorf("coordinate %d out of CABA bounds: [%f, %f]", i, lng, lat)
		}
	}
	t.Logf("route: %d edges, %.0f m, %.0f s", route.EdgeCount, route.DistanceMeters, route.DurationSeconds)
}

func TestPostgresRepositoryFindWalkRoute_SamePoint(t *testing.T) {
	repo := newTestRepo(t)

	stats, err := repo.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}
	if stats.NodesCount == 0 {
		t.Skip("road graph is empty — run scripts/osm import before testing routing")
	}

	// Origin == destination snaps to a single node with no traversed edge → no route.
	query := WalkRouteQuery{FromLat: -34.6037, FromLng: -58.3816, ToLat: -34.6037, ToLng: -58.3816}
	if _, err := repo.FindWalkRoute(context.Background(), query); !errors.Is(err, ErrNoRoute) {
		t.Errorf("want ErrNoRoute for coincident endpoints, got %v", err)
	}
}
