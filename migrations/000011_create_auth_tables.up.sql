-- User accounts and authentication. Relational CRUD only (no PostGIS), so these
-- tables are owned by sqlc; pgx still owns the geospatial queries.
-- gen_random_uuid() is built into PostgreSQL 13+.

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_active     BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Opaque refresh tokens, stored only as a SHA-256 hash. Rotation links a revoked
-- session to its successor via replaced_by.
CREATE TABLE IF NOT EXISTS refresh_sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,
    user_agent  TEXT,
    ip_address  INET,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    replaced_by UUID REFERENCES refresh_sessions(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_refresh_sessions_user_id ON refresh_sessions (user_id);
-- Active-session lookups (refresh/logout) only care about non-revoked rows.
CREATE INDEX IF NOT EXISTS idx_refresh_sessions_active
    ON refresh_sessions (user_id) WHERE revoked_at IS NULL;

-- Every login outcome, for auditing and future rate limiting.
CREATE TABLE IF NOT EXISTS login_attempts (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email      TEXT NOT NULL,
    ip_address INET,
    success    BOOLEAN NOT NULL,
    reason     TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_login_attempts_email_created
    ON login_attempts (email, created_at DESC);

-- User-attributable auth events (register, login, refresh, logout, ...).
CREATE TABLE IF NOT EXISTS audit_logs (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    action     TEXT NOT NULL,
    metadata   JSONB,
    ip_address INET,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id_created
    ON audit_logs (user_id, created_at DESC);
