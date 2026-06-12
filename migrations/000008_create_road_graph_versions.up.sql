-- Named road-graph versions. Risk artifacts (crime snaps, network neighborhoods)
-- are computed against a specific graph version so a graph re-import never
-- silently invalidates scores: a new import is a new version.
CREATE TABLE IF NOT EXISTS road_graph_versions (
    id BIGSERIAL PRIMARY KEY,

    name        TEXT NOT NULL UNIQUE,
    description TEXT,

    is_active BOOLEAN NOT NULL DEFAULT false,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- At most one active graph version.
CREATE UNIQUE INDEX IF NOT EXISTS idx_one_active_road_graph_version
ON road_graph_versions (is_active)
WHERE is_active = true;

INSERT INTO road_graph_versions (name, description, is_active)
VALUES (
    'caba_walking_graph_v1',
    'Cleaned walkable road graph for CABA (OSM import + quality layer, routable_road_edges surface).',
    true
)
ON CONFLICT (name) DO NOTHING;
