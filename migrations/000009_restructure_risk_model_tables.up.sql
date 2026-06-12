-- Restructure the placeholder risk schema (000005/000006) for the Network
-- Temporal KDE model. Both tables are placeholder-only at this point:
-- edge_risk_scores was never populated and risk_model_versions held a single
-- unused seed, so they are dropped and recreated rather than ALTERed.
--
-- Shape changes:
--   risk_model_versions: status -> is_active (partial unique index), plus
--     type, description, graph_version_id, train_until.
--   edge_risk_scores: temporal key (time_bucket, weekday_type) added, plus
--     raw_score and the p95 used for normalization.

DROP TABLE IF EXISTS edge_risk_scores;
DROP TABLE IF EXISTS risk_model_versions;

CREATE TABLE risk_model_versions (
    id BIGSERIAL PRIMARY KEY,

    name        TEXT NOT NULL UNIQUE,
    type        TEXT NOT NULL,
    description TEXT,

    graph_version_id BIGINT REFERENCES road_graph_versions(id),

    -- Every tunable weight of the model. Changing parameters = new version.
    parameters JSONB NOT NULL,

    -- Crimes dated after train_until never contribute to this version's scores.
    train_until DATE,

    is_active BOOLEAN NOT NULL DEFAULT false,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_one_active_risk_model
ON risk_model_versions (is_active)
WHERE is_active = true;

-- Where each crime enters the walkable graph, per graph version. Crimes too far
-- from any routable edge are kept (auditable) but flagged out of scoring.
CREATE TABLE crime_network_snaps (
    crime_id         BIGINT NOT NULL REFERENCES crimes(id)              ON DELETE CASCADE,
    graph_version_id BIGINT NOT NULL REFERENCES road_graph_versions(id) ON DELETE CASCADE,

    snapped_edge_id BIGINT NOT NULL REFERENCES road_edges(id) ON DELETE CASCADE,

    snap_distance_meters DOUBLE PRECISION NOT NULL,
    snap_fraction        DOUBLE PRECISION,
    snapped_geom         GEOMETRY(Point, 4326),

    is_valid_for_network_scoring BOOLEAN NOT NULL DEFAULT true,
    rejection_reason             TEXT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (crime_id, graph_version_id)
);

CREATE INDEX idx_crime_network_snaps_edge
ON crime_network_snaps (snapped_edge_id);

CREATE INDEX idx_crime_network_snaps_valid
ON crime_network_snaps (graph_version_id, is_valid_for_network_scoring);

-- Which edges are reachable from each edge over walking-network distance,
-- within a bandwidth. This is the propagation surface of the KDE: crime
-- influence travels through these pairs, never through euclidean distance.
-- Keyed by bandwidth so calibration variants coexist.
CREATE TABLE edge_network_neighborhoods (
    graph_version_id BIGINT NOT NULL REFERENCES road_graph_versions(id) ON DELETE CASCADE,

    source_edge_id BIGINT NOT NULL REFERENCES road_edges(id) ON DELETE CASCADE,
    target_edge_id BIGINT NOT NULL REFERENCES road_edges(id) ON DELETE CASCADE,

    bandwidth_meters        DOUBLE PRECISION NOT NULL,
    network_distance_meters DOUBLE PRECISION NOT NULL,

    path_edges_count INTEGER,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (graph_version_id, source_edge_id, target_edge_id, bandwidth_meters)
);

CREATE INDEX idx_edge_network_neighborhoods_target
ON edge_network_neighborhoods (graph_version_id, target_edge_id);

CREATE INDEX idx_edge_network_neighborhoods_source
ON edge_network_neighborhoods (graph_version_id, source_edge_id);

-- The contract table the Go API consumes. One row per edge, model version and
-- temporal context: 4 time buckets x {weekday, weekend} plus all_day/all.
CREATE TABLE edge_risk_scores (
    edge_id          BIGINT NOT NULL REFERENCES road_edges(id)          ON DELETE CASCADE,
    model_version_id BIGINT NOT NULL REFERENCES risk_model_versions(id) ON DELETE CASCADE,

    time_bucket  TEXT NOT NULL,
    weekday_type TEXT NOT NULL,

    raw_score  DOUBLE PRECISION NOT NULL,
    risk_score DOUBLE PRECISION NOT NULL CHECK (risk_score >= 0 AND risk_score <= 1),

    p95_reference DOUBLE PRECISION,
    computed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (edge_id, model_version_id, time_bucket, weekday_type)
);

CREATE INDEX idx_edge_risk_scores_lookup
ON edge_risk_scores (model_version_id, time_bucket, weekday_type);

CREATE INDEX idx_edge_risk_scores_risk
ON edge_risk_scores (model_version_id, time_bucket, weekday_type, risk_score DESC);

-- Explainability/debugging companion to edge_risk_scores: incident counts
-- (sums of crimes.quantity) behind each score. Metric data only — the API
-- exposes these numbers, never narrative explanations.
CREATE TABLE edge_risk_score_components (
    edge_id          BIGINT NOT NULL REFERENCES road_edges(id)          ON DELETE CASCADE,
    model_version_id BIGINT NOT NULL REFERENCES risk_model_versions(id) ON DELETE CASCADE,

    time_bucket  TEXT NOT NULL,
    weekday_type TEXT NOT NULL,

    crime_count          INTEGER          NOT NULL DEFAULT 0,
    weighted_crime_score DOUBLE PRECISION NOT NULL DEFAULT 0,

    robbery_count INTEGER NOT NULL DEFAULT 0,
    theft_count   INTEGER NOT NULL DEFAULT 0,
    threats_count INTEGER NOT NULL DEFAULT 0,

    armed_count      INTEGER NOT NULL DEFAULT 0,
    motorcycle_count INTEGER NOT NULL DEFAULT 0,

    same_bucket_crime_count     INTEGER NOT NULL DEFAULT 0,
    adjacent_bucket_crime_count INTEGER NOT NULL DEFAULT 0,
    other_bucket_crime_count    INTEGER NOT NULL DEFAULT 0,

    max_single_crime_contribution DOUBLE PRECISION,

    computed_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (edge_id, model_version_id, time_bucket, weekday_type)
);

CREATE INDEX idx_edge_risk_score_components_lookup
ON edge_risk_score_components (model_version_id, time_bucket, weekday_type);

-- One row per evaluation (edge-ranking backtest or route simulation).
CREATE TABLE risk_model_evaluation_runs (
    id BIGSERIAL PRIMARY KEY,

    model_version_id BIGINT NOT NULL REFERENCES risk_model_versions(id) ON DELETE CASCADE,

    train_until DATE NOT NULL,
    test_from   DATE NOT NULL,
    test_to     DATE NOT NULL,

    status TEXT NOT NULL DEFAULT 'pending',

    metrics    JSONB,
    parameters JSONB NOT NULL,

    passed         BOOLEAN,
    failure_reason TEXT,

    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_risk_model_evaluation_runs_model
ON risk_model_evaluation_runs (model_version_id);

-- Real future used as evaluation ground truth (test-window crimes propagated
-- with the same machinery but no recency decay). Never feeds scoring.
CREATE TABLE edge_risk_backtest_labels (
    edge_id           BIGINT NOT NULL REFERENCES road_edges(id)                  ON DELETE CASCADE,
    evaluation_run_id BIGINT NOT NULL REFERENCES risk_model_evaluation_runs(id) ON DELETE CASCADE,

    time_bucket  TEXT NOT NULL,
    weekday_type TEXT NOT NULL,

    future_weighted_crime_score DOUBLE PRECISION NOT NULL DEFAULT 0,
    future_crime_count          INTEGER          NOT NULL DEFAULT 0,
    future_robbery_count        INTEGER          NOT NULL DEFAULT 0,
    future_armed_count          INTEGER          NOT NULL DEFAULT 0,
    future_motorcycle_count     INTEGER          NOT NULL DEFAULT 0,

    PRIMARY KEY (edge_id, evaluation_run_id, time_bucket, weekday_type)
);

-- Evaluation model: trained on crimes <= 2022-12-31, judged against 2023.
-- Severity weights cover the dataset's real taxonomy (ROBO, HURTO, LESIONES,
-- VIALIDAD, AMENAZAS, HOMICIDIOS); see the change's design.md for rationale.
INSERT INTO risk_model_versions (
    name, type, description, graph_version_id, parameters, train_until, is_active
)
SELECT
    'network_temporal_edge_risk_v1_eval_2022',
    'deterministic_network_kde',
    'Network Temporal KDE deterministic risk model trained with crimes up to 2022-12-31 for evaluation against 2023.',
    g.id,
    '{
      "crime_snap_max_distance_meters": 80,
      "snap_distance_decay_meters": 30,

      "network_bandwidth_meters": 350,
      "network_distance_decay_meters": 100,

      "recency_half_life_days": 365,

      "severity_base_weights": {
        "HOMICIDIOS": 3.0,
        "ROBO": 1.5,
        "LESIONES": 1.2,
        "HURTO": 1.0,
        "AMENAZAS": 0.7,
        "VIALIDAD": 0.3,
        "DEFAULT": 1.0
      },

      "weapon_multiplier": 1.5,
      "motorcycle_multiplier": 1.25,

      "temporal_weights": {
        "same_bucket": 1.0,
        "adjacent_bucket": 0.5,
        "other_bucket": 0.2
      },

      "weekday_weights": {
        "same_type": 1.0,
        "other_type": 0.25,
        "all": 1.0
      },

      "normalization": {
        "method": "p95_clamp"
      },

      "risk_levels": {
        "low_max": 0.33,
        "moderate_max": 0.66,
        "high_max": 1.0
      }
    }'::jsonb,
    '2022-12-31',
    false
FROM road_graph_versions g
WHERE g.name = 'caba_walking_graph_v1'
ON CONFLICT (name) DO NOTHING;
