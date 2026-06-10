//go:build integration

// Integration test for the Postgres road-graph repository. Excluded from the
// default build; run against a live database with:
//
//	go test -tags=integration ./internal/roadgraph/...
//
// Uses DATABASE_URL when set, otherwise the local docker-compose Postgres.
package roadgraph

import (
	"context"
	"os"
	"testing"
	"time"

	postgresplatform "github.com/tokiou/caba-inseguridad-routes-go/internal/platform/postgres"
)

const defaultTestDSN = "postgres://postgres:postgres@localhost:5434/caba_routes?sslmode=disable"

func newTestRepo(t *testing.T) *PostgresRepository {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = defaultTestDSN
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := postgresplatform.NewPool(ctx, dsn)
	if err != nil {
		t.Skipf("skipping: cannot reach Postgres at %s: %v", dsn, err)
	}
	t.Cleanup(pool.Close)
	return NewRepository(pool)
}

func TestPostgresRepositoryGetStats(t *testing.T) {
	repo := newTestRepo(t)

	stats, err := repo.GetStats(context.Background())
	if err != nil {
		t.Fatalf("GetStats: %v", err)
	}

	// Counts are always non-negative; walkable edges never exceed total edges.
	if stats.NodesCount < 0 || stats.EdgesCount < 0 || stats.WalkableEdges < 0 || stats.RiskScoredEdges < 0 {
		t.Fatalf("negative count in stats: %+v", stats)
	}
	if stats.WalkableEdges > stats.EdgesCount {
		t.Errorf("walkable edges %d exceed total edges %d", stats.WalkableEdges, stats.EdgesCount)
	}
	if stats.RoutableEdges > stats.EdgesCount {
		t.Errorf("routable edges %d exceed total edges %d", stats.RoutableEdges, stats.EdgesCount)
	}
	if stats.RoutableEdges+stats.ExcludedEdges != stats.EdgesCount {
		t.Errorf("routable %d + excluded %d != total edges %d", stats.RoutableEdges, stats.ExcludedEdges, stats.EdgesCount)
	}

	// Non-zero assertions only make sense once the OSM import has run; otherwise
	// this is a fresh schema and the graph is legitimately empty.
	if stats.NodesCount == 0 {
		t.Skip("road graph is empty — run scripts/osm import before asserting non-zero stats")
	}

	if stats.EdgesCount == 0 {
		t.Error("graph has nodes but no edges")
	}
	// After cleanup most edges remain routable; some may be excluded (>= 0).
	if stats.RoutableEdges == 0 {
		t.Error("graph has edges but none are routable")
	}
	if stats.ExcludedEdges < 0 {
		t.Errorf("excluded edges negative: %d", stats.ExcludedEdges)
	}
	// The CABA walkable graph must sit within the city's bounding box.
	if stats.MinLat < -34.75 || stats.MaxLat > -34.50 {
		t.Errorf("latitude bounds out of CABA: [%f, %f]", stats.MinLat, stats.MaxLat)
	}
	if stats.MinLng < -58.55 || stats.MaxLng > -58.33 {
		t.Errorf("longitude bounds out of CABA: [%f, %f]", stats.MinLng, stats.MaxLng)
	}
	t.Logf("graph stats: %+v", stats)
}
