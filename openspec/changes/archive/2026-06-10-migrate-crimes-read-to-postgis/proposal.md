# Proposal — Migrate crimes read-path to PostgreSQL + PostGIS

## Why

The ETL already loads crimes into PostgreSQL + PostGIS (capability `data-pipeline`), but the Go read
path (`GET /api/v1/crimes/nearby`) still queries MongoDB via `$nearSphere`. The system runs two
datastores for the same data, and MongoDB is now redundant for the main path. This change cuts the
crimes read-path over to PostGIS and removes MongoDB from the Go application entirely.

## What

- Replace the MongoDB-backed crimes repository with a PostgreSQL + PostGIS implementation using
  **pgx** (`ST_DWithin` proximity query, ordered by distance).
- Remove MongoDB from the Go app: delete `mongo_repository.go` and `internal/platform/mongo`, drop
  the mongo driver dependency, and switch config/wiring to a pgx connection pool.
- Keep the existing HTTP contract of `GET /api/v1/crimes/nearby` **unchanged** (same request params
  and same JSON response shape). This is a datastore migration, not an API redesign.

## In scope

- `internal/crimes`: concrete Postgres repository (in `repository.go`; no separate `pg_repository.go`).
- `internal/platform/postgres`: pgx connection pool.
- Config, app wiring, `.env(.example)`, `go.mod` cleanup.
- Tests: existing unit tests (mock-based) stay green; add a build-tagged integration test against a
  live PostGIS.

## Out of scope

- sqlc (it will be introduced for the relational CRUD of a future users / saved-routes capability;
  see design.md — pgx owns PostGIS, sqlc owns CRUD).
- Changing the `/crimes/nearby` response shape (e.g. exposing `distance_meters`) — possible later.
- Users / authentication / saved routes / risk scoring.
- Removing the Docker MongoDB container's data volume (already stopped, kept for now).
