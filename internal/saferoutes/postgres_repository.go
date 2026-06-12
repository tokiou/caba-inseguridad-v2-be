package saferoutes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements Repository on PostgreSQL + PostGIS + pgRouting.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const activeModelQuery = `
SELECT
    id,
    name,
    type,
    COALESCE(train_until::text, ''),
    COALESCE((parameters->'risk_levels'->>'low_max')::float8, 0.33),
    COALESCE((parameters->'risk_levels'->>'moderate_max')::float8, 0.66)
FROM risk_model_versions
WHERE is_active = true`

func (r *PostgresRepository) ActiveModel(ctx context.Context) (ModelVersion, error) {
	var m ModelVersion
	err := r.pool.QueryRow(ctx, activeModelQuery).Scan(
		&m.ID, &m.Name, &m.Type, &m.TrainUntil, &m.LowMax, &m.ModerateMax,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ModelVersion{}, ErrNoActiveModel
	}
	if err != nil {
		return ModelVersion{}, fmt.Errorf("saferoutes: active model: %w", err)
	}
	return m, nil
}

func (r *PostgresRepository) RouteProfiles(ctx context.Context) (map[string]RouteProfile, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT name, safety_multiplier, max_detour_ratio FROM route_profiles`)
	if err != nil {
		return nil, fmt.Errorf("saferoutes: route profiles: %w", err)
	}
	defer rows.Close()

	profiles := make(map[string]RouteProfile)
	for rows.Next() {
		var p RouteProfile
		if err := rows.Scan(&p.Name, &p.SafetyMultiplier, &p.MaxDetourRatio); err != nil {
			return nil, fmt.Errorf("saferoutes: scan route profile: %w", err)
		}
		profiles[p.Name] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("saferoutes: route profiles: %w", err)
	}
	return profiles, nil
}

// snapQuery resolves each endpoint to the nearest routable edge's nearer
// endpoint node (entering at the nearest *edge* avoids nodes orphaned by the
// quality cleanup), returning the geography distance from the request point to
// the chosen node so the service can enforce the snapping limit.
// $1/$2 = origin lng/lat, $3/$4 = destination lng/lat.
const snapQuery = `
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
    SELECT
        CASE WHEN ST_Distance(nf.geom::geography, p.src_pt::geography)
                  <= ST_Distance(nt.geom::geography, p.src_pt::geography)
             THEN se.from_node_id ELSE se.to_node_id END AS id,
        LEAST(ST_Distance(nf.geom::geography, p.src_pt::geography),
              ST_Distance(nt.geom::geography, p.src_pt::geography)) AS dist
    FROM src_edge se
    CROSS JOIN params p
    JOIN road_nodes nf ON nf.id = se.from_node_id
    JOIN road_nodes nt ON nt.id = se.to_node_id
),
dst AS (
    SELECT
        CASE WHEN ST_Distance(nf.geom::geography, p.dst_pt::geography)
                  <= ST_Distance(nt.geom::geography, p.dst_pt::geography)
             THEN de.from_node_id ELSE de.to_node_id END AS id,
        LEAST(ST_Distance(nf.geom::geography, p.dst_pt::geography),
              ST_Distance(nt.geom::geography, p.dst_pt::geography)) AS dist
    FROM dst_edge de
    CROSS JOIN params p
    JOIN road_nodes nf ON nf.id = de.from_node_id
    JOIN road_nodes nt ON nt.id = de.to_node_id
)
SELECT
    (SELECT id FROM src), (SELECT dist FROM src),
    (SELECT id FROM dst), (SELECT dist FROM dst)`

func (r *PostgresRepository) SnapEndpoints(ctx context.Context, query SafeRoutesQuery) (SnapResult, error) {
	var (
		originNode, destNode *int64
		originDist, destDist *float64
	)
	err := r.pool.QueryRow(ctx, snapQuery,
		query.OriginLng, query.OriginLat, query.DestLng, query.DestLat,
	).Scan(&originNode, &originDist, &destNode, &destDist)
	if err != nil {
		return SnapResult{}, fmt.Errorf("saferoutes: snap endpoints: %w", err)
	}
	if originNode == nil || destNode == nil {
		// Empty graph: nothing to walk on.
		return SnapResult{}, ErrPointOutsideGraph
	}
	return SnapResult{
		OriginNodeID:   *originNode,
		OriginDistance: *originDist,
		DestNodeID:     *destNode,
		DestDistance:   *destDist,
	}, nil
}

// riskCostInner is the edge SQL handed to pgRouting. pgr_dijkstra/pgr_ksp
// execute it as a standalone statement, so it cannot reference outer bind
// parameters: the multiplier, model id and context are interpolated instead.
// All three are server-side values (route_profiles row, active model row, and
// a whitelisted bucket constant) — never user input; validateContext guards
// the strings and the numbers are typed int64/float64.
const riskCostInner = `SELECT e.id, e.from_node_id AS source, e.to_node_id AS target,
e.length_meters * (1 + %g * COALESCE(r.risk_score, 0)) AS cost
FROM routable_road_edges e
LEFT JOIN edge_risk_scores r
  ON r.edge_id = e.id AND r.model_version_id = %d
 AND r.time_bucket = '%s' AND r.weekday_type = '%s'`

const distanceCostInner = `SELECT id, from_node_id AS source, to_node_id AS target,
length_meters AS cost FROM routable_road_edges`

var validTimeBuckets = map[string]bool{
	"morning": true, "afternoon": true, "evening": true, "night": true, "all_day": true,
}

var validWeekdayTypes = map[string]bool{"weekday": true, "weekend": true, "all": true}

func validateContext(timeBucket, weekdayType string) error {
	if !validTimeBuckets[timeBucket] || !validWeekdayTypes[weekdayType] {
		return fmt.Errorf("saferoutes: invalid risk context %q/%q", timeBucket, weekdayType)
	}
	return nil
}

// routeQuery wraps one pgr_dijkstra path: per-edge rows enriched with the
// context's risk score and crime components, aggregated into a JSON edge list
// plus a merged LineString. An empty path yields edge_count = 0 / NULLs.
// %s = inner cost SQL; $1/$2 = from/to node, $3 = model id, $4/$5 = context.
const routeQueryTemplate = `
WITH path AS (
    SELECT seq, edge
    FROM pgr_dijkstra($q$%s$q$, $1::bigint, $2::bigint, false)
    WHERE edge <> -1
),
enriched AS (
    SELECT
        path.seq,
        e.id, e.length_meters, e.walk_duration_seconds, e.geom,
        COALESCE(r.risk_score, 0) AS risk_score,
        COALESCE(c.crime_count, 0) AS crime_count,
        COALESCE(c.robbery_count, 0) AS robbery_count,
        COALESCE(c.theft_count, 0) AS theft_count,
        COALESCE(c.threats_count, 0) AS threats_count,
        COALESCE(c.armed_count, 0) AS armed_count,
        COALESCE(c.motorcycle_count, 0) AS motorcycle_count,
        COALESCE(c.same_bucket_crime_count, 0) AS same_bucket_crime_count
    FROM path
    JOIN road_edges e ON e.id = path.edge
    LEFT JOIN edge_risk_scores r
      ON r.edge_id = e.id AND r.model_version_id = $3
     AND r.time_bucket = $4 AND r.weekday_type = $5
    LEFT JOIN edge_risk_score_components c
      ON c.edge_id = e.id AND c.model_version_id = $3
     AND c.time_bucket = $4 AND c.weekday_type = $5
)
SELECT
    COUNT(*),
    ST_AsGeoJSON(ST_LineMerge(ST_Collect(geom ORDER BY seq))),
    COALESCE(json_agg(json_build_object(
        'edge_id', id,
        'length_meters', length_meters,
        'duration_seconds', walk_duration_seconds,
        'risk_score', risk_score,
        'crime_count', crime_count,
        'robbery_count', robbery_count,
        'theft_count', theft_count,
        'threats_count', threats_count,
        'armed_count', armed_count,
        'motorcycle_count', motorcycle_count,
        'same_bucket_crime_count', same_bucket_crime_count
    ) ORDER BY seq), '[]')::text
