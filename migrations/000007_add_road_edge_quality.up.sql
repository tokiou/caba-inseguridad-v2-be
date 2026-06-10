-- Non-destructive quality layer for the walkable road graph. Anomalous edges
-- (zero-length, self-loops, invalid geometry, suspiciously long) are MARKED
-- non-routable by scripts/osm/cleanup_road_graph.sql, never deleted, so the
-- import stays auditable and reversible.
--
--   is_walkable  -> provenance: edge came from the walking-profile import
--   is_routable  -> fitness:    edge is valid/safe for routing algorithms

ALTER TABLE road_edges
    ADD COLUMN IF NOT EXISTS is_routable        BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS excluded_reason    TEXT,
    ADD COLUMN IF NOT EXISTS quality_checked_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS road_edges_is_routable_idx
ON road_edges (is_routable);

CREATE INDEX IF NOT EXISTS road_edges_excluded_reason_idx
ON road_edges (excluded_reason);

-- Matches the router's filter and the routable_road_edges view predicate.
CREATE INDEX IF NOT EXISTS road_edges_routing_filter_idx
ON road_edges (is_walkable, is_routable);

-- The surface a future router queries — never raw road_edges.
CREATE OR REPLACE VIEW routable_road_edges AS
SELECT *
FROM road_edges
WHERE is_walkable = true
  AND is_routable = true;
