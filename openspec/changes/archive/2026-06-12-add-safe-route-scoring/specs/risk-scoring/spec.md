# Risk Scoring — Delta Specification

## ADDED Requirements

### Requirement: Road graph versioning

The system SHALL persist named road-graph versions in `road_graph_versions` with a unique `name`,
an `is_active` boolean enforced to at most one active row by a partial unique index, and a creation
timestamp. The current cleaned CABA walkable graph SHALL be seeded as `caba_walking_graph_v1`
(active). All risk artifacts (snaps, neighborhoods) reference a graph version.

#### Scenario: Single active graph version

- GIVEN `caba_walking_graph_v1` is active
- WHEN a second row is inserted with `is_active = true`
- THEN the insert is rejected by the partial unique index

### Requirement: Risk model versioning

The system SHALL persist versioned risk models in `risk_model_versions` with a unique `name`, a
`type` (e.g. `deterministic_network_kde`), a `graph_version_id` reference, a JSONB `parameters`
object holding every tunable weight (snap decay, bandwidth, network decay, recency half-life,
severity weights covering the dataset's crime types, weapon/motorcycle multipliers,
temporal/weekday weights, normalization method, risk-level thresholds), a `train_until` date, and an
`is_active` boolean with a partial unique index allowing at most one active model. Changing model
parameters SHALL be a new version, never a mutation of an existing one. This replaces the
placeholder `status`-based table from the road-graph foundation milestone (which held no real data).

#### Scenario: Evaluation model seeded inactive

- WHEN the migrations are applied
- THEN `network_temporal_edge_risk_v1_eval_2022` exists with `type = 'deterministic_network_kde'`,
  `train_until = '2022-12-31'`, `is_active = false`
- AND its `parameters` include `network_bandwidth_meters`, `network_distance_decay_meters`,
  `crime_snap_max_distance_meters`, `snap_distance_decay_meters`, `recency_half_life_days`,
  `severity_base_weights`, `weapon_multiplier`, `motorcycle_multiplier`, `temporal_weights`,
  `weekday_weights`, `normalization`, and `risk_levels`

#### Scenario: At most one active model

- GIVEN an active model exists
- WHEN another model is set `is_active = true` without deactivating the first in the same
  transaction
- THEN the partial unique index rejects it

### Requirement: Crime-to-network snapping

The offline pipeline SHALL snap each scoreable crime (valid `geom`, valid `date`, `hour` in 0–23,
`quantity > 0`) to its nearest **routable** edge (`routable_road_edges`) by geography distance,
persisting per `(crime_id, graph_version_id)`: the edge, the snap distance in meters, the position
along the edge (`snap_fraction`), and the snapped point. Crimes farther than
`crime_snap_max_distance_meters` (80) from any routable edge SHALL be persisted with
`is_valid_for_network_scoring = false` and `rejection_reason = 'no_edge_within_snap_distance'` and
excluded from scoring. Snapping SHALL be idempotent (re-runs upsert).

#### Scenario: Crime near a street snaps to it

- GIVEN a crime 12 m from the nearest routable edge
- WHEN `snap-crimes` runs
- THEN a `crime_network_snaps` row exists with that edge, `snap_distance_meters ≈ 12`, and
  `is_valid_for_network_scoring = true`

#### Scenario: Isolated crime is rejected, not dropped

- GIVEN a crime more than 80 m from every routable edge
- WHEN `snap-crimes` runs
- THEN its snap row has `is_valid_for_network_scoring = false` and
  `rejection_reason = 'no_edge_within_snap_distance'`

### Requirement: Network neighborhoods

The pipeline SHALL precompute, per graph version and bandwidth, which edges are reachable from each
source edge over **walking-network distance** (`edge_network_neighborhoods`), defined as
`min(dist_to(target.from_node), dist_to(target.to_node)) + target.length/2`, where Dijkstra is
seeded from both endpoints of the source edge at `source.length/2`. Only pairs with
`network_distance_meters <= bandwidth_meters` are stored; the self pair is stored at distance 0.
Crime influence MUST propagate through this table — euclidean `ST_DWithin` MUST NOT be the primary
propagation model.

#### Scenario: Influence respects street connectivity

- GIVEN two edges 90 m apart in a straight line but separated by a city block with no connecting
  street within the bandwidth
- WHEN neighborhoods are built
- THEN no `edge_network_neighborhoods` pair connects them

#### Scenario: Self pair

- WHEN neighborhoods are built for edge E
- THEN the pair (E, E) exists with `network_distance_meters = 0`

### Requirement: Deterministic Network Temporal KDE scoring

