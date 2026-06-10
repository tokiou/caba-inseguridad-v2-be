package crimes

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	FindNearby(ctx context.Context, query NearbyCrimesQuery) ([]Crime, error)
}

// findNearbyQuery returns crimes within radius meters of the point, nearest first.
// Parameters: $1 = longitude, $2 = latitude, $3 = radius (meters). Coordinates are
// passed as [lng, lat] — never swapped. to_char keeps Date a "YYYY-MM-DD" string.
const findNearbyQuery = `
SELECT source_id, year, month, day,
       to_char(date, 'YYYY-MM-DD') AS date,
       hour, crime_type, crime_subtype, weapon_used, motorcycle_used,
       neighborhood, commune, quantity,
       ST_X(geom) AS longitude, ST_Y(geom) AS latitude
FROM crimes
WHERE ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography, $3)
ORDER BY ST_Distance(geom::geography, ST_SetSRID(ST_MakePoint($1, $2), 4326)::geography) ASC`

// PostgresRepository implements Repository against PostgreSQL + PostGIS using pgx.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) FindNearby(ctx context.Context, query NearbyCrimesQuery) ([]Crime, error) {
	rows, err := r.pool.Query(ctx, findNearbyQuery, query.Lng, query.Lat, query.RadiusMeters)
	if err != nil {
		return nil, fmt.Errorf("crimes: find nearby: %w", err)
	}
	defer rows.Close()

	items, err := pgx.CollectRows(rows, scanCrime)
	if err != nil {
		return nil, fmt.Errorf("crimes: scan nearby results: %w", err)
	}

	return items, nil
}

func scanCrime(row pgx.CollectableRow) (Crime, error) {
	var (
		c              Crime
		hour           int
		weaponUsed     bool
		motorcycleUsed bool
		lng, lat       float64
	)

	if err := row.Scan(
		&c.SourceID, &c.Year, &c.Month, &c.Day, &c.Date,
		&hour, &c.CrimeType, &c.CrimeSubtype, &weaponUsed, &motorcycleUsed,
		&c.Neighborhood, &c.Commune, &c.Quantity,
		&lng, &lat,
	); err != nil {
		return Crime{}, err
	}

	c.Hour = &hour
	c.WeaponUsed = &weaponUsed
	c.MotorcycleUsed = &motorcycleUsed
	c.Location = GeoJSON{Type: "Point", Coordinates: []float64{lng, lat}}

	return c, nil
}
