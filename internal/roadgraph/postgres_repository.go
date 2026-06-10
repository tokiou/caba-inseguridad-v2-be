package roadgraph

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// statsQuery computes all graph counts plus the node bounding box in one round
// trip. ST_Extent over an empty road_nodes returns NULL, so each bound is
// COALESCEd to 0 — an empty graph yields zeros, not an error.
const statsQuery = `
SELECT
    (SELECT COUNT(*) FROM road_nodes)                       AS nodes_count,
    (SELECT COUNT(*) FROM road_edges)                       AS edges_count,
    (SELECT COUNT(*) FROM road_edges WHERE is_walkable)     AS walkable_edges,
    (SELECT COUNT(*) FROM road_edges WHERE is_routable)     AS routable_edges,
    (SELECT COUNT(*) FROM road_edges WHERE NOT is_routable) AS excluded_edges,
    (SELECT COUNT(*) FROM edge_risk_scores)                 AS risk_scored_edges,
    COALESCE(ST_YMin(ext), 0)                               AS min_lat,
    COALESCE(ST_XMin(ext), 0)                               AS min_lng,
    COALESCE(ST_YMax(ext), 0)                               AS max_lat,
    COALESCE(ST_XMax(ext), 0)                               AS max_lng
FROM (SELECT ST_Extent(geom) AS ext FROM road_nodes) e`

// PostgresRepository implements Repository against PostgreSQL + PostGIS using pgx.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetStats(ctx context.Context) (GraphStats, error) {
	var s GraphStats
	if err := r.pool.QueryRow(ctx, statsQuery).Scan(
		&s.NodesCount, &s.EdgesCount, &s.WalkableEdges, &s.RoutableEdges, &s.ExcludedEdges,
		&s.RiskScoredEdges, &s.MinLat, &s.MinLng, &s.MaxLat, &s.MaxLng,
	); err != nil {
		return GraphStats{}, fmt.Errorf("roadgraph: get stats: %w", err)
	}
	return s, nil
}