FROM enriched`

func (r *PostgresRepository) FindRoute(ctx context.Context, req RouteRequest) (RoutePath, error) {
	if err := validateContext(req.TimeBucket, req.WeekdayType); err != nil {
		return RoutePath{}, err
	}

	inner := distanceCostInner
	if req.SafetyMultiplier > 0 {
		inner = fmt.Sprintf(riskCostInner,
			req.SafetyMultiplier, req.ModelVersionID, req.TimeBucket, req.WeekdayType)
	}
	query := fmt.Sprintf(routeQueryTemplate, inner)

	var (
		edgeCount int
		geomJSON  *string
		edgesJSON string
	)
	err := r.pool.QueryRow(ctx, query,
		req.FromNodeID, req.ToNodeID, req.ModelVersionID, req.TimeBucket, req.WeekdayType,
	).Scan(&edgeCount, &geomJSON, &edgesJSON)
	if err != nil {
		return RoutePath{}, fmt.Errorf("saferoutes: find route: %w", err)
	}
	if edgeCount == 0 || geomJSON == nil {
		return RoutePath{}, ErrNoRoute
	}
	return decodeRoutePath(*geomJSON, edgesJSON)
}

// kspQueryTemplate returns the K shortest paths by plain distance, one row per
// path, same enrichment as routeQueryTemplate.
// $1/$2 = from/to node, $3 = K, $4 = model id, $5/$6 = context.
const kspQueryTemplate = `
WITH path AS (
    SELECT path_id, seq, edge
    FROM pgr_ksp($q$` + distanceCostInner + `$q$, $1::bigint, $2::bigint, $3::integer, directed := false)
    WHERE edge <> -1
),
enriched AS (
    SELECT
        path.path_id, path.seq,
        e.id, e.length_meters, e.walk_duration_seconds, e.geom,
        COALESCE(r.risk_score, 0) AS risk_score,
        COALESCE(c.crime_count, 0) AS crime_count,
        COALESCE(c.robbery_count, 0) AS robbery_count,
        COALESCE(c.theft_count, 0) AS theft_count,
        COALESCE(c.threats_count, 0) AS threats_count,
        COALESCE(c.armed_count, 0) AS armed_count,
        COALESCE(c.motorcycle_count, 0) AS motorcycle_count,
        COALESCE(c.same_bucket_crime_count, 0) AS same_bucket_crime_count
    FROM path
    JOIN road_edges e ON e.id = path.edge
    LEFT JOIN edge_risk_scores r
      ON r.edge_id = e.id AND r.model_version_id = $4
     AND r.time_bucket = $5 AND r.weekday_type = $6
    LEFT JOIN edge_risk_score_components c
      ON c.edge_id = e.id AND c.model_version_id = $4
     AND c.time_bucket = $5 AND c.weekday_type = $6
)
SELECT
    path_id,
    ST_AsGeoJSON(ST_LineMerge(ST_Collect(geom ORDER BY seq))),
    json_agg(json_build_object(
        'edge_id', id,
        'length_meters', length_meters,
        'duration_seconds', walk_duration_seconds,
        'risk_score', risk_score,
        'crime_count', crime_count,
        'robbery_count', robbery_count,
        'theft_count', theft_count,
        'threats_count', threats_count,
        'armed_count', armed_count,
        'motorcycle_count', motorcycle_count,
        'same_bucket_crime_count', same_bucket_crime_count
    ) ORDER BY seq)::text
