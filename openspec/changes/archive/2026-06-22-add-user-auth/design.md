# Design — user accounts and JWT authentication

## Token model

| Token | Type | TTL | Transport | Storage |
|-------|------|-----|-----------|---------|
| Access | JWT (HS256) | 15 min | response body → `Authorization: Bearer` | stateless, not stored |
| Refresh | opaque random (32 bytes, base64url) | 7 days | HttpOnly cookie | **hash only** in `refresh_sessions` |

**Access token (JWT, HS256, signed with `JWT_SECRET`).** Claims: `sub` (user id), `typ` `"access"`,
`iat`, `exp`, `jti` (uuid). The middleware verifies the signature, `exp`, and `typ == "access"`, then
loads the user. Short TTL keeps the stateless token's blast radius small — there is no access-token
revocation list; logout works by revoking the refresh session so no new access token can be minted.

**Refresh token (opaque).** 32 bytes from `crypto/rand`, base64url-encoded. It is **not** a JWT — it
carries no claims and is only a lookup key. We store `sha256(token)` in `refresh_sessions.token_hash`
(UNIQUE). SHA-256 (not bcrypt) is correct here: the token is high-entropy random, so a fast hash is
both safe and lets us look the session up by hash in one indexed query. Passwords, which are
low-entropy, use bcrypt instead.

## Refresh rotation

Every successful `/auth/refresh` **rotates** the token: the presented session is marked
`revoked_at = now()`, a new session row is created, the old row's `replaced_by` points at the new one,
and a new cookie is set. This bounds the lifetime of any single refresh token and leaves an auditable
chain. Reuse of an already-revoked token is rejected (401); automatic revocation of the whole token
family on reuse (theft detection) is recorded-but-not-enforced now and deferred (see proposal
out-of-scope).

## Password hashing

`golang.org/x/crypto/bcrypt`, default cost 12. bcrypt embeds its own salt and cost in the hash string,
so `password_hash` is a single `TEXT` column and verification is `bcrypt.CompareHashAndPassword`.
(The spec allows argon2id; bcrypt is chosen for simplicity and because it needs no tuning struct.)

## Cookie

```
Set-Cookie: <REFRESH_COOKIE_NAME>=<token>; HttpOnly; Secure; SameSite=Lax;
            Path=/api/v1/auth; Max-Age=604800
```

Two deviations from the original spec, both deliberate:

- **`Path=/api/v1/auth`** (not `/auth/refresh`). All routes are mounted under `/api/v1`, and
  `/auth/logout` must also receive the cookie to revoke the session. Scoping to `/api/v1/auth` covers
  both `refresh` and `logout` while keeping the cookie off every other endpoint.
- **`Secure` is env-gated** via `COOKIE_SECURE` (default `false` for local HTTP dev, `true` in prod).
  `SameSite` comes from `COOKIE_SAMESITE` (`lax` default).

## CORS (required change)

The refresh cookie is useless cross-origin unless CORS allows credentials. `internal/app/routes.go`
must change `AllowCredentials: false → true`. With credentials enabled the allowed origin must stay a
concrete value (`http://localhost:8081`) — the `*` wildcard is invalid with credentials. `Set-Cookie`
flows on the login/refresh responses; the browser must call these with `fetch(..., {credentials:
"include"})`.

## Router wiring — protected group

Today `NewRouter` registers every `Registrar` flat under `/api/v1`. To gate only `/routes/safe` while
keeping handlers ignorant of auth, split registration into public and protected groups:

```go
r.Route("/api/v1", func(r chi.Router) {
    for _, reg := range publicRegistrars { reg.Register(r) }   // health, crimes, routes, roadgraph, auth
    r.Group(func(r chi.Router) {
        r.Use(authMiddleware)                                  // auth.Middleware(...)
        for _, reg := range protectedRegistrars { reg.Register(r) } // saferoutes
    })
})
```

`NewRouter`'s signature changes to take the auth middleware plus the two registrar lists. `app.New`
decides membership: `auth`'s own endpoints are public (you must be able to log in without a token);
`saferoutes` is protected. The auth middleware is a `func(http.Handler) http.Handler` exported from
`internal/auth` and is constructed with the user repository + JWT verifier.

The current-user is passed down via `context`: `auth.WithUser(ctx, user)` /
`auth.UserFromContext(ctx)`, mirroring how request-id is threaded. `/auth/me` and any future
user-scoped handler read it from there.

## Data access — sqlc (decided)

Per `project.md` / `CLAUDE.md`, sqlc owns relational CRUD and is **introduced with the users
capability**. The auth tables are purely relational (no PostGIS), so sqlc fits the "pgx for
PostGIS/geospatial, sqlc for relational CRUD" rule exactly. This change is the first sqlc usage in the
repo.

Setup:

