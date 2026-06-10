-- First risk model version, seeded as the active model. The scoring worker
-- (future milestone) will read these parameters to compute edge_risk_scores.
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