FROM enriched
GROUP BY path_id
ORDER BY path_id`

func (r *PostgresRepository) FindCandidateRoutes(ctx context.Context, req CandidateRouteRequest) ([]RoutePath, error) {
	if err := validateContext(req.TimeBucket, req.WeekdayType); err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, kspQueryTemplate,
		req.FromNodeID, req.ToNodeID, req.K, req.ModelVersionID, req.TimeBucket, req.WeekdayType,
	)
	if err != nil {
		return nil, fmt.Errorf("saferoutes: find candidate routes: %w", err)
	}
	defer rows.Close()

	var paths []RoutePath
	for rows.Next() {
		var (
			pathID    int
			geomJSON  *string
			edgesJSON string
		)
		if err := rows.Scan(&pathID, &geomJSON, &edgesJSON); err != nil {
			return nil, fmt.Errorf("saferoutes: scan candidate route: %w", err)
		}
		if geomJSON == nil {
			continue
		}
		path, err := decodeRoutePath(*geomJSON, edgesJSON)
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("saferoutes: find candidate routes: %w", err)
	}
	return paths, nil
}

func decodeRoutePath(geomJSON, edgesJSON string) (RoutePath, error) {
	var geom GeoJSONLineString
	if err := json.Unmarshal([]byte(geomJSON), &geom); err != nil {
		return RoutePath{}, fmt.Errorf("saferoutes: decode route geometry: %w", err)
	}
	if geom.Type != "LineString" {
		return RoutePath{}, fmt.Errorf("saferoutes: unexpected route geometry type %q", geom.Type)
	}
	var edges []PathEdge
	if err := json.Unmarshal([]byte(edgesJSON), &edges); err != nil {
		return RoutePath{}, fmt.Errorf("saferoutes: decode route edges: %w", err)
	}
	return RoutePath{Edges: edges, Geometry: geom}, nil
}
