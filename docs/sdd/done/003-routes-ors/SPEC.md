# Spec 003 — Routes Module (OpenRouteService)

## Goal

Implement the `internal/routes` module to expose an endpoint that computes a route between two points within CABA (Ciudad Autónoma de Buenos Aires). Route calculation is delegated to the OpenRouteService (ORS) external API via HTTP. This is the foundational routing layer — risk scoring and crime-weighted path optimization are explicitly out of scope for this milestone.

## Branch

`feat/routes-ors`

## Scope

### Included

- `GET /api/v1/routes` endpoint with origin, destination, and optional transport profile
- Validation that both points are within CABA bounds (same bounds used by the crimes module)
- Optional `profile` query param (`driving-car` | `foot-walking` | `cycling-regular`), default `driving-car`
- ORS HTTP client (`ors_client.go`) using stdlib `net/http`
- New config fields: `ORS_API_KEY`, `ORS_BASE_URL`
- Unit tests for service and handler layers

### Not Included

- Crime risk scoring or safe route weighting
- Route caching
- Multi-waypoint routes
- Coordinates outside CABA
- Reverse geocoding or address resolution

---

## Expected Structure

```
internal/routes/
  model.go        ← Route, GeoJSONLineString, Waypoint structs
  dto.go          ← RouteQuery, RouteResponse
  errors.go       ← sentinel errors
  ors_client.go   ← ORS HTTP client (implements routingClient)
  service.go      ← routingClient interface, CABA validation, defaults
  handler.go      ← GET /api/v1/routes, implements app.Registrar
  service_test.go
  handler_test.go
```

---

## Environment Variables

New variables to add to `internal/config/config.go` and `.env.example`:

```env
ORS_API_KEY=              # required — no default, server fails to start if empty
ORS_BASE_URL=https://api.openrouteservice.org  # default provided
```

---

## Dependencies

No new Go module dependencies. Uses stdlib `net/http` for the ORS HTTP client.

---

## Architecture

```
GET /api/v1/routes
       ↓
   Handler          (parse & validate HTTP params, map errors to HTTP codes)
       ↓
   Service          (routingClient interface, CABA validation, defaults)
       ↓
   ORSClient        (HTTP GET → ORS API, decode GeoJSON response)
       ↓
 OpenRouteService   (external API)
```

**Layer rules:**
- Handler parses HTTP params and returns JSON. No external API calls, no business logic.
- Service defines and owns the `routingClient` interface, validates domain rules, calls the client.
- `ORSClient` encapsulates all ORS HTTP communication. No handler logic.
- `main.go` only wires dependencies.

---

## Config — `internal/config/config.go`

Add to the `Config` struct:

```go
ORSAPIKey  string
ORSBaseURL string
```

Add to `Load()`:

```go
ORSAPIKey:  getEnv("ORS_API_KEY", ""),
ORSBaseURL: getEnv("ORS_BASE_URL", "https://api.openrouteservice.org"),
```

---

## Endpoint — Get Route

### Request

```
GET /api/v1/routes?origin_lat=-34.6037&origin_lng=-58.3816&dest_lat=-34.5895&dest_lng=-58.4201&profile=driving-car
```

### Query Parameters

| Param | Type | Required | Default | Description |
|---|---|---|---|---|
| `origin_lat` | float64 | yes | — | Origin latitude |
| `origin_lng` | float64 | yes | — | Origin longitude |
| `dest_lat` | float64 | yes | — | Destination latitude |
| `dest_lng` | float64 | yes | — | Destination longitude |
| `profile` | string | no | `driving-car` | Transport profile: `driving-car`, `foot-walking`, `cycling-regular` |

### Validations (enforced in service layer)

- `origin_lat`, `origin_lng` must parse as float64 — handler returns 400 on parse failure
- `dest_lat`, `dest_lng` must parse as float64 — handler returns 400 on parse failure
- Both origin and destination must be within CABA: `lat ∈ [-35, -34]`, `lng ∈ [-59, -58]` → `ErrInvalidCoordinates`
- Origin coordinates must differ from destination → `ErrSamePoint`
- `profile` must be one of `driving-car`, `foot-walking`, `cycling-regular` → `ErrInvalidProfile`

### Response 200 OK

```json
{
  "origin": {
    "lat": -34.6037,
    "lng": -58.3816
  },
  "destination": {
    "lat": -34.5895,
    "lng": -58.4201
  },
  "profile": "driving-car",
  "distance_meters": 3241.5,
  "duration_seconds": 478.2,
  "geometry": {
    "type": "LineString",
    "coordinates": [
      [-58.3816, -34.6037],
      [-58.3820, -34.6010],
      [-58.4201, -34.5895]
    ]
  }
}
```

