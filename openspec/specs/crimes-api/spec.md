# Crimes API Specification

## Purpose

Expose nearby crimes for a given point in CABA via an HTTP endpoint, using a geospatial proximity
query. This is the read API over the crime dataset produced by the data pipeline.

> Data access: **PostgreSQL + PostGIS `ST_DWithin`** via pgx
> (`internal/crimes/repository.go`, `PostgresRepository`). MongoDB is no longer used.

## Requirements

### Requirement: Nearby crimes endpoint

The API SHALL expose `GET /api/v1/crimes/nearby` accepting `lat` and `lng`, an optional `radius`
(meters), an optional `limit` (page size), and an optional `cursor`, returning a page of crimes near
the point ordered nearest-first.

#### Scenario: Successful nearby query

- GIVEN valid CABA coordinates `lat` and `lng`
- WHEN a client sends `GET /api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=300&limit=100`
- THEN the response is HTTP 200
- AND the body contains `lat`, `lng`, `radius_meters`, `count`, `items`, `next_cursor`, and `has_more`
- AND `items` holds at most `limit` crime records ordered nearest-first

#### Scenario: Default radius and limit

- GIVEN a request with no `radius` and no `limit`
- WHEN the endpoint is queried
- THEN a default radius of 300 meters and a default limit of 100 are applied

### Requirement: Keyset pagination

The nearby endpoint SHALL page results with a keyset cursor ordered by `(distance, id)`. It accepts a
`limit` (page size, default 100, max 500) and an optional opaque `cursor`. The response SHALL include
`next_cursor` (the opaque token for the next page, `null` when there are no more results) and
`has_more`. `count` SHALL be the number of items in the current page.

#### Scenario: First page

- GIVEN a valid nearby query with `limit=100` and no `cursor`
- WHEN the endpoint is queried and more than 100 matches exist within the radius
- THEN at most 100 items are returned, ordered nearest-first
- AND `has_more` is `true` and `next_cursor` is a non-null opaque token

#### Scenario: Following a cursor

- GIVEN a `next_cursor` returned by a previous page
- WHEN the endpoint is queried again with the same `lat`/`lng`/`radius` and that `cursor`
- THEN the next page continues strictly after the previous page's last item (no duplicates, no gaps)
- AND distances are non-decreasing across page boundaries

#### Scenario: Last page

- WHEN the final page is returned
- THEN `has_more` is `false` and `next_cursor` is `null`

#### Scenario: Invalid limit or cursor

- GIVEN `limit` that is non-numeric, less than 1, or greater than 500, OR a `cursor` that is not a
  valid token
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

### Requirement: Query parameter validation

The service SHALL validate inputs as approximate CABA coordinates and a bounded radius, returning
HTTP 400 for invalid input.

#### Scenario: Coordinates out of CABA bounds

- GIVEN `lat` outside `[-35, -34]` or `lng` outside `[-59, -58]`
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

#### Scenario: Missing or unparseable coordinates

- GIVEN a missing `lat`/`lng` or a value that does not parse as a float
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

#### Scenario: Radius out of range

- GIVEN a `radius` less than 1 or greater than 2000
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

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

### Requirement: Internal errors are not leaked

Data-layer failures SHALL surface as a generic HTTP 500 without exposing datastore internals.

#### Scenario: Datastore failure

- GIVEN the underlying query fails
- WHEN the endpoint is queried
- THEN the response is HTTP 500 with error code `internal_error`
- AND no datastore error detail appears in the response body

### Requirement: Layered architecture for the read path

The request flow SHALL be `handler → service → repository interface → PostgresRepository → PostgreSQL/PostGIS`.
Handlers MUST NOT contain data-access logic; the repository MUST NOT contain HTTP logic. PostGIS /
geospatial access uses pgx; relational CRUD (future capabilities) uses sqlc.

#### Scenario: Layer boundaries respected

- WHEN the crimes endpoint is implemented or modified
- THEN HTTP parsing lives only in the handler, validation in the service, and PostGIS access behind the
  `Repository` interface implemented by `PostgresRepository`
