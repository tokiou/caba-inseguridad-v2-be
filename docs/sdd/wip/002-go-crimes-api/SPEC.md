# SPEC — Go Crimes API + MongoDB Geospatial Query

## Goal

Implement the first Go backend milestone for the CABA unsafe routes project.

This milestone validates that the Go API can:

1. Start an HTTP server.
2. Connect to MongoDB.
3. Expose a health check endpoint.
4. Query nearby crimes using MongoDB geospatial queries.

---

## Branch
feat/go-crimes-api


---

## Scope

### Included

- Initial Go backend structure.
- Environment-based configuration.
- MongoDB connection.
- HTTP routing with `chi`.
- `GET /api/v1/health` endpoint.
- `GET /api/v1/crimes/nearby` endpoint.
- MongoDB geospatial query using `location`.
- Basic query parameter validation.
- Consistent JSON responses.

### Not Included

- OpenRouteService integration.
- Safe route calculation.
- Route risk scoring.
- Frontend.
- Authentication.
- Aggregated statistics.
- ETL changes.

---

## Expected Structure

```txt
caba-inseguridad-routes-go/
  cmd/
    api/
      main.go

  internal/
    app/
      app.go
      routes.go

    config/
      config.go

    platform/
      mongo/
        client.go

    health/
      handler.go

    crimes/
      model.go
      dto.go
      handler.go
      service.go
      repository.go
      mongo_repository.go

  etl/
    python/

  data/
    raw/
    processed/

  go.mod
  go.sum
  .env.example
  README.md
  AGENTS.md
```

---

## Environment Variables

Add or validate `.env.example`:

```env
APP_ENV=development
HTTP_PORT=8080

MONGO_URI=mongodb://localhost:27017
MONGO_DATABASE=caba_routes
MONGO_CRIMES_COLLECTION=crimes
```

---

## Dependencies

Use:

```bash
go get github.com/go-chi/chi/v5
go get go.mongodb.org/mongo-driver/mongo
go get go.mongodb.org/mongo-driver/bson
go get go.mongodb.org/mongo-driver/mongo/options
go get github.com/joho/godotenv
```

---

## Architecture

Expected request flow:

```txt
HTTP request
   ↓
chi router
   ↓
handler
   ↓
service
   ↓
repository interface
   ↓
mongo repository
   ↓
MongoDB
```

Architecture rules:

- Handlers must not query MongoDB directly.
- Services must not know MongoDB implementation details.
- Repositories must encapsulate data access.
- `main.go` must only wire dependencies and start the server.

---

## `cmd/api/main.go`

Responsibilities:

1. Load configuration.
2. Create base context.
3. Connect to MongoDB.
4. Create repositories.
5. Create services.
6. Create handlers.
7. Create router.
8. Start HTTP server.
9. Close MongoDB connection gracefully on shutdown.

`main.go` must not contain business logic.

---

## `internal/config/config.go`

Expose a `Config` struct:

```go
type Config struct {
    AppEnv                string
    HTTPPort              string
    MongoURI              string
    MongoDatabase         string
    MongoCrimesCollection string
}
```

Read configuration from environment variables.

Acceptable defaults:

```txt
APP_ENV=development
HTTP_PORT=8080
MONGO_URI=mongodb://localhost:27017
MONGO_DATABASE=caba_routes
MONGO_CRIMES_COLLECTION=crimes
```

---

## `internal/platform/mongo/client.go`

Responsibility:

```txt
Create and validate the MongoDB connection.
```

Expected function:

```go
func NewClient(ctx context.Context, uri string) (*mongo.Client, error)
```

It must:

1. Call `mongo.Connect`.
2. Ping MongoDB.
3. Return the connected client.

---

## `internal/app/routes.go`

Register routes using `chi`:

```txt
GET /api/v1/health
GET /api/v1/crimes/nearby
```

Conceptually:

```go
r.Route("/api/v1", func(r chi.Router) {
    r.Get("/health", healthHandler.Check)
    r.Get("/crimes/nearby", crimesHandler.GetNearby)
})
```

---

## Endpoint — Health Check

### Request

```http
GET /api/v1/health
```

### Response `200`

```json
{
  "status": "ok"
}
```

---

## Endpoint — Nearby Crimes

### Request

```http
GET /api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=300
```

### Query Parameters

