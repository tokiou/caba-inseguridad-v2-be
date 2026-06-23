# Safe Routes Specification

## Purpose

Expose `GET /api/v1/routes/safe`: walking routes over the local CABA graph that trade distance
against **estimated historical crime exposure** using the precomputed `edge_risk_scores` of the
active risk model. Returns `fastest`, `balanced`, `safest`, and `least_safe_candidate` with
metric-only metadata (distance, duration, risk score/level, vs-fastest comparisons, crime metrics,
GeoJSON geometry). Lives in `internal/saferoutes/` (pgx + pgRouting); never computes risk at
runtime.

## Requirements

### Requirement: Route profiles table

The system SHALL persist routing profiles in `route_profiles` (`name`, `safety_multiplier`,
`max_detour_ratio`, `description`), seeded idempotently: `fastest` (0.0 / 1.0), `balanced`
(1.5 / 1.35), `safest` (3.0 / 1.75). Profile tuning is data, not code.

#### Scenario: Seed is upserted

- WHEN the migration runs twice
- THEN exactly three profile rows exist with the seeded multipliers

### Requirement: Safe routes endpoint

The API SHALL expose `GET /api/v1/routes/safe` accepting `origin_lat`, `origin_lng`, `dest_lat`,
`dest_lng` (required, WGS84, within CABA, distinct) and `datetime` (optional RFC3339; defaults to
the current time in America/Argentina/Buenos_Aires). It SHALL resolve the request's `time_bucket`
(morning 6–11 / afternoon 12–17 / evening 18–21 / night otherwise) and `weekday_type`
(weekend = Sat/Sun) using the same boundaries as the scoring pipeline, look up the active risk
model, and return four routes — `fastest`, `balanced`, `safest`, `least_safe_candidate` — plus the
echoed origin/destination, resolved context, and active model metadata (`id`, `name`, `type`,
`train_until`). The response SHALL contain metrics only — no narrative safety claims.

#### Scenario: Night-time request returns four routes

- GIVEN an active risk model with populated scores
- WHEN a client requests a route with `datetime=2026-06-12T23:00:00-03:00`
- THEN the response is HTTP 200 with `time_bucket = "night"`, `weekday_type` matching the date,
  and a `routes` array containing kinds `fastest`, `balanced`, `safest`, `least_safe_candidate`

#### Scenario: Invalid input

- WHEN coordinates are missing, unparseable, outside CABA, or origin equals destination, or
  `datetime` is not RFC3339
- THEN the response is HTTP 400 `invalid_request` and no routing query runs

#### Scenario: Point outside the walkable graph

- GIVEN an origin whose nearest routable edge is farther than 150 m
- WHEN the route is requested
- THEN the response is HTTP 400 with error code `origin_or_destination_outside_walkable_graph`

#### Scenario: No active model

- GIVEN no `risk_model_versions` row is active
- WHEN the route is requested
- THEN the response is HTTP 503 with error code `risk_model_unavailable`

#### Scenario: Disconnected endpoints

- GIVEN snapped endpoints with no connecting path on the routable graph
- WHEN the route is requested
- THEN the response is HTTP 404 `route_not_found`

### Requirement: Risk-weighted routing cost

`fastest` SHALL minimize plain `length_meters`. `balanced` and `safest` SHALL minimize
`length_meters × (1 + safety_multiplier × COALESCE(risk_score, 0))` via `pgr_dijkstra` over
`routable_road_edges`, joining `edge_risk_scores` for the active model and resolved context. Edges
without a score cost their plain length. Multiplier, model id, and context SHALL be bound as query
parameters — never concatenated from user input. Origin/destination snap to the nearest routable
edge's nearer endpoint (≤ 150 m).

#### Scenario: Safest avoids high-risk corridors

- GIVEN a high-risk corridor on the shortest path and a moderately longer low-risk alternative
- WHEN `safest` is computed
- THEN it takes the alternative while `fastest` keeps the corridor

### Requirement: Least-safe candidate

`least_safe_candidate` SHALL be selected — never optimized for — by generating K = 10 distance-ranked
candidate paths with `pgr_ksp`, discarding those whose distance exceeds `fastest × 1.75`, and
returning the remaining candidate with the highest `route_risk_score`. If KSP yields no candidates,
the field SHALL be omitted from `routes` (degraded, documented behavior).

#### Scenario: Bounded detour

- WHEN `least_safe_candidate` is returned
- THEN its `distance_meters ≤ fastest.distance_meters × 1.75`

### Requirement: Route risk aggregation

Per route the service SHALL compute `weighted_avg_edge_risk = Σ(length×risk)/Σ(length)`,
`route_risk_score = 0.75 × weighted_avg_edge_risk + 0.25 × max_edge_risk`, and map it to
`risk_level` (`low` ≤ 0.33 < `moderate` ≤ 0.66 < `high`) using the active model's thresholds. It
SHALL also report `high_risk_edge_meters` / `high_risk_edge_percent` (length with edge risk ≥ the
high threshold), `max_edge_risk`, and `avg_edge_risk`.

#### Scenario: Single dangerous stretch is not averaged away

- GIVEN a route of 20 low-risk edges and one edge with risk 1.0
- WHEN aggregated
- THEN `route_risk_score` exceeds `weighted_avg_edge_risk` by the 0.25 × max component

### Requirement: Route metadata is metric-only