`compute-scores` SHALL populate `edge_risk_scores` and `edge_risk_score_components` for a named
model version using **only crimes dated ≤ the model's `train_until`**. Each valid snapped crime
contributes to every reachable edge:
`quantity × severity × weapon × motorcycle × exp(-snap_dist/snap_decay) ×
exp(-net_dist/net_decay) × 0.5^(days_old/half_life) × temporal_weight × weekday_weight`, where
`days_old = train_until - crime.date`. Scores SHALL be computed for 9 contexts: 4 time buckets
(`morning` 6–11, `afternoon` 12–17, `evening` 18–21, `night` otherwise) × {`weekday`, `weekend`}
plus `all_day`/`all` with temporal and weekday weights = 1.0. Per context,
`risk_score = min(raw_score / p95(raw_score), 1)` (0 when p95 = 0), with the p95 stored as
`p95_reference`. Components SHALL record incident counts (sum of `quantity`) total and per
type/flag, the same/adjacent/other-bucket split, and the maximum single-crime contribution.
Recomputation for the same model SHALL replace its rows (idempotent). The computation MUST be
deterministic: same inputs and parameters produce identical scores.

#### Scenario: Full coverage of scored contexts

- GIVEN N routable edges with at least one contributing crime context
- WHEN `compute-scores` completes
- THEN `edge_risk_scores` holds rows only for the 9 valid `(time_bucket, weekday_type)` contexts
- AND every `risk_score` is within [0, 1]

#### Scenario: Temporal hold-out respected

- GIVEN model `…_eval_2022` with `train_until = 2022-12-31`
- WHEN scores are computed
- THEN no crime dated 2023-01-01 or later contributes to any score

#### Scenario: Time-of-day differentiation

- GIVEN an edge whose neighborhood crimes occurred mostly at night
- WHEN scores are computed
- THEN its `night` context raw score exceeds its `morning` context raw score

### Requirement: Temporal backtest evaluation

`evaluate` SHALL build `edge_risk_backtest_labels` from crimes in a held-out test window (snapped
and propagated identically but **without recency decay**), then compute per context: PAI@Top5%,
PAI@Top10%, Recall@Top10%, Precision@Top5%, TopDecileLift, and Spearman correlation — where top-X%
is by **total network length**, not edge count. Results SHALL persist to
`risk_model_evaluation_runs` with `passed` set by the gate: PAI@Top5 ≥ 3.0, Recall@Top10 ≥ 0.30,
TopDecileLift ≥ 3.0, and decile 10 holding more future crime than deciles 5 and 1. Test-window
crimes MUST NOT influence the evaluated scores. A model that fails the gate MUST NOT be finalized
or activated.

#### Scenario: Evaluation run recorded

- WHEN `evaluate --model …_eval_2022 --test-from 2023-01-01 --test-to 2023-12-31` completes
- THEN a `risk_model_evaluation_runs` row exists with the window, the model's parameters snapshot,
  per-context metrics JSON, and a boolean `passed`

#### Scenario: Failing model stays inactive

- GIVEN an evaluation run with `passed = false`
- WHEN `finalize` is invoked for that base model
- THEN it aborts without creating or activating a final model

### Requirement: Route-level simulation

`evaluate-routes` SHALL generate deterministic (seeded) origin–destination pairs within CABA
(euclidean 500 m–8 km, both endpoints snappable) and compare `fastest`, `balanced`, and `safest`
routes computed with the model's scores against future-window exposure, reporting average extra
distance and exposure reduction vs `fastest`, persisted as an evaluation run.

#### Scenario: Trade-off metrics reported

- WHEN `evaluate-routes` completes
- THEN the run's metrics include, per profile, mean `extra_distance_vs_fastest_percent` and mean
  `exposure_reduction_vs_fastest_percent`

### Requirement: Calibration produces versioned candidates

When the gate fails, `calibrate` SHALL sweep a bounded parameter grid (bandwidth, network decay,
recency half-life, weapon and motorcycle multipliers), persisting each candidate as its own
`risk_model_versions` row (`…_candidate_NNN`) with its own scores and evaluation run, and SHALL
select by `0.45·norm(PAI@Top5) + 0.35·norm(Recall@Top10) + 0.20·norm(TopDecileLift)`. The grid is
finite; no unbounded search.

#### Scenario: Candidates are traceable

- WHEN calibration evaluates a parameter combination
- THEN a candidate model version with those exact parameters and a linked evaluation run exist

### Requirement: Finalization and activation

`finalize` SHALL create the production model (`network_temporal_edge_risk_v1`) by copying the
approved parameters, setting `train_until` to the maximum crime date available, recomputing scores
and components over **all** data, and activating it atomically (deactivate all, activate one, in a
single transaction). Only a model whose base passed evaluation may be finalized.

#### Scenario: Final model active and fresh

- WHEN `finalize --activate` completes
- THEN `network_temporal_edge_risk_v1` is the only active model
- AND its `train_until` equals `MAX(crimes.date)`
- AND its scores cover all 9 contexts

### Requirement: Offline-only computation

All scoring, snapping, neighborhood building, evaluation, and calibration SHALL run offline in the
Python pipeline (`etl/risk_network_kde/`). The Go API MUST NOT compute or mutate risk scores at
runtime; it only reads `edge_risk_scores`, `edge_risk_score_components`, `risk_model_versions`, and
`route_profiles`. The score tables are the stable contract: future model families (ML, community,
hybrid) replace the producer, never the consumer.

#### Scenario: API is read-only over scores

- WHEN any `/api/v1/routes/safe` request is served
- THEN no row in any risk table is inserted, updated, or deleted
