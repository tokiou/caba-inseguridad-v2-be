-- Routing profiles consumed by /api/v1/routes/safe. Tuning the safety/detour
-- trade-off is data, not a deploy.
CREATE TABLE IF NOT EXISTS route_profiles (
    name TEXT PRIMARY KEY,

    safety_multiplier DOUBLE PRECISION NOT NULL,
    max_detour_ratio  DOUBLE PRECISION NOT NULL,

    description TEXT
);

INSERT INTO route_profiles (name, safety_multiplier, max_detour_ratio, description)
VALUES
    ('fastest',  0.0, 1.00, 'Shortest walking route by distance.'),
    ('balanced', 1.5, 1.35, 'Balances distance and estimated historical exposure.'),
    ('safest',   3.0, 1.75, 'Prioritizes lower estimated historical exposure.')
ON CONFLICT (name) DO UPDATE SET
    safety_multiplier = EXCLUDED.safety_multiplier,
    max_detour_ratio  = EXCLUDED.max_detour_ratio,
    description       = EXCLUDED.description;
