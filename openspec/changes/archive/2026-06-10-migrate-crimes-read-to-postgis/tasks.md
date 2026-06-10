# Tasks — Migrate crimes read-path to PostgreSQL + PostGIS

1. Add `github.com/jackc/pgx/v5` dependency.
2. Create `internal/platform/postgres/pool.go` — `NewPool(ctx, dsn) (*pgxpool.Pool, error)` with ping.
3. Rewrite `internal/crimes/repository.go` — keep `Repository` interface; add `PostgresRepository`
   + `NewRepository(pool)` + `FindNearby` (PostGIS `ST_DWithin`, map rows → `Crime`).
4. Delete `internal/crimes/mongo_repository.go`.
5. Remove bson tags from `internal/crimes/model.go` (Mongo gone).
6. `internal/config/config.go` — drop Mongo fields; add `DatabaseURL`.
7. `internal/app/app.go` — wire pgx pool + `crimes.NewRepository`; `App.Close` closes the pool.
8. `cmd/api/main.go` — update the shutdown close log message (no longer "mongo").
9. Delete `internal/platform/mongo/` and run `go mod tidy` (drops mongo driver).
10. Update `.env.example` and `.env` — Postgres vars instead of Mongo.
11. Add `internal/crimes/repository_integration_test.go` (`//go:build integration`).
12. `go build ./...`; `go test ./...` (unit, green); `go test -tags=integration ./internal/crimes/...`
    against the live PostGIS.
13. Update `CLAUDE.md` (stack, layout, architecture rules, env, ETL, spec-first rule) and
    `openspec/project.md` (data-access split).
14. Archive: merge the delta into `openspec/specs/crimes-api/spec.md`; move this change folder to
    `openspec/changes/archive/2026-06-10-migrate-crimes-read-to-postgis/`.
