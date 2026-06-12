-- Restore the 000005/000006 placeholder shape.

DROP TABLE IF EXISTS edge_risk_backtest_labels;
DROP TABLE IF EXISTS risk_model_evaluation_runs;
DROP TABLE IF EXISTS edge_risk_score_components;
DROP TABLE IF EXISTS edge_risk_scores;
DROP TABLE IF EXISTS edge_network_neighborhoods;
DROP TABLE IF EXISTS crime_network_snaps;
DROP TABLE IF EXISTS risk_model_versions;

CREATE TABLE IF NOT EXISTS risk_model_versions (
    id BIGSERIAL PRIMARY KEY,

    name   TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,

    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,

    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at TIMESTAMPTZ
);

ALTER TABLE risk_model_versions
ADD CONSTRAINT risk_model_versions_status_check
CHECK (status IN ('draft', 'active', 'archived'));

CREATE TABLE IF NOT EXISTS edge_risk_scores (
    edge_id          BIGINT NOT NULL REFERENCES road_edges(id)          ON DELETE CASCADE,
    model_version_id BIGINT NOT NULL REFERENCES risk_model_versions(id) ON DELETE CASCADE,

    risk_score           DOUBLE PRECISION NOT NULL,
    crime_count          INTEGER          NOT NULL DEFAULT 0,
    weighted_crime_score DOUBLE PRECISION NOT NULL DEFAULT 0,

    computed_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (edge_id, model_version_id)
);

ALTER TABLE edge_risk_scores
ADD CONSTRAINT edge_risk_scores_risk_score_range_check
CHECK (risk_score >= 0 AND risk_score <= 1);

ALTER TABLE edge_risk_scores
ADD CONSTRAINT edge_risk_scores_crime_count_non_negative_check
CHECK (crime_count >= 0);

CREATE INDEX IF NOT EXISTS edge_risk_scores_model_version_idx
ON edge_risk_scores (model_version_id);

CREATE INDEX IF NOT EXISTS edge_risk_scores_risk_score_idx
ON edge_risk_scores (risk_score);

INSERT INTO risk_model_versions (name, status, parameters, activated_at)
VALUES (
    'v1_crime_density_distance_decay',
    'active',
    '{
        "crime_search_radius_meters": 100,
        "risk_sensitivity_default": 2.0,
        "walking_speed_meters_per_second": 1.4
    }'::jsonb,
    now()
)
ON CONFLICT (name) DO NOTHING;
