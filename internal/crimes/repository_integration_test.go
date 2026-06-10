//go:build integration

// Integration tests for the Postgres + PostGIS repository. Excluded from the
// default build; run against a live database with:
//
//	go test -tags=integration ./internal/crimes/...
//
// Uses DATABASE_URL when set, otherwise the local docker-compose Postgres.
package crimes

import (
	"context"
	"math"
	"os"
	"testing"
	"time"

	postgresplatform "github.com/tokiou/caba-inseguridad-routes-go/internal/platform/postgres"
)

const defaultTestDSN = "postgres://postgres:postgres@localhost:5434/caba_routes?sslmode=disable"

// Obelisco — a dense central CABA point with many nearby crimes.
const (
	obeliscoLat = -34.6037
	obeliscoLng = -58.3816
)

func TestPostgresRepositoryFindNearby(t *testing.T) {
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
	defer pool.Close()

	repo := NewRepository(pool)

	items, err := repo.FindNearby(ctx, NearbyCrimesQuery{
		Lat:          obeliscoLat,
		Lng:          obeliscoLng,
		RadiusMeters: 300,
	})
	if err != nil {
		t.Fatalf("FindNearby: %v", err)
	}

	if len(items) == 0 {
		t.Fatal("expected nearby crimes around the Obelisco, got none")
	}
	t.Logf("found %d crimes within 300m of the Obelisco", len(items))

	// PostGIS orders by ellipsoidal ST_Distance(geography); our verification
	// haversine is spherical, so the two disagree by under ~1m at this range.
	// A small tolerance absorbs that while still catching any real ordering bug
	// (which would be off by tens or hundreds of meters).
	const orderingToleranceM = 5
	var maxDist float64
	for i, c := range items {
		if len(c.Location.Coordinates) != 2 {
			t.Fatalf("item %d: want 2 coordinates, got %d", i, len(c.Location.Coordinates))
		}
		lng, lat := c.Location.Coordinates[0], c.Location.Coordinates[1]

		// ST_X = longitude (~ -58.x), ST_Y = latitude (~ -34.x) — not swapped.
		if lat < -35 || lat > -34 {
			t.Errorf("item %d: latitude %f out of CABA bounds", i, lat)
		}
		if lng < -59 || lng > -58 {
			t.Errorf("item %d: longitude %f out of CABA bounds", i, lng)
		}

		// Results must come back nearest-first.
		dist := haversineMeters(obeliscoLat, obeliscoLng, lat, lng)
		if dist < maxDist-orderingToleranceM {
			t.Errorf("item %d not ordered by distance: %.2fm well below prior max %.2fm", i, dist, maxDist)
		}
		if dist > maxDist {
			maxDist = dist
		}

		if dist > 400 { // 300m radius + slack for great-circle vs PostGIS geography
			t.Errorf("item %d distance %.2fm exceeds the 300m radius", i, dist)
		}

		if len(c.Date) != 10 {
			t.Errorf("item %d: date %q is not YYYY-MM-DD", i, c.Date)
		}
	}
}

func haversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadiusM = 6371000.0
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
