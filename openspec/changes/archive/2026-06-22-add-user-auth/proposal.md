# Add user accounts and JWT authentication

## Why

The API is fully public today. To test CABA Rutas Seguras as a real product — and to lay the
foundation for the planned user features (saved routes, avoid-points, community reports) — we need
accounts and authentication. The first concrete need is to gate `GET /api/v1/routes/safe` behind a
logged-in user, so safe-route requests are attributable and the public surface shrinks to what is
genuinely open data.

The goal is **simple but solid**: email + password accounts, short-lived JWT access tokens, and an
opaque, rotating refresh token stored hashed in Postgres — standard, well-understood building blocks,
no third-party identity provider.

## What changes

A new `auth` capability under `internal/auth/`, following the project's layered structure:

1. **Accounts** — `POST /api/v1/auth/register` and an `users` table (email unique, bcrypt password
   hash, `is_active`).
2. **Login** — `POST /api/v1/auth/login` verifies the password, returns a 15-minute JWT access token
   in the body, and sets an HttpOnly refresh-token cookie (7 days). The opaque refresh token is
   stored **only as a hash** in `refresh_sessions`.
3. **Refresh** — `POST /api/v1/auth/refresh` reads the cookie, validates the active session, **rotates**
   the refresh token (revokes the old, issues a new one, links them via `replaced_by`), and returns a
   fresh access token.
4. **Logout** — `POST /api/v1/auth/logout` revokes the current refresh session and clears the cookie.
5. **Current user** — `GET /api/v1/auth/me` returns the authenticated user.
6. **Auth middleware** — validates the `Authorization: Bearer <jwt>` access token, confirms
   `typ=access`, loads the user, rejects inactive users, and injects the current user into the
   request context.
7. **Protect `GET /api/v1/routes/safe`** — it now requires a valid access token (401 otherwise).
8. **Observability** — `login_attempts` and `audit_logs` tables record auth events (success/failure,
   logout, refresh) for later rate-limiting and auditing.

New dependencies: `golang.org/x/crypto/bcrypt` (password hashing) and `github.com/golang-jwt/jwt/v5`
(access tokens). New migration `000011_create_auth_tables`. New env vars for the JWT secret, token
TTLs, and cookie behaviour.

## In scope

- The five `/auth/*` endpoints, the auth middleware, and gating `/routes/safe`.
- `users`, `refresh_sessions`, `login_attempts`, `audit_logs` tables (migration 000011).
- CORS change to `AllowCredentials: true` (required for the refresh cookie) and router wiring for a
  protected route group.
- Unit tests (service, handler, token, middleware) and `//go:build integration` repository tests.

## Out of scope

- Roles / authorization beyond "is this a valid, active user" (no admin vs user).
- Rate limiting on `/auth/login` (the `login_attempts` table is added now so it can be layered on
  later without a migration).
- Email verification, password reset, OAuth / social login, refresh-token reuse-detection lockout
  (rotation + `replaced_by` is recorded, but automatic family revocation on reuse is deferred).
- Protecting the other public endpoints (`/crimes/nearby`, `/routes`, `/roadgraph/*`) — they stay
  public as open data for now; only `/routes/safe` is gated.
- A frontend.
