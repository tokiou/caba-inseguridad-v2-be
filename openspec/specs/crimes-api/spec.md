# Crimes API Specification

## Purpose

Expose nearby crimes for a given point in CABA via an HTTP endpoint, using a geospatial proximity
query. This is the read API over the crime dataset produced by the data pipeline.

> Current data access: **MongoDB `$nearSphere`** (`internal/crimes/mongo_repository.go`). Migration
> of this read path to PostgreSQL + PostGIS (`ST_DWithin`) is planned but not yet implemented — the
> data pipeline already loads PostGIS, the Go read path does not yet consume it.

## Requirements

### Requirement: Nearby crimes endpoint

The API SHALL expose `GET /api/v1/crimes/nearby` accepting `lat` and `lng` query parameters and an
optional `radius` (meters), returning crimes near the point ordered by proximity.

#### Scenario: Successful nearby query

- GIVEN valid CABA coordinates `lat` and `lng`
- WHEN a client sends `GET /api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=300`
- THEN the response is HTTP 200
- AND the body contains `lat`, `lng`, `radius_meters`, `count`, and `items`
- AND items are crime records ordered nearest-first

#### Scenario: Default radius

- GIVEN a request with no `radius` parameter
- WHEN the endpoint is queried
- THEN a default radius of 300 meters is applied

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

The data layer SHALL query using GeoJSON `[longitude, latitude]` order.

#### Scenario: Coordinates passed as [lng, lat]

- GIVEN the API receives `lat` and `lng`
- WHEN it builds the geospatial query
- THEN it passes coordinates as `[longitude, latitude]`, never `[latitude, longitude]`

### Requirement: Internal errors are not leaked

Data-layer failures SHALL surface as a generic HTTP 500 without exposing datastore internals.

#### Scenario: Datastore failure

- GIVEN the underlying query fails
- WHEN the endpoint is queried
- THEN the response is HTTP 500 with error code `internal_error`
- AND no datastore error detail appears in the response body

### Requirement: Layered architecture for the read path

The request flow SHALL be `handler → service → repository interface → concrete repository`. Handlers
MUST NOT contain data-access logic; repositories MUST NOT contain HTTP logic.

#### Scenario: Layer boundaries respected

- WHEN the crimes endpoint is implemented or modified
- THEN HTTP parsing lives only in the handler, validation in the service, and data access behind the
  `Repository` interface
