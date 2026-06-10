-- Risk scoring schema for the safe-walking engine. This milestone only creates
-- the tables; population (the scoring worker) is a future milestone.

-- Named, versioned risk models. Exactly one is expected to be 'active' at a time.
CREATE TABLE IF NOT EXISTS risk_model_versions (
    id BIGSERIAL PRIMARY KEY,

    name   TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,

    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,

    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    activated_at TIMESTAMPTZ
);

ALTER TABLE risk_model_versions
DROP CONSTRAINT IF EXISTS risk_model_versions_status_check;

ALTER TABLE risk_model_versions
ADD CONSTRAINT risk_model_versions_status_check
CHECK (status IN ('draft', 'active', 'archived'));

-- Per-edge risk under a given model version. Cascade-deletes with either parent.
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
DROP CONSTRAINT IF EXISTS edge_risk_scores_risk_score_range_check;

ALTER TABLE edge_risk_scores
ADD CONSTRAINT edge_risk_scores_risk_score_range_check
CHECK (risk_score >= 0 AND risk_score <= 1);

ALTER TABLE edge_risk_scores
DROP CONSTRAINT IF EXISTS edge_risk_scores_crime_count_non_negative_check;

ALTER TABLE edge_risk_scores
ADD CONSTRAINT edge_risk_scores_crime_count_non_negative_check
CHECK (crime_count >= 0);

CREATE INDEX IF NOT EXISTS edge_risk_scores_model_version_idx
ON edge_risk_scores (model_version_id);

CREATE INDEX IF NOT EXISTS edge_risk_scores_risk_score_idx
ON edge_risk_scores (risk_score);
