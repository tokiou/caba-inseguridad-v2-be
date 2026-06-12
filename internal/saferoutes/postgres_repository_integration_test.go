//go:build integration

package saferoutes

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Requires: live PostGIS with the imported graph, applied migrations 000008+,
// and an active risk model with populated scores (run the Python pipeline).
//   go test -tags=integration ./internal/saferoutes/...

func integrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://postgres:postgres@localhost:5434/caba_routes?sslmode=disable"
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestSafeRoutesIntegration(t *testing.T) {
	repo := NewRepository(integrationPool(t))
	service := NewService(repo)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	response, err := service.SafeRoutes(ctx, SafeRoutesQuery{
		// Palermo -> Balvanera, well inside the walkable graph.
		OriginLat: -34.58, OriginLng: -58.42,
		DestLat: -34.60, DestLng: -58.40,
		At: time.Date(2026, 6, 12, 23, 0, 0, 0, time.FixedZone("-03", -3*3600)),
	})
	if err != nil {
		t.Fatalf("SafeRoutes: %v", err)
	}

	if response.TimeBucket != "night" || response.WeekdayType != "weekday" {
		t.Fatalf("context = %s/%s, want night/weekday", response.TimeBucket, response.WeekdayType)
	}
	if len(response.Routes) < 3 {
		t.Fatalf("routes = %d, want at least fastest/balanced/safest", len(response.Routes))
	}

	byKind := map[string]SafeRoute{}
	for _, route := range response.Routes {
		byKind[route.Kind] = route
		if route.DistanceMeters <= 0 || route.DurationMinutes <= 0 {
			t.Fatalf("route %s has no distance/duration: %+v", route.Kind, route)
		}
		if route.RiskScore < 0 || route.RiskScore > 1 {
			t.Fatalf("route %s risk out of [0,1]: %v", route.Kind, route.RiskScore)
		}
		if route.Geometry.Type != "LineString" || len(route.Geometry.Coordinates) < 2 {
			t.Fatalf("route %s has invalid geometry", route.Kind)
		}
	}

	fastest, safest := byKind["fastest"], byKind["safest"]
	if safest.RiskScore > fastest.RiskScore {
		t.Fatalf("safest risk %v exceeds fastest %v", safest.RiskScore, fastest.RiskScore)
	}
	if safest.DistanceMeters < fastest.DistanceMeters {
		t.Fatalf("safest distance %v shorter than fastest %v", safest.DistanceMeters, fastest.DistanceMeters)
	}
	if leastSafe, ok := byKind["least_safe_candidate"]; ok {
		if leastSafe.DistanceMeters > fastest.DistanceMeters*1.75 {
			t.Fatalf("least safe candidate detour %v exceeds 1.75x fastest %v",
				leastSafe.DistanceMeters, fastest.DistanceMeters)
		}
	}
}

func TestActiveModelIntegration(t *testing.T) {
	repo := NewRepository(integrationPool(t))
	model, err := repo.ActiveModel(context.Background())
	if err != nil {
		t.Fatalf("ActiveModel: %v", err)
	}
	if model.Name == "" || model.LowMax <= 0 || model.ModerateMax <= model.LowMax {
		t.Fatalf("suspicious model row: %+v", model)
	}
}
