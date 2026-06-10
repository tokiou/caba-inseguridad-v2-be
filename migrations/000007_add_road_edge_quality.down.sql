DROP VIEW IF EXISTS routable_road_edges;

DROP INDEX IF EXISTS road_edges_routing_filter_idx;
DROP INDEX IF EXISTS road_edges_excluded_reason_idx;
DROP INDEX IF EXISTS road_edges_is_routable_idx;

ALTER TABLE road_edges
    DROP COLUMN IF EXISTS quality_checked_at,
    DROP COLUMN IF EXISTS excluded_reason,
    DROP COLUMN IF EXISTS is_routable;