- `sqlc.yaml` at repo root, engine `postgresql`, `sql_package: pgx/v5`, `emit_pointers_for_null_types`
  on so nullable columns map to pointers.
- **Schema source** = the existing `migrations/*.up.sql` (sqlc reads them as DDL). **Queries** live in
  `internal/auth/db/queries/auth.sql` (annotated `-- name: CreateUser :one`, etc.).
- **Generated code** → `internal/auth/db/` (package `authdb`): `Queries`, `Querier`, models, params.
  Codegen via `go run github.com/sqlc-dev/sqlc/cmd/sqlc generate` (pinned tool dependency — no global
  install required), wired so `go generate ./...` regenerates it.
- The auth `PostgresRepository` wraps `*authdb.Queries` (constructed from the `*pgxpool.Pool`, which
  satisfies sqlc's `DBTX`). The domain `Repository` interface stays hand-written and pgx-agnostic;
  sqlc lives behind it. Multi-statement flows that must be atomic (login session-create + audit,
  refresh rotation) use `pool.Begin` + `qtx := queries.WithTx(tx)`.

Generated `authdb` types are mapped to domain `model.go` types at the repository boundary so the rest
of the codebase never imports the generated package.

## Tables (migration 000011)

`gen_random_uuid()` (built into PG13+) supplies UUID primary keys; no app-side uuid generation needed
for inserts (`RETURNING id`). Schema follows the user's spec, with indexes added for the hot lookups:

- `users(id, email UNIQUE, password_hash, is_active, created_at, updated_at)`.
- `refresh_sessions(id, user_id FK, token_hash UNIQUE, user_agent, ip_address INET, created_at,
  expires_at, revoked_at NULL, replaced_by FK NULL)` — index on `user_id`; lookups are by
  `token_hash`.
- `login_attempts(id, email, ip_address, success, reason, created_at)` — index on `(email, created_at)`.
- `audit_logs(id, user_id FK NULL, action, metadata JSONB, ip_address, user_agent, created_at)`.

A partial index `WHERE revoked_at IS NULL` on `refresh_sessions` keeps active-session lookups cheap.

## Domain layout (`internal/auth/`)

```
model.go        User, RefreshSession, enums (audit actions)
dto.go          RegisterRequest/Response, LoginRequest/Response, MeResponse
errors.go       ErrEmailTaken, ErrInvalidCredentials, ErrInactiveUser,
                ErrInvalidToken, ErrSessionNotFound/Revoked/Expired, ErrEmailRequired, ...
token.go        JWT mint/verify (HS256) + opaque refresh generation + sha256 hashing
repository.go   Repository interface + PostgresRepository (users, refresh_sessions,
                login_attempts, audit_logs)
service.go      Register/Login/Refresh/Logout/Authenticate orchestration, bcrypt,
                rotation, attempt + audit recording
handler.go      five endpoints, cookie set/clear, error→status mapping; Register(r)
middleware.go   Middleware(repo, verifier) + context helpers WithUser/UserFromContext
*_test.go       unit (service/handler/token/middleware) + integration (repository)
```

Error→status mapping (handler), consistent with `httpx`:

| Error | Status | code |
|-------|--------|------|
| `ErrEmailRequired` / invalid body / weak password | 400 | `invalid_request` |
| `ErrEmailTaken` | 409 | `email_taken` |
| `ErrInvalidCredentials` | 401 | `invalid_credentials` |
| `ErrInactiveUser` | 403 | `account_inactive` |
| missing/invalid/expired access token (middleware) | 401 | `unauthorized` |
| missing/invalid/revoked/expired refresh session | 401 | `invalid_refresh` |
| unexpected | 500 | `internal_error` |

## Config additions (`internal/config`)

```env
JWT_SECRET=change_me            # required; app refuses to start in non-dev if empty/default
ACCESS_TOKEN_TTL_MINUTES=15
REFRESH_TOKEN_TTL_DAYS=7
REFRESH_COOKIE_NAME=refresh_token
COOKIE_SECURE=false             # true in production
COOKIE_SAMESITE=lax
```

`JWT_SECRET` is validated at startup: empty (or the literal `change_me`) is allowed only when
`APP_ENV=development`, otherwise `app.New` returns an error and the server does not start. The secret
is never logged.

## Security checklist (from the spec)

- bcrypt password hashing; never return `password_hash`.
- Opaque, high-entropy refresh token; only its SHA-256 hash stored.
- HttpOnly + Secure (prod) + SameSite cookie; refresh token never in response body / localStorage.
- Refresh rotation on every use; logout revokes the session and clears the cookie.
- Short access-token TTL.
- `login_attempts` recorded for every login (success and failure) for later rate limiting.
- Constant-time password compare via bcrypt; identical `invalid_credentials` response whether the
  email is unknown or the password is wrong (no user enumeration).