> Note: GeoJSON coordinates are `[longitude, latitude]` — same convention as the crimes module.

### Response 400 Bad Request

```json
{
  "error": "invalid_request",
  "message": "origin and destination must be valid CABA coordinates"
}
```

### Response 404 Not Found

```json
{
  "error": "route_not_found",
  "message": "no route found between the given points"
}
```

### Response 502 Bad Gateway

```json
{
  "error": "external_service_error",
  "message": "route service is temporarily unavailable"
}
```

### Response 500 Internal Server Error

```json
{
  "error": "internal_error",
  "message": "could not calculate route"
}
```

---

## Domain Model — `internal/routes/model.go`

```go
package routes

type Route struct {
    Distance float64           `json:"distance_meters"`
    Duration float64           `json:"duration_seconds"`
    Geometry GeoJSONLineString `json:"geometry"`
}

type GeoJSONLineString struct {
    Type        string      `json:"type"`
    Coordinates [][]float64 `json:"coordinates"`
}

type Waypoint struct {
    Lat float64 `json:"lat"`
    Lng float64 `json:"lng"`
}
```

---

## DTOs — `internal/routes/dto.go`

```go
package routes

type RouteQuery struct {
    OriginLat float64
    OriginLng float64
    DestLat   float64
    DestLng   float64
    Profile   string
}

type RouteResponse struct {
    Origin      Waypoint          `json:"origin"`
    Destination Waypoint          `json:"destination"`
    Profile     string            `json:"profile"`
    Distance    float64           `json:"distance_meters"`
    Duration    float64           `json:"duration_seconds"`
    Geometry    GeoJSONLineString `json:"geometry"`
}
```

---

## Errors — `internal/routes/errors.go`

```go
package routes

import "errors"

var (
    ErrInvalidCoordinates = errors.New("invalid coordinates")
    ErrSamePoint          = errors.New("origin and destination are the same point")
    ErrInvalidProfile     = errors.New("invalid transport profile")
    ErrRouteNotFound      = errors.New("no route found")
    ErrExternalService    = errors.New("external routing service error")
)
```

---

## ORS Client — `internal/routes/ors_client.go`

Implements `routingClient` (defined in `service.go`) by calling the ORS Directions API.

**ORS API call:**

```
GET {ORSBaseURL}/v2/directions/{profile}?start={origin_lng},{origin_lat}&end={dest_lng},{dest_lat}
Authorization: Bearer {ORSAPIKey}
Accept: application/json
```

**ORS response shape (relevant fields):**

```json
{
  "type": "FeatureCollection",
  "features": [{
    "type": "Feature",
    "geometry": {
      "type": "LineString",
      "coordinates": [[lng, lat], ...]
    },
    "properties": {
      "summary": {
        "distance": 3241.5,
        "duration": 478.2
      }
    }
  }]
}
```

**Mapping rules:**
- `features[0].properties.summary.distance` → `Route.Distance`
- `features[0].properties.summary.duration` → `Route.Duration`
- `features[0].geometry` → `Route.Geometry`
- ORS HTTP 4xx → `ErrRouteNotFound` (route not routable)
- ORS HTTP 5xx or network error → `ErrExternalService`
- Empty `features` slice → `ErrRouteNotFound`

**Constructor:**

```go
func NewORSClient(apiKey, baseURL string, httpClient *http.Client) *ORSClient
```

Pass `*http.Client` as a dependency to allow injection of a mock transport in tests.

---

## Service — `internal/routes/service.go`

Define the `routingClient` interface here, next to its only consumer:

```go
type routingClient interface {
    GetRoute(ctx context.Context, query RouteQuery) (Route, error)
}

const DefaultProfile = "driving-car"

var allowedProfiles = map[string]bool{
    "driving-car":      true,
    "foot-walking":     true,
    "cycling-regular":  true,
}

func NewService(client routingClient) *Service
```

**`GetRoute(ctx, query) (RouteResponse, error)` must:**

1. If `query.Profile` is empty, set it to `DefaultProfile`
2. Validate profile is in `allowedProfiles` → `ErrInvalidProfile`
3. Validate origin is within CABA bounds → `ErrInvalidCoordinates`
4. Validate destination is within CABA bounds → `ErrInvalidCoordinates`
5. If origin == destination (same lat+lng) → `ErrSamePoint`
6. Call `client.GetRoute(ctx, query)`
7. Map result to `RouteResponse`

---

## Handler — `internal/routes/handler.go`

Accepts a `service` interface and `*slog.Logger`. Implements `app.Registrar`.

