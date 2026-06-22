# Safe Routes — Delta Specification

## MODIFIED Requirements

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
