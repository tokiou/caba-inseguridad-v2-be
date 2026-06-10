# Routing Specification

## Purpose

Compute a route between two points within CABA by delegating to the OpenRouteService (ORS) external
API. This is the foundational routing layer; crime-weighted risk scoring and safe-path optimization
are out of scope for this capability.

## Requirements

### Requirement: Route endpoint

The API SHALL expose `GET /api/v1/routes` accepting `origin_lat`, `origin_lng`, `dest_lat`,
`dest_lng`, and an optional `profile`, returning a route geometry with distance and duration.

#### Scenario: Successful route

- GIVEN valid CABA origin and destination coordinates
- WHEN a client sends `GET /api/v1/routes?origin_lat=-34.6037&origin_lng=-58.3816&dest_lat=-34.5895&dest_lng=-58.4201`
- THEN the response is HTTP 200
- AND the body contains `origin`, `destination`, `profile`, `distance_meters`, `duration_seconds`,
  and a GeoJSON `LineString` geometry with `[longitude, latitude]` coordinates

#### Scenario: Default transport profile

- GIVEN a request with no `profile`
- WHEN the endpoint is queried
- THEN the profile defaults to `driving-car`

### Requirement: Input validation

The service SHALL validate coordinates, profile, and distinctness of endpoints, returning HTTP 400
for invalid input.

#### Scenario: Endpoint out of CABA bounds

- GIVEN an origin or destination outside `lat ∈ [-35, -34]` / `lng ∈ [-59, -58]`
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

#### Scenario: Origin equals destination

- GIVEN identical origin and destination coordinates
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

#### Scenario: Invalid profile

- GIVEN a `profile` not in `driving-car`, `foot-walking`, `cycling-regular`
- WHEN the endpoint is queried
- THEN the response is HTTP 400 with error code `invalid_request`

### Requirement: ORS upstream error mapping

The ORS client SHALL map upstream responses so that only a genuine no-route is a 404, and
auth/rate-limit/bad-request/server failures are surfaced as 502.

#### Scenario: No route between points

- GIVEN ORS responds 404 or returns an empty feature set
- WHEN the route is requested
- THEN the response is HTTP 404 with error code `route_not_found`

#### Scenario: Upstream auth or availability failure

- GIVEN ORS responds with 401/403/429/400 or 5xx, or is unreachable
- WHEN the route is requested
- THEN the response is HTTP 502 with error code `external_service_error`
- AND the upstream status and a bounded body snippet are recorded in logs for diagnosis

### Requirement: Routing configuration

The service SHALL read `ORS_API_KEY` (required) and `ORS_BASE_URL` (default
`https://api.openrouteservice.org`) from configuration.

#### Scenario: Configuration loaded

- WHEN the app starts
- THEN the ORS client is constructed from `ORS_API_KEY` and `ORS_BASE_URL`

### Requirement: Layered architecture for routing

The flow SHALL be `handler → service → routingClient → ORS`. The `routingClient` interface is
defined next to its consumer in the service; the handler holds no upstream-call logic and the client
holds no HTTP-handler logic.

#### Scenario: Layer boundaries respected

- WHEN the routing endpoint is implemented or modified
- THEN HTTP parsing lives only in the handler, validation/defaults in the service, and all ORS
  communication behind the `routingClient` interface