```go
func (h *Handler) Register(r chi.Router) {
    r.Get("/routes", h.GetRoute)
}
```

**Error → HTTP mapping:**

| Error | HTTP Status | Error code |
|---|---|---|
| `ErrInvalidCoordinates` | 400 | `invalid_request` |
| `ErrSamePoint` | 400 | `invalid_request` |
| `ErrInvalidProfile` | 400 | `invalid_request` |
| `ErrRouteNotFound` | 404 | `route_not_found` |
| `ErrExternalService` | 502 | `external_service_error` |
| default | 500 | `internal_error` |

---

## Wiring — `internal/app/app.go`

Add after crimes wiring:

```go
orsClient := routes.NewORSClient(cfg.ORSAPIKey, cfg.ORSBaseURL, &http.Client{Timeout: 10 * time.Second})
routesService := routes.NewService(orsClient)
routesHandler := routes.NewHandler(routesService, log)
```

Pass `routesHandler` to `NewRouter(...)`.

---

## Tests

### `service_test.go` — minimum cases

| Case | Expected |
|---|---|
| Origin outside CABA | `ErrInvalidCoordinates` |
| Destination outside CABA | `ErrInvalidCoordinates` |
| Origin == Destination | `ErrSamePoint` |
| Invalid profile | `ErrInvalidProfile` |
| Empty profile | defaults to `driving-car`, calls client |
| Client returns route | returns `RouteResponse` with correct fields |
| Client returns `ErrRouteNotFound` | propagated |
| Client returns `ErrExternalService` | propagated |

Use a mock implementing `routingClient` — same pattern as `crimes/service_test.go`.

### `handler_test.go` — minimum cases

| Case | Expected HTTP |
|---|---|
| Missing `origin_lat` | 400 |
| Unparseable `origin_lng` | 400 |
| Missing `dest_lat` | 400 |
| Invalid profile value | 400 |
| Service returns `ErrInvalidCoordinates` | 400 |
| Service returns `ErrSamePoint` | 400 |
| Service returns `ErrRouteNotFound` | 404 |
| Service returns `ErrExternalService` | 502 |
| Success | 200 with correct JSON body |

Use `httptest.NewRecorder` — same pattern as `crimes/handler_test.go`.

---

## Acceptance Criteria

1. `GET /api/v1/routes?origin_lat=-34.6037&origin_lng=-58.3816&dest_lat=-34.5895&dest_lng=-58.4201` returns 200 with a GeoJSON LineString geometry, distance, and duration.
2. Either coordinate pair outside CABA bounds returns 400.
3. Origin equals destination returns 400.
4. `?profile=invalid` returns 400.
5. When ORS is unreachable or returns 5xx, the endpoint returns 502.
6. `go test ./...` passes with no failures.
7. `go build ./...` produces no errors.

---

## Manual Test Commands

```bash
# Happy path — driving route across CABA
curl "http://localhost:8080/api/v1/routes?origin_lat=-34.6037&origin_lng=-58.3816&dest_lat=-34.5895&dest_lng=-58.4201"

# Walking profile
curl "http://localhost:8080/api/v1/routes?origin_lat=-34.6037&origin_lng=-58.3816&dest_lat=-34.5895&dest_lng=-58.4201&profile=foot-walking"

# Out-of-CABA origin → 400
curl "http://localhost:8080/api/v1/routes?origin_lat=-33.0&origin_lng=-58.3&dest_lat=-34.5895&dest_lng=-58.4201"

# Same origin and destination → 400
curl "http://localhost:8080/api/v1/routes?origin_lat=-34.6037&origin_lng=-58.3816&dest_lat=-34.6037&dest_lng=-58.3816"

# Invalid profile → 400
curl "http://localhost:8080/api/v1/routes?origin_lat=-34.6037&origin_lng=-58.3816&dest_lat=-34.5895&dest_lng=-58.4201&profile=rocket"
```

---

## Agent Prompt

```
Implement the internal/routes module as specified in docs/sdd/wip/003-routes-ors/SPEC.md.

Key points:
- No repository.go file — the routingClient interface is defined inline in service.go, next to its only consumer
- ORSClient (ors_client.go) implements routingClient using stdlib net/http — no new dependencies
- NewORSClient takes (apiKey, baseURL string, httpClient *http.Client) so tests can inject a mock transport
- Add ORSAPIKey and ORSBaseURL to internal/config/config.go and .env.example
- Wire in internal/app/app.go: NewORSClient → NewService → NewHandler, pass routesHandler to NewRouter
- Write service_test.go and handler_test.go following the same mock patterns as the crimes tests
- Run go test ./... and go build ./... before reporting done
```