Each returned route SHALL include: `kind`, `distance_meters`, `duration_minutes` (from the graph's
walk durations), `risk_score`, `risk_level`, `extra_distance_vs_fastest_meters`,
`extra_duration_vs_fastest_minutes`, `risk_reduction_vs_fastest_percent`, `high_risk_edge_meters`,
`high_risk_edge_percent`, `max_edge_risk`, `avg_edge_risk`, a `crime_metrics` object
(`crime_count`, `robbery_count`, `theft_count`, `threats_count`, `armed_count`,
`motorcycle_count`, `same_bucket_crime_count` — sums of per-edge component metrics for the resolved
context), and a GeoJSON `LineString` geometry in `[lng, lat]` order.

Each route SHALL also include the following explainability metadata, derived from the per-edge risk
and crime components already read for the resolved context — **metrics and categorical values only,
no free-text explanations**:

- `riskiest_segment` — the single edge with the highest `risk_score` on the route: its `risk_score`,
  `risk_level`, `length_meters`, a representative `point` (`{lat, lng}`), and its crime counts
  (`crime_count`, `robbery_count`, `armed_count`, `theft_count`, `threats_count`,
  `motorcycle_count`). Omitted only when the route has no edges.
- `segments` — an ordered, per-edge list; each entry carries `risk_score`, `robbery_count`,
  `length_meters`, and a representative `point` (`{lat, lng}`). This is the minimal block-by-block
  view that lets a client see where one route diverges in risk from a near-parallel alternative.
- `dominant_factor` — the categorical crime type with the largest count on the route, one of
  `"robbery" | "theft" | "threats" | "none"` (`none` when there are no counts).
- `armed_share_percent` — `armed_count / crime_count × 100` for the route (0 when `crime_count` is 0).
- `time_of_day_risk` — the SAME route's risk recomputed for each of the four time buckets
  (`morning`, `afternoon`, `evening`, `night`) for the resolved `weekday_type`, each as
  `{risk_score, risk_level}`, plus `peak_bucket` naming the worst bucket. Bucket granularity only
  (no hourly resolution). Omitted only when the route has no edges.

The crime counts surfaced here are sums of per-edge estimated exposure, not distinct incidents;
`dominant_factor` and `armed_share_percent` are therefore relative comparisons (each underlying
count is inflated the same way) and SHALL be presented as relative exposure, never as absolute
incident totals. No new risk computation or model parameter is introduced: the values are read and
reshaped from `edge_risk_scores` / `edge_risk_score_components` for the active model.

#### Scenario: Crime metrics come from precomputed components

- WHEN a route is returned
- THEN its `crime_metrics` are sums over the route's edges of `edge_risk_score_components` for the
  active model and resolved context, with no runtime crime-table query

#### Scenario: Riskiest segment identifies the edge driving the score

- GIVEN a route of mostly low-risk edges and one edge with a clearly higher `risk_score`
- WHEN the route is returned
- THEN `riskiest_segment.risk_score` equals that edge's risk and `riskiest_segment.point` lies on
  that edge
- AND it equals the route's `max_edge_risk`

#### Scenario: Segments cover the whole route in order

- WHEN a route with N edges is returned
- THEN `segments` has N entries in path order, each with `risk_score`, `robbery_count`,
  `length_meters`, and a `point`
- AND the sum of `segments[].length_meters` equals the route's `distance_meters`

#### Scenario: Dominant factor and armed share reflect the route's counts

- GIVEN a route whose summed `robbery_count` exceeds its `theft_count` and `threats_count`
- WHEN the route is returned
- THEN `dominant_factor = "robbery"`
- AND `armed_share_percent` equals `armed_count / crime_count × 100`

#### Scenario: Time-of-day risk exposes the worst bucket

- GIVEN a route whose edges carry higher risk in the `night` context than in other buckets for the
  resolved `weekday_type`
- WHEN the route is returned
- THEN `time_of_day_risk` contains a `{risk_score, risk_level}` entry for each of `morning`,
  `afternoon`, `evening`, `night`
- AND `time_of_day_risk.peak_bucket = "night"`

#### Scenario: Metadata stays metric-only

- WHEN any route is returned
- THEN the explainability fields contain only numbers, coordinates, and the fixed `dominant_factor`
  / `peak_bucket` enums — no free-text narrative

### Requirement: Safe routes requires authentication

`GET /api/v1/routes/safe` SHALL require a valid access token. The endpoint SHALL be mounted behind the
auth middleware so that requests without a valid `Authorization: Bearer <access_token>` are rejected
before any routing or risk work is done. The other open-data endpoints (`/crimes/nearby`, `/routes`,
`/roadgraph/*`) remain public.

#### Scenario: Unauthenticated safe-route request is rejected

- WHEN a client calls `GET /api/v1/routes/safe` with no or an invalid/expired access token
- THEN the response is `401` with error code `unauthorized`
- AND no route or risk computation is performed

#### Scenario: Authenticated safe-route request succeeds

- GIVEN a valid access token for an active user
- WHEN the client calls `GET /api/v1/routes/safe` with `Authorization: Bearer <token>` and valid
  origin/destination parameters
- THEN the request is processed and returns the safe-route response as specified

### Requirement: Layered architecture

The capability SHALL live in `internal/saferoutes/` following
`handler → service → repository interface → PostgresRepository (pgx + pgRouting)`. The repository
interface is the routing-engine seam — a future engine (non-pgRouting) implements the same
interface without touching handler or service. Risk scores are read, never computed, at request
time.

#### Scenario: Layer boundaries respected

- WHEN the capability is implemented
- THEN HTTP parsing exists only in the handler, context/profile orchestration in the service, and
  all SQL behind the repository interface
