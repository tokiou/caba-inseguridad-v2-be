-- Normalize the raw osm2pgrouting tables into the clean internal road graph.
--
-- Source (raw, tool-owned, created by import_osm_graph.sh with --prefix osm_):
--   osm_ways_vertices_pgr(id, the_geom geometry(Point,4326), ...)
--   osm_ways(gid, source, target, name, length_m, the_geom geometry(LineString,4326),
--            tag_id, ...)   -- tag_id -> configuration table (highway class; future backfill)
-- Target (clean, app-owned): road_nodes, road_edges.
--
-- Idempotent: ON CONFLICT on the unique source ids, so re-running after a
-- re-import inserts only new rows. Column mapping was written after inspecting
-- information_schema.columns for the two osm_* tables (osm2pgrouting 2.3.8).
--
-- Run from the repo root, e.g.:
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f scripts/osm/normalize_osm_graph.sql

\set ON_ERROR_STOP on

BEGIN;

-- Nodes: one road_nodes row per graph vertex. the_geom is already SRID 4326.
INSERT INTO road_nodes (source_node_id, geom)
SELECT
    v.id        AS source_node_id,
    v.the_geom  AS geom
FROM osm_ways_vertices_pgr v
WHERE v.the_geom IS NOT NULL
ON CONFLICT (source_node_id) DO NOTHING;

-- Edges: one road_edges row per walkable segment, endpoints resolved to our
-- node ids via source_node_id. length_m is metres (verified == ST_Length on
-- ::geography); walking duration assumes 1.4 m/s. highway_type left NULL this
-- milestone (resolvable later from osm_ways.tag_id -> configuration).
INSERT INTO road_edges (
    source_edge_id,
    from_node_id,
    to_node_id,
    street_name,
    highway_type,
    length_meters,
    walk_duration_seconds,
    is_walkable,
    geom
)
SELECT
    w.gid                                                   AS source_edge_id,
    rn_from.id                                              AS from_node_id,
    rn_to.id                                                AS to_node_id,
    NULLIF(w.name, '')                                      AS street_name,
    NULL                                                    AS highway_type,
    COALESCE(w.length_m, ST_Length(w.the_geom::geography))  AS length_meters,
    COALESCE(w.length_m, ST_Length(w.the_geom::geography)) / 1.4 AS walk_duration_seconds,
    true                                                    AS is_walkable,
    w.the_geom                                              AS geom
FROM osm_ways w
JOIN road_nodes rn_from ON rn_from.source_node_id = w.source
JOIN road_nodes rn_to   ON rn_to.source_node_id   = w.target
WHERE w.source IS NOT NULL
  AND w.target IS NOT NULL
  AND w.the_geom IS NOT NULL
ON CONFLICT (source_edge_id) DO NOTHING;

COMMIT;
