CREATE TABLE IF NOT EXISTS crimes (
    id BIGSERIAL PRIMARY KEY,

    source_id TEXT NOT NULL UNIQUE,

    year SMALLINT NOT NULL,
    month SMALLINT NOT NULL CHECK (month BETWEEN 1 AND 12),
    day SMALLINT NOT NULL CHECK (day BETWEEN 1 AND 31),

    date DATE NOT NULL,
    hour SMALLINT NOT NULL CHECK (hour BETWEEN 0 AND 23),

    crime_type TEXT NOT NULL,
    crime_subtype TEXT,

    weapon_used BOOLEAN NOT NULL DEFAULT false,
    motorcycle_used BOOLEAN NOT NULL DEFAULT false,

    neighborhood TEXT,
    commune SMALLINT,

    quantity INTEGER NOT NULL DEFAULT 1 CHECK (quantity > 0),

    geom GEOMETRY(Point, 4326) NOT NULL,

    raw_payload JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_crimes_geom
ON crimes
USING GIST (geom);

CREATE INDEX IF NOT EXISTS idx_crimes_date
ON crimes (date);

CREATE INDEX IF NOT EXISTS idx_crimes_hour
ON crimes (hour);

CREATE INDEX IF NOT EXISTS idx_crimes_type_subtype
ON crimes (crime_type, crime_subtype);

CREATE INDEX IF NOT EXISTS idx_crimes_neighborhood
ON crimes (neighborhood);

CREATE INDEX IF NOT EXISTS idx_crimes_commune
ON crimes (commune);