| Param | Required | Type | Description |
|---|---:|---|---|
| `lat` | yes | float | Latitude |
| `lng` | yes | float | Longitude |
| `radius` | no | int | Radius in meters |

Default:

```txt
radius = 300
```

Validation rules:

```txt
lat must be between -35 and -34
lng must be between -59 and -58
radius must be greater than 0
radius must be less than or equal to 2000
```

If `radius` is missing, use `300`.

If `radius` is greater than `2000`, return `400`.

---

### Response `200`

```json
{
  "lat": -34.5895,
  "lng": -58.4201,
  "radius_meters": 300,
  "count": 2,
  "items": [
    {
      "source_id": "123456",
      "year": 2024,
      "month": 5,
      "day": 17,
      "date": "2024-05-17",
      "hour": 18,
      "crime_type": "ROBO",
      "crime_subtype": "ROBO TOTAL",
      "weapon_used": true,
      "motorcycle_used": false,
      "neighborhood": "PALERMO",
      "commune": 14,
      "quantity": 1,
      "location": {
        "type": "Point",
        "coordinates": [-58.4201, -34.5895]
      }
    }
  ]
}
```

---

### Response `400`

For invalid coordinates:

```json
{
  "error": "invalid_request",
  "message": "lat and lng are required and must be valid CABA coordinates"
}
```

For invalid radius:

```json
{
  "error": "invalid_request",
  "message": "radius must be between 1 and 2000 meters"
}
```

---

### Response `500`

For internal errors:

```json
{
  "error": "internal_error",
  "message": "could not fetch nearby crimes"
}
```

Do not expose internal MongoDB errors to the client.

---

## Domain Model

File:

```txt
internal/crimes/model.go
```

Expected model:

```go
type Crime struct {
    SourceID       string   `json:"source_id" bson:"source_id"`
    Year           int      `json:"year" bson:"year"`
    Month          int      `json:"month" bson:"month"`
    Day            int      `json:"day" bson:"day"`
    Date           string   `json:"date" bson:"date"`
    Hour           *int     `json:"hour" bson:"hour"`
    CrimeType      string   `json:"crime_type" bson:"crime_type"`
    CrimeSubtype   *string  `json:"crime_subtype" bson:"crime_subtype"`
    WeaponUsed     *bool    `json:"weapon_used" bson:"weapon_used"`
    MotorcycleUsed *bool    `json:"motorcycle_used" bson:"motorcycle_used"`
    Neighborhood   *string  `json:"neighborhood" bson:"neighborhood"`
    Commune        *int     `json:"commune" bson:"commune"`
    Quantity       int      `json:"quantity" bson:"quantity"`
    Location       GeoJSON  `json:"location" bson:"location"`
}

type GeoJSON struct {
    Type        string    `json:"type" bson:"type"`
    Coordinates []float64 `json:"coordinates" bson:"coordinates"`
}
```

Nullable JSON fields should be represented with pointers:

```go
*int
*bool
*string
```

---

## DTOs

File:

```txt
internal/crimes/dto.go
```

Internal query DTO:

```go
type NearbyCrimesQuery struct {
    Lat          float64
    Lng          float64
    RadiusMeters int
}
```

Response DTO:

```go
type NearbyCrimesResponse struct {
    Lat          float64 `json:"lat"`
    Lng          float64 `json:"lng"`
    RadiusMeters int     `json:"radius_meters"`
    Count        int     `json:"count"`
    Items        []Crime `json:"items"`
}
```

---

## Repository Interface

File:

```txt
internal/crimes/repository.go
```

Define the repository interface:

```go
type Repository interface {
    FindNearby(ctx context.Context, query NearbyCrimesQuery) ([]Crime, error)
}
```

---

## Mongo Repository

File:

```txt
internal/crimes/mongo_repository.go
```

Implement:

```go
func (r *MongoRepository) FindNearby(ctx context.Context, query NearbyCrimesQuery) ([]Crime, error)
```

Expected MongoDB query:

```js
{
  location: {
    $nearSphere: {
      $geometry: {
        type: "Point",
        coordinates: [lng, lat]
      },
      $maxDistance: radius
    }
  }
}
```

Important:

```txt
MongoDB GeoJSON coordinates must use [longitude, latitude].
```

Do not use:

```txt
[latitude, longitude]
```

When using `$nearSphere`, MongoDB naturally returns documents ordered by proximity.

---

## Service

File:

