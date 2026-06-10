package crimes

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	FindNearby(ctx context.Context, query NearbyCrimesQuery) (CrimePage, error)
}

// CrimePage is one keyset page of nearby crimes. Next is the cursor for the
// following page, or nil when HasMore is false.
type CrimePage struct {
	Items   []Crime
	HasMore bool
	Next    *Cursor
}

// findNearbyQuery returns crimes within radius meters of the point, ordered by
// (distance, id). Parameters: $1 = longitude, $2 = latitude, $3 = radius
// (meters), $4 = cursor distance (nullable), $5 = cursor id (nullable),
// $6 = row limit. Coordinates are [lng, lat] — never swapped. Distance is
// computed once in the inner query so the keyset predicate can reuse it.
const findNearbyQuery = `
SELECT source_id, year, month, day, date, hour, crime_type, crime_subtype,
       weapon_used, motorcycle_used, neighborhood, commune, quantity,
       longitude, latitude, id, distance
FROM (
    SELECT source_id, year, month, day,
           to_char(date, 'YYYY-MM-DD') AS date,
           hour, crime_type, crime_subtype, weapon_used, motorcycle_used,
           neighborhood, commune, quantity,
           ST_X(geom) AS longitude, ST_Y(geom) AS latitude, id,
           ST_Distance(geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography) AS distance
    FROM crimes
    WHERE ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)
) c
WHERE $4::float8 IS NULL
   OR c.distance > $4
   OR (c.distance = $4 AND c.id > $5)
ORDER BY c.distance ASC, c.id ASC
LIMIT $6`

// PostgresRepository implements Repository against PostgreSQL + PostGIS using pgx.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) FindNearby(ctx context.Context, query NearbyCrimesQuery) (CrimePage, error) {
	var cursorDistance *float64
	var cursorID *int64
	if query.Cursor != nil {
		cursorDistance = &query.Cursor.Distance
		cursorID = &query.Cursor.ID
	}

	// Fetch one extra row to detect whether a further page exists.
	rows, err := r.pool.Query(ctx, findNearbyQuery,
		query.Lng, query.Lat, query.RadiusMeters,
		cursorDistance, cursorID, query.Limit+1,
	)
	if err != nil {
		return CrimePage{}, fmt.Errorf("crimes: find nearby: %w", err)
	}
	defer rows.Close()

	// Keyset values (distance, id) are tracked per row so the next cursor can
	// be built from the last kept row after the probe row is dropped.
	items := make([]Crime, 0, query.Limit)
	keys := make([]Cursor, 0, query.Limit)

	for rows.Next() {
		var (
			c              Crime
			hour           int
			weaponUsed     bool
			motorcycleUsed bool
			lng, lat       float64
			key            Cursor
		)
		if err := rows.Scan(
			&c.SourceID, &c.Year, &c.Month, &c.Day, &c.Date,
			&hour, &c.CrimeType, &c.CrimeSubtype, &weaponUsed, &motorcycleUsed,
			&c.Neighborhood, &c.Commune, &c.Quantity,
			&lng, &lat, &key.ID, &key.Distance,
		); err != nil {
			return CrimePage{}, fmt.Errorf("crimes: scan nearby results: %w", err)
		}

		c.Hour = &hour
		c.WeaponUsed = &weaponUsed
		c.MotorcycleUsed = &motorcycleUsed
		c.Location = GeoJSON{Type: "Point", Coordinates: []float64{lng, lat}}

		items = append(items, c)
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return CrimePage{}, fmt.Errorf("crimes: read nearby results: %w", err)
	}

	page := CrimePage{Items: items}
	if len(items) > query.Limit {
		// More than a full page came back: drop the probe row and point the
		// cursor at the last row we keep.
		page.Items = items[:query.Limit]
		next := keys[query.Limit-1]
		page.HasMore = true
		page.Next = &next
	}

	return page, nil
}
