# Crimes API — Delta

## MODIFIED Requirements

### Requirement: Geospatial coordinate order

The data layer SHALL query **PostgreSQL + PostGIS** using `ST_DWithin` over the `geom`
`GEOMETRY(Point, 4326)` column, with coordinates passed in `[longitude, latitude]` order, returning
matches within the radius ordered nearest-first. The read path uses **pgx** (raw SQL); it no longer
uses MongoDB.

#### Scenario: Proximity query uses [lng, lat]

- GIVEN the API receives `lat` and `lng`
- WHEN the repository builds the PostGIS query
- THEN it calls `ST_DWithin(geom::geography, ST_SetSRID(ST_MakePoint($lng,$lat),4326)::geography, $radius)`
- AND results are ordered by `ST_Distance` ascending (nearest first)
- AND `ST_X(geom)` maps to longitude and `ST_Y(geom)` to latitude in the returned records

#### Scenario: Response shape unchanged

- WHEN a valid nearby query succeeds
- THEN the JSON response is identical to before the migration: `lat`, `lng`, `radius_meters`, `count`,
  and `items` (each item a crime with `location` as a GeoJSON Point `[lng, lat]`)

### Requirement: Layered architecture for the read path

The request flow SHALL be `handler → service → repository interface → PostgresRepository → PostgreSQL/PostGIS`.
Handlers MUST NOT contain data-access logic; the repository MUST NOT contain HTTP logic. PostGIS /
geospatial access uses pgx; relational CRUD (future capabilities) uses sqlc.

#### Scenario: Layer boundaries respected

- WHEN the crimes endpoint is implemented or modified
- THEN HTTP parsing lives only in the handler, validation in the service, and PostGIS access behind the
  `Repository` interface implemented by `PostgresRepository`
