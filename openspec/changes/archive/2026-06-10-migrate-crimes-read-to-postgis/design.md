# Design — Migrate crimes read-path to PostgreSQL + PostGIS

## Data-access split: pgx for PostGIS, sqlc for CRUD

Decision: **pgx (jackc/pgx v5) owns all PostGIS/geospatial queries; sqlc owns relational CRUD.**

- sqlc's analyzer does not know PostGIS functions (`ST_DWithin`, `ST_X`, `ST_MakePoint`, …); they are
  not in its catalog and break `sqlc generate`. The crimes proximity query is pure PostGIS, so it is
  written with pgx (raw SQL + manual scan).
- sqlc is **not introduced in this change** because there is no relational CRUD yet. It will be added
  with the future users / saved-routes capability, where it removes scan boilerplate and gives
  compile-time-checked queries. Both tools sit on top of pgx and coexist cleanly.

This split is recorded as a project convention (CLAUDE.md + `openspec/project.md`).

## Repository file layout

Per the request, there is no `pg_repository.go`. With a single datastore, the `Repository` interface
and its one concrete Postgres implementation both live in `internal/crimes/repository.go`. The struct
is named `PostgresRepository` (documents the backing store; mirrors the old `MongoRepository`).

## Preserve the HTTP contract

The `Repository` interface is unchanged:

```go
FindNearby(ctx context.Context, query NearbyCrimesQuery) ([]Crime, error)
```

So `service.go`, `dto.go`, `handler.go`, and their mock-based unit tests are untouched. The Postgres
repository maps each row back into the existing `Crime` domain struct, including
`Location = { type: "Point", coordinates: [lng, lat] }`. Response shape is identical to the Mongo era.

### Query

```sql
SELECT source_id, year, month, day,
       to_char(date, 'YYYY-MM-DD') AS date,
       hour, crime_type, crime_subtype, weapon_used, motorcycle_used,
       neighborhood, commune, quantity,
       ST_X(geom) AS longitude, ST_Y(geom) AS latitude
FROM crimes
WHERE ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($1,$2),4326)::geography, $3)
ORDER BY ST_Distance(geom::geography, ST_SetSRID(ST_MakePoint($1,$2),4326)::geography) ASC;
```

- `$1 = lng`, `$2 = lat`, `$3 = radius_meters`. Coordinate order `[lng, lat]` — never swapped.
- `to_char(date,'YYYY-MM-DD')` keeps `Crime.Date` a string identical to the previous output (pgx would
  otherwise decode `DATE` as `time.Time`).
- No `LIMIT`: matches the previous Mongo `$nearSphere`/`$maxDistance` behavior (all matches within the
  radius, nearest first). A future limit/pagination is noted but out of scope.

## Removing MongoDB from the Go app

- Delete `internal/crimes/mongo_repository.go` and `internal/platform/mongo/client.go`.
- `config.go`: drop `MongoURI/MongoDatabase/MongoCrimesCollection`; add `DatabaseURL`
  (default `postgres://postgres:postgres@localhost:5434/caba_routes?sslmode=disable`).
- `app.go`: build a `*pgxpool.Pool` via `internal/platform/postgres`, inject into
  `crimes.NewRepository`; `App.Close` closes the pool.
- `go mod tidy` removes `go.mongodb.org/mongo-driver/v2` and adds `github.com/jackc/pgx/v5`.

## Testing

- Unit tests (`service_test.go`, `handler_test.go`) use mocks of the unchanged interfaces → stay green
  with no edits.
- New `repository_integration_test.go` guarded by `//go:build integration`: hits the live PostGIS
  (`DATABASE_URL`, default `localhost:5434`), runs `FindNearby` near the Obelisco, asserts results are
  within CABA bounds and ordered by distance. Excluded from the default `go test ./...` so CI without a
  DB stays green; run with `go test -tags=integration ./internal/crimes/...`.
