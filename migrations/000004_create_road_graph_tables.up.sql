-- Walkable road graph for CABA: nodes (intersections / connection points) and
-- edges (walkable street segments). Built offline from OpenStreetMap via
-- osm2pgrouting and normalized into these clean tables (see scripts/osm/).
-- Risk attaches to edges, not nodes (see 000005_create_risk_model_tables).

CREATE TABLE IF NOT EXISTS road_nodes (
    id BIGSERIAL PRIMARY KEY,

    -- Originating graph-tool vertex id (osm2pgrouting ways_vertices_pgr.id).
    source_node_id BIGINT NOT NULL UNIQUE,

    geom GEOMETRY(Point, 4326) NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS road_nodes_geom_gist_idx
ON road_nodes
USING GIST (geom);

CREATE INDEX IF NOT EXISTS road_nodes_source_node_id_idx
ON road_nodes (source_node_id);

CREATE TABLE IF NOT EXISTS road_edges (
    id BIGSERIAL PRIMARY KEY,

    -- Originating graph-tool edge id (osm2pgrouting ways.gid). One OSM way may
    -- split into several edges at intersections; each split has a unique id.
    source_edge_id BIGINT NOT NULL UNIQUE,

    from_node_id BIGINT NOT NULL REFERENCES road_nodes(id),
    to_node_id   BIGINT NOT NULL REFERENCES road_nodes(id),

    street_name  TEXT,
    highway_type TEXT,

    length_meters         DOUBLE PRECISION NOT NULL,
    walk_duration_seconds DOUBLE PRECISION NOT NULL,

    is_walkable BOOLEAN NOT NULL DEFAULT true,

    geom GEOMETRY(LineString, 4326) NOT NULL,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS road_edges_geom_gist_idx
ON road_edges
USING GIST (geom);

CREATE INDEX IF NOT EXISTS road_edges_from_node_id_idx
ON road_edges (from_node_id);

CREATE INDEX IF NOT EXISTS road_edges_to_node_id_idx
ON road_edges (to_node_id);

CREATE INDEX IF NOT EXISTS road_edges_source_edge_id_idx
ON road_edges (source_edge_id);

CREATE INDEX IF NOT EXISTS road_edges_walkable_idx
ON road_edges (is_walkable);