```txt
internal/crimes/service.go
```

Responsibility:

```txt
Validate business rules and call the repository.
```

It must validate:

```txt
lat/lng are valid approximate CABA coordinates
radius is between 1 and 2000
radius defaults to 300 when missing
```

Expected method:

```go
func (s *Service) GetNearby(ctx context.Context, query NearbyCrimesQuery) (NearbyCrimesResponse, error)
```

---

## Handler

File:

```txt
internal/crimes/handler.go
```

Responsibilities:

1. Read HTTP query parameters.
2. Convert string params to typed values.
3. Call the service.
4. Return JSON responses.

Read these query params:

```txt
lat
lng
radius
```

Example:

```http
GET /api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=300
```

---

## Error Handling

Use consistent JSON responses.

Error format:

```json
{
  "error": "invalid_request",
  "message": "..."
}
```

Suggested error codes:

```txt
invalid_request
internal_error
```

Do not expose internal MongoDB error details to the client.

---

## Tests

Minimum expected tests for this branch:

- Service validates invalid coordinates.
- Service validates invalid radius.
- Service applies default radius.
- Handler returns `400` when `lat` is missing.
- Handler returns `400` when `lng` is missing.
- Handler returns `400` when `radius` is not a number.

Optional:

- Repository integration test with local MongoDB.
- Handler tests using `httptest`.

For this milestone, prioritize:

```txt
service tests + handler tests
```

---

## Acceptance Criteria

This branch is complete when:

- `go run ./cmd/api` starts the server.
- `GET /api/v1/health` returns `200`.
- The app connects successfully to MongoDB.
- `GET /api/v1/crimes/nearby` queries real MongoDB data.
- The MongoDB query uses `location.coordinates` as `[lng, lat]`.
- The endpoint returns `count` and `items`.
- Invalid query params return `400`.
- Internal errors return `500`.
- There is no MongoDB logic inside handlers.
- There is no HTTP logic inside repositories.
- Main service and handler tests pass.

---

## Manual Test Commands

Start the app:

```bash
go run ./cmd/api
```

Health check:

```bash
curl "http://localhost:8080/api/v1/health"
```

Nearby crimes:

```bash
curl "http://localhost:8080/api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=300"
```

Default radius:

```bash
curl "http://localhost:8080/api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201"
```

Invalid coordinates:

```bash
curl "http://localhost:8080/api/v1/crimes/nearby?lat=0&lng=0&radius=300"
```

---

## Agent Prompt

```txt
Implement the first Go backend milestone for the CABA unsafe routes project.

Branch: feat/go-crimes-api

Scope:
- Build the initial Go backend skeleton.
- Use chi as HTTP router.
- Connect to MongoDB.
- Expose health check.
- Expose nearby crimes endpoint.
- Do not implement OpenRouteService.
- Do not implement safe route calculation.
- Do not implement frontend.
- Do not modify the ETL pipeline.

Expected structure:
cmd/api/main.go
internal/app/app.go
internal/app/routes.go
internal/config/config.go
internal/platform/mongo/client.go
internal/health/handler.go
internal/crimes/model.go
internal/crimes/dto.go
internal/crimes/handler.go
internal/crimes/service.go
internal/crimes/repository.go
internal/crimes/mongo_repository.go

Environment variables:
APP_ENV=development
HTTP_PORT=8080
MONGO_URI=mongodb://localhost:27017
MONGO_DATABASE=caba_routes
MONGO_CRIMES_COLLECTION=crimes

Endpoints:
GET /api/v1/health
GET /api/v1/crimes/nearby?lat=-34.5895&lng=-58.4201&radius=300

Nearby crimes behavior:
- lat and lng are required.
- radius is optional and defaults to 300.
- lat must be between -35 and -34.
- lng must be between -59 and -58.
- radius must be between 1 and 2000.
- Query MongoDB using $nearSphere.
- MongoDB GeoJSON coordinates must use [longitude, latitude].
- Return lat, lng, radius_meters, count and items.

Architecture rules:
- Handler reads HTTP params and returns JSON.
- Service validates business rules and calls repository.
- Repository interface hides MongoDB.
- Mongo repository implements MongoDB query.
- main.go only wires dependencies and starts the server.

Testing:
- Add service tests for invalid coordinates, invalid radius and default radius.
- Add handler tests for missing lat/lng and invalid radius.
```