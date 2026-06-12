package roadgraph

import (
	"context"
	"encoding/json"
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

// walkRouteQuery snaps origin/destination to the nearest *routable edge* and
// enters the graph at that edge's nearer endpoint, then runs an undirected
// pgr_dijkstra over routable_road_edges with distance as cost and aggregates the
// path edges into total distance/duration, an edge count, and a merged LineString.
//
// Snapping targets the nearest edge (not the nearest node) on purpose: a bare
// nearest-node snap can land on a node whose edges were all marked non-routable by
// the quality cleanup — an orphan in the routable subgraph — and pgr_dijkstra then
// finds no path. Snapping to the nearest routable edge guarantees the entry node
// participates in the routable graph.
//
// Aggregation over an empty path returns one row with edge_count = 0 and a NULL
// geometry — the caller maps that to ErrNoRoute.
// $1/$2 = origin lng/lat, $3/$4 = destination lng/lat (ST_MakePoint is lng, lat).
const walkRouteQuery = `
WITH params AS (
    SELECT ST_SetSRID(ST_MakePoint($1, $2), 4326) AS src_pt,
           ST_SetSRID(ST_MakePoint($3, $4), 4326) AS dst_pt
),
src_edge AS (
    SELECT e.from_node_id, e.to_node_id
    FROM routable_road_edges e, params p
    ORDER BY e.geom <-> p.src_pt
    LIMIT 1
),
dst_edge AS (
    SELECT e.from_node_id, e.to_node_id
    FROM routable_road_edges e, params p
    ORDER BY e.geom <-> p.dst_pt
    LIMIT 1
),
src AS (
    SELECT CASE WHEN ST_Distance(nf.geom, p.src_pt) <= ST_Distance(nt.geom, p.src_pt)
                THEN se.from_node_id ELSE se.to_node_id END AS id
    FROM src_edge se
    CROSS JOIN params p
    JOIN road_nodes nf ON nf.id = se.from_node_id
    JOIN road_nodes nt ON nt.id = se.to_node_id
),
dst AS (
    SELECT CASE WHEN ST_Distance(nf.geom, p.dst_pt) <= ST_Distance(nt.geom, p.dst_pt)
                THEN de.from_node_id ELSE de.to_node_id END AS id
    FROM dst_edge de
    CROSS JOIN params p
    JOIN road_nodes nf ON nf.id = de.from_node_id
    JOIN road_nodes nt ON nt.id = de.to_node_id
),
path AS (
    SELECT seq, edge
    FROM pgr_dijkstra(
        'SELECT id, from_node_id AS source, to_node_id AS target, length_meters AS cost FROM routable_road_edges',
        (SELECT id FROM src),
        (SELECT id FROM dst),
        false
    )
)
SELECT
    (SELECT id FROM src)                                       AS from_node_id,
    (SELECT id FROM dst)                                       AS to_node_id,
    COALESCE(SUM(re.length_meters), 0)                         AS distance_meters,
    COALESCE(SUM(re.walk_duration_seconds), 0)                 AS duration_seconds,
    COUNT(re.id)                                               AS edge_count,
    ST_AsGeoJSON(ST_LineMerge(ST_Collect(re.geom ORDER BY path.seq))) AS geometry
FROM path
JOIN road_edges re ON re.id = path.edge
WHERE path.edge <> -1`

func (r *PostgresRepository) FindWalkRoute(ctx context.Context, query WalkRouteQuery) (WalkRoute, error) {
	var (
		fromNode, toNode             *int64
		distanceMeters, durationSecs float64
		edgeCount                    int
		geomJSON                     *string
	)
	if err := r.pool.QueryRow(ctx, walkRouteQuery,
		query.FromLng, query.FromLat, query.ToLng, query.ToLat,
	).Scan(&fromNode, &toNode, &distanceMeters, &durationSecs, &edgeCount, &geomJSON); err != nil {
		return WalkRoute{}, fmt.Errorf("roadgraph: find walk route: %w", err)
	}

	// No snapped node means the graph is empty; no edges / NULL geometry means the
	// endpoints exist but are not connected. Both are "no route" to the caller.
	if fromNode == nil || toNode == nil || edgeCount == 0 || geomJSON == nil {
		return WalkRoute{}, ErrNoRoute
	}

	var geom GeoJSONLineString
	if err := json.Unmarshal([]byte(*geomJSON), &geom); err != nil {
		return WalkRoute{}, fmt.Errorf("roadgraph: decode route geometry: %w", err)
	}
	if geom.Type != "LineString" {
		return WalkRoute{}, fmt.Errorf("roadgraph: unexpected route geometry type %q", geom.Type)
	}

	return WalkRoute{
		From:            RoutePoint{Lat: query.FromLat, Lng: query.FromLng, SnappedNodeID: *fromNode},
		To:              RoutePoint{Lat: query.ToLat, Lng: query.ToLng, SnappedNodeID: *toNode},
		DistanceMeters:  distanceMeters,
		DurationSeconds: durationSecs,
		EdgeCount:       edgeCount,
		Geometry:        geom,
	}, nil
}
