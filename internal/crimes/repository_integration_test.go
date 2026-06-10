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

func TestPostgresRepositoryFindNearby(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	const limit = 500
	page, err := repo.FindNearby(ctx, NearbyCrimesQuery{
		Lat:          obeliscoLat,
		Lng:          obeliscoLng,
		RadiusMeters: 300,
		Limit:        limit,
	})
	if err != nil {
		t.Fatalf("FindNearby: %v", err)
	}

	if len(page.Items) == 0 {
		t.Fatal("expected nearby crimes around the Obelisco, got none")
	}
	if len(page.Items) > limit {
		t.Fatalf("page returned %d items, exceeds limit %d", len(page.Items), limit)
	}
	// The Obelisco has far more than 500 crimes within 300m, so there must be more.
	if !page.HasMore || page.Next == nil {
		t.Fatalf("expected has_more with a cursor, got has_more=%v next=%v", page.HasMore, page.Next)
	}

	const orderingToleranceM = 5 // spherical haversine vs PostGIS ellipsoidal distance
	var maxDist float64
	for i, c := range page.Items {
		if len(c.Location.Coordinates) != 2 {
			t.Fatalf("item %d: want 2 coordinates, got %d", i, len(c.Location.Coordinates))
		}
		lng, lat := c.Location.Coordinates[0], c.Location.Coordinates[1]
		if lat < -35 || lat > -34 {
			t.Errorf("item %d: latitude %f out of CABA bounds", i, lat)
		}
		if lng < -59 || lng > -58 {
			t.Errorf("item %d: longitude %f out of CABA bounds", i, lng)
		}
		dist := haversineMeters(obeliscoLat, obeliscoLng, lat, lng)
		if dist < maxDist-orderingToleranceM {
			t.Errorf("item %d not ordered by distance: %.2fm below prior max %.2fm", i, dist, maxDist)
		}
		if dist > maxDist {
			maxDist = dist
		}
		if len(c.Date) != 10 {
			t.Errorf("item %d: date %q is not YYYY-MM-DD", i, c.Date)
		}
	}
}

// TestPostgresRepositoryFindNearbyPagination walks every page via the cursor and
// proves the keyset is correct: no duplicates, no gaps, distances never go
// backwards across page boundaries, and the paginated walk yields exactly the
// same set as a single unpaginated query.
func TestPostgresRepositoryFindNearbyPagination(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	base := NearbyCrimesQuery{Lat: obeliscoLat, Lng: obeliscoLng, RadiusMeters: 300}

	// Reference: one big page with everything (no further pages).
	all, err := repo.FindNearby(ctx, withLimit(base, 50000, nil))
	if err != nil {
		t.Fatalf("reference query: %v", err)
	}
	if all.HasMore {
		t.Fatal("reference limit too small; increase it")
	}
	reference := make(map[string]bool, len(all.Items))
	for _, c := range all.Items {
		reference[c.SourceID] = true
	}
	t.Logf("reference set: %d crimes within 300m", len(reference))

	// Walk all pages with a small limit.
	const pageSize = 250
	const orderingToleranceM = 5
	seen := make(map[string]bool, len(reference))
	var cursor *Cursor
	var maxDist float64
	pages := 0

	for {
		page, err := repo.FindNearby(ctx, withLimit(base, pageSize, cursor))
		if err != nil {
			t.Fatalf("page %d: %v", pages, err)
		}
		if len(page.Items) > pageSize {
			t.Fatalf("page %d returned %d items, exceeds page size %d", pages, len(page.Items), pageSize)
		}
		for _, c := range page.Items {
			if seen[c.SourceID] {
				t.Errorf("duplicate source_id %q across pages", c.SourceID)
			}
			seen[c.SourceID] = true

			lng, lat := c.Location.Coordinates[0], c.Location.Coordinates[1]
			dist := haversineMeters(obeliscoLat, obeliscoLng, lat, lng)
			if dist < maxDist-orderingToleranceM {
				t.Errorf("distance went backwards across pages: %.2fm below prior max %.2fm", dist, maxDist)
			}
			if dist > maxDist {
				maxDist = dist
			}
		}
		pages++
		if !page.HasMore {
			if page.Next != nil {
				t.Error("last page must not carry a cursor")
			}
			break
		}
		if page.Next == nil {
			t.Fatalf("page %d has_more but no cursor", pages)
		}
		cursor = page.Next
		if pages > 10000 {
			t.Fatal("pagination did not terminate")
		}
	}

	t.Logf("walked %d pages, %d crimes", pages, len(seen))
	if len(seen) != len(reference) {
		t.Errorf("paginated walk saw %d crimes, reference has %d", len(seen), len(reference))
	}
	for id := range reference {
		if !seen[id] {
			t.Errorf("paginated walk missed source_id %q", id)
		}
	}
}

func withLimit(q NearbyCrimesQuery, limit int, cursor *Cursor) NearbyCrimesQuery {
	q.Limit = limit
	q.Cursor = cursor
	return q
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
