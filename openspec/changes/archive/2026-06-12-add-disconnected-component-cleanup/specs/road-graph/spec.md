# Road Graph — Delta Specification

## MODIFIED Requirements

### Requirement: Edge routability quality layer

`road_edges` SHALL carry a non-destructive quality layer: a boolean `is_routable` (default `true`),
a nullable `excluded_reason`, and a nullable `quality_checked_at`. A `routable_road_edges` view SHALL
expose exactly the edges that are both `is_walkable` and `is_routable`. Quality classification SHALL
mark edges (never delete them), so the layer is auditable and reversible. `is_walkable` denotes
import provenance; `is_routable` denotes fitness for routing. After the per-edge rules, edges
outside the **largest connected component** of the surviving routable graph SHALL be marked
non-routable (`disconnected_component`), so nearest-edge snapping can never strand a route request
on an isolated island.

#### Scenario: Quality columns and view exist

- WHEN the migration is applied
- THEN `road_edges` has `is_routable`, `excluded_reason`, and `quality_checked_at`
- AND a `routable_road_edges` view returns rows where `is_walkable = true AND is_routable = true`
- AND there are indexes on `is_routable`, `excluded_reason`, and `(is_walkable, is_routable)`

#### Scenario: Cleanup marks anomalous edges non-routable without deleting

- GIVEN an imported graph containing zero/negative-length, self-loop, invalid-geometry,
  over-5000 m, or disconnected-island edges
- WHEN the cleanup script runs
- THEN those edges have `is_routable = false` with `excluded_reason` set to
  `zero_or_negative_length`, `self_loop`, `invalid_geometry`, `suspicious_long_edge_over_5000m`,
  or `disconnected_component` respectively
- AND no `road_edges` row is deleted (the total edge count is unchanged)

#### Scenario: Cleanup is idempotent

- GIVEN the cleanup script has already run
- WHEN it runs again
- THEN the set of routable and excluded edges is identical to the previous run (it resets quality
  state before re-applying the rules)

#### Scenario: Routable graph is a single connected component

- GIVEN the cleanup script has run
- WHEN `pgr_connectedComponents` is computed over `routable_road_edges`
- THEN exactly one component remains
- AND any two routable edges are mutually reachable
