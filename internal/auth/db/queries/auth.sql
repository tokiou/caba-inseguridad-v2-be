-- name: CreateUser :one
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING id, email, password_hash, is_active, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, is_active, created_at, updated_at
FROM users
WHERE email = $1;

-- name: GetUserByID :one
SELECT id, email, password_hash, is_active, created_at, updated_at
FROM users
WHERE id = $1;

-- name: CreateRefreshSession :one
INSERT INTO refresh_sessions (user_id, token_hash, user_agent, ip_address, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, token_hash, user_agent, ip_address, created_at, expires_at, revoked_at, replaced_by;

-- name: GetRefreshSessionByHash :one
SELECT id, user_id, token_hash, user_agent, ip_address, created_at, expires_at, revoked_at, replaced_by
FROM refresh_sessions
WHERE token_hash = $1;

-- name: RevokeRefreshSession :exec
UPDATE refresh_sessions
SET revoked_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeAndReplaceRefreshSession :exec
UPDATE refresh_sessions
SET revoked_at = now(), replaced_by = $2
WHERE id = $1 AND revoked_at IS NULL;

-- name: InsertLoginAttempt :exec
INSERT INTO login_attempts (email, ip_address, success, reason)
VALUES ($1, $2, $3, $4);

-- name: InsertAuditLog :exec
INSERT INTO audit_logs (user_id, action, metadata, ip_address, user_agent)
VALUES ($1, $2, $3, $4, $5);
