# Tasks — user accounts and JWT authentication

## 1. Spec
- [x] 1.1 Write delta `specs/auth/spec.md` (ADDED requirements for accounts, login, refresh, logout,
      me, access token, refresh rotation, auth middleware, audit/login-attempt recording).
- [x] 1.2 Write delta `specs/safe-routes/spec.md` (ADDED "Authentication required" on `/routes/safe`).
- [x] 1.3 `openspec validate add-user-auth --strict` passes.

## 2. Dependencies & config
- [x] 2.1 `go get golang.org/x/crypto/bcrypt github.com/golang-jwt/jwt/v5`.
- [x] 2.2 Extend `internal/config/config.go`: `JWTSecret`, `AccessTokenTTL`, `RefreshTokenTTL`,
      `RefreshCookieName`, `CookieSecure`, `CookieSameSite`; add `.env.example` entries.
- [x] 2.3 Validate `JWT_SECRET` at startup (reject empty/`change_me` unless `APP_ENV=development`).

## 2b. sqlc setup (first use in repo)
- [x] 2b.1 Add `sqlc.yaml` (engine postgresql, `sql_package: pgx/v5`, schema = `migrations/`,
      queries = `internal/auth/db/queries/`, out = `internal/auth/db`, package `authdb`).
- [x] 2b.2 Write `internal/auth/db/queries/auth.sql` with annotated queries (CreateUser, GetUserByEmail,
      GetUserByID, CreateSession, GetSessionByHash, RevokeSession, MarkReplacedBy, InsertLoginAttempt,
      InsertAuditLog).
- [x] 2b.3 Pin sqlc as a tool dependency and run `go run github.com/sqlc-dev/sqlc/cmd/sqlc generate`;
      add a `//go:generate` directive so `go generate ./...` regenerates `internal/auth/db`.

## 3. Migration (`migrations/000011_create_auth_tables.{up,down}.sql`)
- [x] 3.1 `users`, `refresh_sessions`, `login_attempts`, `audit_logs` per design, with indexes
      (`refresh_sessions(user_id)`, partial `WHERE revoked_at IS NULL`, `login_attempts(email,created_at)`).
- [x] 3.2 Down migration drops the four tables.
- [x] 3.3 Apply locally and confirm `\d` shapes.

## 4. Domain — `internal/auth/`
- [x] 4.1 `model.go` — `User`, `RefreshSession`, audit-action constants.
- [x] 4.2 `dto.go` — request/response DTOs (no `password_hash` ever serialized).
- [x] 4.3 `errors.go` — sentinel errors per the design's mapping table.
- [x] 4.4 `token.go` — JWT mint/verify (HS256), opaque refresh generation, SHA-256 hashing.
- [x] 4.5 `repository.go` — hand-written `Repository` interface + `PostgresRepository` wrapping
      `*authdb.Queries` (CreateUser, GetUserByEmail, GetUserByID, CreateSession, GetSessionByHash,
      RevokeSession, recordLoginAttempt, writeAudit); atomic flows via `pool.Begin`/`WithTx`; map
      generated types → domain `model.go` types at the boundary.
- [x] 4.6 `service.go` — Register, Login, Refresh, Logout, Authenticate; bcrypt; rotation; attempt +
      audit recording; no-enumeration credential errors.
- [x] 4.7 `handler.go` — five endpoints, cookie set/clear, `Register(r)`, error→status mapping.
- [x] 4.8 `middleware.go` — `Middleware(...)` + `WithUser` / `UserFromContext`.

## 5. Wiring
- [x] 5.1 `internal/app/routes.go` — `AllowCredentials: true`; split `NewRouter` into public +
      protected (auth-middleware) registrar groups.
- [x] 5.2 `internal/app/app.go` — build auth repo/service/handler/middleware; register `auth` public,
      `saferoutes` protected.

## 6. Tests
- [x] 6.1 `token_test.go` — mint→verify round-trip, expired/wrong-typ/bad-signature rejected.
- [x] 6.2 `service_test.go` — register dup email, login success/failure, refresh rotation, revoked
      reuse rejected, inactive user, logout (fake repository).
- [x] 6.3 `handler_test.go` — status/body + cookie assertions per endpoint (table-driven).
- [x] 6.4 `middleware_test.go` — 401 without/with bad token, success injects user, inactive → 403.
- [x] 6.5 `repository_integration_test.go` (`//go:build integration`) — user + session CRUD, rotation.

## 7. Verify & archive
- [x] 7.1 `go build ./...`, `go test ./...`, `go test -tags=integration ./internal/auth/...`.
- [x] 7.2 End-to-end curl: register → login (capture cookie + token) → `/routes/safe` 401 without
      token, 200 with token → refresh (new token, rotated cookie) → logout → refresh now 401.
- [x] 7.3 Update `.env.example`, `CLAUDE.md` (new domain + env vars + protected route), and the
      "Not yet implemented" / milestones tables.
- [x] 7.4 Archive the change; merge deltas into `openspec/specs/auth/` and `specs/safe-routes/`.
