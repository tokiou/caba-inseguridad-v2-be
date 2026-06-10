-- Road graph quality cleanup — marks anomalous edges as non-routable WITHOUT
-- deleting any imported data. Idempotent and safe to rerun: it resets quality
-- state first, then re-applies the rules, so the outcome is independent of how
-- many times it has run. Run after an OSM import + normalize, from the repo root:
--
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f scripts/osm/cleanup_road_graph.sql
--
-- Requires migration 000007 (is_routable / excluded_reason / quality_checked_at,
-- routable_road_edges view).
--
-- Each rule only touches rows still is_routable = true, so the first matching
-- reason wins. Exclusion reasons:
--   invalid_geometry                 geom NULL or not ST_IsValid
--   zero_or_negative_length          length_meters <= 0
--   self_loop                        from_node_id = to_node_id
--   suspicious_long_edge_over_5000m  length_meters > 5000 (single indivisible
--                                    urban edge; almost certainly an OSM artifact)
--
-- Validation queries (run manually after cleanup):
--
--   -- counts by status
--   SELECT COALESCE(excluded_reason, 'routable') AS status, COUNT(*) AS count
--   FROM road_edges GROUP BY COALESCE(excluded_reason, 'routable') ORDER BY count DESC;
--
--   -- quality summary
--   SELECT COUNT(*) AS total_edges,
--          COUNT(*) FILTER (WHERE is_walkable) AS walkable_edges,
--          COUNT(*) FILTER (WHERE is_routable) AS routable_edges,
--          COUNT(*) FILTER (WHERE NOT is_routable) AS excluded_edges
--   FROM road_edges;
--
--   -- longest excluded edges
--   SELECT id, source_edge_id, street_name, length_meters, excluded_reason,
--          ST_AsText(geom) AS geom_wkt
--   FROM road_edges WHERE NOT is_routable ORDER BY length_meters DESC LIMIT 20;
--
--   -- routable view count (== routable_edges above)
--   SELECT COUNT(*) FROM routable_road_edges;

\set ON_ERROR_STOP on

BEGIN;

-- Reset quality state so the pass is fully idempotent.
UPDATE road_edges
SET is_routable        = true,
    excluded_reason    = NULL,
    quality_checked_at = now();

-- Rule 1: invalid or missing geometry.
UPDATE road_edges
SET is_routable        = false,
    excluded_reason    = 'invalid_geometry',
    quality_checked_at = now()
WHERE is_routable = true
  AND (geom IS NULL OR NOT ST_IsValid(geom));

-- Rule 2: zero or negative length.
UPDATE road_edges
SET is_routable        = false,
    excluded_reason    = 'zero_or_negative_length',
    quality_checked_at = now()
WHERE is_routable = true
  AND length_meters <= 0;

-- Rule 3: self-loop (edge connects a node to itself).
UPDATE road_edges
SET is_routable        = false,
    excluded_reason    = 'self_loop',
    quality_checked_at = now()
WHERE is_routable = true
  AND from_node_id = to_node_id;

-- Rule 4: suspiciously long single urban walking edge.
UPDATE road_edges
SET is_routable        = false,
    excluded_reason    = 'suspicious_long_edge_over_5000m',
    quality_checked_at = now()
WHERE is_routable = true
  AND length_meters > 5000;

-- Recreate the routable surface (idempotent; harmless if unchanged).
CREATE OR REPLACE VIEW routable_road_edges AS
SELECT *
FROM road_edges
WHERE is_walkable = true
  AND is_routable = true;

COMMIT;
