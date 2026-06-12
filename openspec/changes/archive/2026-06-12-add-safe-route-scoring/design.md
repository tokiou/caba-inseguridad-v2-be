# Design — Network Temporal KDE risk scoring + Safe Routes API

## 1. Model summary

`network_temporal_edge_risk_v1` (`type = deterministic_network_kde`). For every crime snapped to the
walkable graph and every edge reachable from it within `network_bandwidth_meters` of **walking
distance**, the crime contributes:

```
contribution = quantity
             * severity_weight(crime_type)
             * weapon_multiplier (if weapon_used)
             * motorcycle_multiplier (if motorcycle_used)
             * exp(-snap_distance / snap_distance_decay)
             * exp(-network_distance / network_distance_decay)
             * 0.5 ^ (days_old / recency_half_life_days)
             * temporal_weight(crime_bucket → target_bucket)
             * weekday_weight(crime_weekday_type → target_weekday_type)
```

`raw_score(edge, context) = Σ contributions`; `risk_score = min(raw_score / p95(context), 1)`
(`p95_clamp` over all routable edges in that context; 0 if p95 = 0).

Contexts (9): `{morning, afternoon, evening, night} × {weekday, weekend}` + `all_day × all`
(temporal and weekday weights forced to 1.0).

## 2. Key decisions and adaptations from the incoming spec

### 2.1 Restructure (drop + recreate) the placeholder risk tables

Migrations 000005/000006 created `risk_model_versions` (with `status` ∈ draft/active/archived) and
`edge_risk_scores` keyed `(edge_id, model_version_id)` — no temporal dimension. Both are unusable
for this model. `edge_risk_scores` has never been populated and `risk_model_versions` holds only the
placeholder seed `v1_crime_density_distance_decay`. Migration `000009` therefore **drops and
recreates** them with the new shape instead of ALTERing; the down migration restores the old shape
and seed. `is_active BOOLEAN` + partial unique index replaces the `status` column — the only state
the system reads is "which model is active", and the partial unique index enforces at most one.

### 2.2 Actual table/column names

The incoming spec assumed `road_edges(source, target, name, highway)`. The real schema
(`000004`/`000007`) is `from_node_id`, `to_node_id`, `street_name`, `highway_type`, plus the quality
layer (`is_walkable`, `is_routable`, view `routable_road_edges`). All pipeline SQL and routing SQL
target the real names, and **only routable edges** (`routable_road_edges`) participate in snapping,
neighborhoods, scoring, and routing — non-routable edges never receive scores or traffic.

### 2.3 Severity weights cover the real crime taxonomy

The dataset contains ROBO (553k), HURTO (443k), LESIONES (106k), VIALIDAD (83k), AMENAZAS (81k),
HOMICIDIOS (~950). The incoming spec only listed AMENAZAS/HURTO/ROBO. Seeded weights:

```
HOMICIDIOS 3.0   — rare but maximal severity signal
ROBO       1.5
LESIONES   1.2   — violent personal harm
HURTO      1.0
AMENAZAS   0.7
VIALIDAD   0.3   — traffic offences; weak personal-security signal but kept (non-zero) as
                   a proxy for chaotic corridors
DEFAULT    1.0
```

Weights live in `risk_model_versions.parameters` — changing them is a new model version, never a
code change.

### 2.4 Set-based SQL computation, not per-crime Python loops

1.27M crimes × ~60–120 reachable edges ≈ 10⁸ (crime, target-edge) pairs — iterating in Python is
not viable. The temporal/weekday weights only depend on the crime's bucket vs. the target context,
so scoring factorizes:

1. **In SQL**: join valid snaps × neighborhoods × crimes (date ≤ `train_until`), compute the
   context-independent `base = quantity·severity·weapon·moto·snap_w·net_w·recency`, and aggregate to
   `(target_edge, crime_bucket, crime_weekday_type)` keeping `SUM(base)`, `MAX(base)`, incident
   counts (`SUM(quantity)`) and per-type/flag counts. This collapses 10⁸ pairs to
   ≤ `edges × 4 × 2` rows.
2. **In SQL**: cross the collapsed rows with the 9 contexts, applying `temporal_weight ×
   weekday_weight` as CASE expressions; aggregate to `(edge, time_bucket, weekday_type)`;
   compute the per-context p95 with `percentile_cont`; insert `edge_risk_scores` +
   `edge_risk_score_components`.

The Python CLI orchestrates, validates, reports metrics, and owns the parts SQL is bad at
(per-edge Dijkstra for neighborhoods, evaluation metrics with numpy/pandas).

Component-count semantics: "counts" are **incident counts** (`SUM(quantity)`), not row counts.
`crime_count` and the per-type/flag counts are context-independent (every crime in the neighborhood
influences every context with non-zero weight); the `same/adjacent/other_bucket_crime_count` split is
per-context. For `all_day` every crime counts as `same_bucket`. `weighted_crime_score = raw_score`.

### 2.5 Neighborhoods: per-edge bounded Dijkstra in Python

`edge_network_neighborhoods` is built one source edge at a time: bidirectional seed (both endpoints
at `length/2`), `heapq` Dijkstra over an adjacency list of `routable_road_edges`, cut at
`bandwidth + max_target_half_length`, target distance =
`min(dist[target.from], dist[target.to]) + target.length/2`, keep ≤ bandwidth; the self pair is
stored with distance 0. Streamed to Postgres with `COPY`. ~103k edges × sub-ms searches plus ~10M
row COPY is minutes, no `igraph` dependency needed. Deterministic: fixed iteration order by edge id.

### 2.6 Evaluation design

- **Labels** (`edge_risk_backtest_labels`): 2023 crimes pushed through the *same* snap +
  neighborhood + weighting machinery but with `recency_weight = 1` (no decay — the future is not
  discounted), aggregated per `(edge, context)`.
- **Edge-ranking metrics** per context, length-weighted: PAI@Top5%/Top10% and Recall@Top10% rank
  edges by `risk_score DESC` and take the top X% **of total network length**;
  Precision@Top5% = share of top-5%-length edges with `future_crime_count > 0`; TopDecileLift =
  future-crime density (score per meter) of decile 10 vs. the median decile (deciles by length);
  Spearman over (risk_score, future score).
- **Gate** (averaged over the 8 non-`all_day` contexts, and `all_day` reported separately):
  PAI@Top5 ≥ 3.0, Recall@Top10 ≥ 0.30, TopDecileLift ≥ 3.0, decile-10 future crime > deciles 5 and
  1. Results persist to `risk_model_evaluation_runs` (`passed`, `metrics` JSONB).
- **Route simulation** (`evaluate-routes`): N seeded-random OD pairs (euclidean 500 m–8 km, both
  endpoints snappable), routes per profile via the same cost function, exposure = Σ future label
  score over route edges. Stored as a separate evaluation run. Targets: balanced ≤ +15% distance &
  ≥ 10% exposure reduction; safest ≤ +35% & ≥ 20%.
- **Determinism**: fixed RNG seed for OD sampling; all SQL aggregation is order-independent;
  reruns for the same model version `DELETE + INSERT` their own rows (idempotent).

### 2.7 Calibration

`calibrate` runs the grid from the spec (bandwidth × decay × half-life × weapon × moto), each
candidate saved as `risk_model_versions` row `…_candidate_NNN` + an evaluation run, selected by
`0.45·norm(PAI@5) + 0.35·norm(Recall@10) + 0.20·norm(TopDecileLift)`. Neighborhoods are reused per
bandwidth (the PK includes `bandwidth_meters`). **Only executed if the eval gate fails** — it is
implemented but not part of the happy path.

### 2.8 Go domain: `internal/saferoutes/`, one package

The incoming spec sketched `internal/routing` + `internal/risk` + `internal/roadnetwork`. The
project's layer rules want one domain dir per capability with the standard file split, and the
repo already has `internal/routes` (ORS) and `internal/roadgraph` (graph status + plain walk
routing). A new `internal/saferoutes/` package keeps the capability self-contained:

```
internal/saferoutes/
  model.go               # RoutePlan, SafeRoute, PathEdge, CrimeMetrics, ModelVersion, GeoJSON…
  dto.go                 # SafeRoutesQuery (parsed input), SafeRoutesResponse
  errors.go              # ErrInvalidCoordinates, ErrPointOutsideGraph, ErrNoRoute, ErrNoActiveModel
  repository.go          # Repository interface (routing engine + risk lookups behind one interface)
  postgres_repository.go # pgx + pgRouting implementation
  service.go             # validation, time-bucket resolution, profile orchestration, comparisons
  risk_aggregation.go    # pure functions: weighted avg, route risk, risk level, high-risk meters
  handler.go             # GET /routes/safe
```

The repository interface *is* the routing-engine seam — a future non-pgRouting engine implements the
same interface. Profiles are read from `route_profiles` per request (3 rows; trivially cacheable
later).

### 2.9 Routing queries

- Snap origin/destination to the nearest **routable edge**, entering at the nearer endpoint (same
  trick as `add-graph-walk-routing` — avoids orphan nodes), but additionally enforce
  `snap distance ≤ 150 m` → else `400 origin_or_destination_outside_walkable_graph`.
- `fastest`: `pgr_dijkstra`, cost = `length_meters`.
- `balanced`/`safest`: `pgr_dijkstra`, cost =
  `length_meters * (1 + safety_multiplier * COALESCE(risk_score, 0))`, LEFT JOIN of
  `edge_risk_scores` filtered by active model + context. The multiplier/model/context values are
  bound as query parameters — never string-concatenated.
- `least_safe_candidate`: `pgr_ksp` (available, pgRouting 3.6.1) K=10 by plain distance, drop
  candidates with `distance > fastest_distance × 1.75`, pick the highest `route_risk_score`.
  Returns `null` if no candidate beats `fastest`'s own risk… no — it always returns the riskiest
  candidate (which may be `fastest` itself); `null` only if KSP yields nothing.
- Each path query returns one row per traversed edge (seq, edge id, length, walk duration, risk
  score, component counts) plus a merged `ST_AsGeoJSON(ST_LineMerge(ST_Collect(...)))` geometry;
  aggregation to route metrics happens in Go (`risk_aggregation.go`, unit-testable).

Route-level crime metrics are **sums of per-edge component metrics** — a crime influencing several
consecutive edges is counted in each; documented as "estimated exposure along the route", not a
distinct-incident count.

### 2.10 Time semantics

`datetime` (RFC3339, optional — defaults to server time in `America/Argentina/Buenos_Aires`) resolves
in its own offset: hour 6–11 `morning`, 12–17 `afternoon`, 18–21 `evening`, else `night`; Sat/Sun
`weekend`. The Go mapping mirrors `etl/risk_network_kde/utils.py` exactly (unit tests on both
sides pin the boundaries: 5→night, 6→morning, 11→morning, 12→afternoon, 17→afternoon, 18→evening,
21→evening, 22→night).

### 2.11 Duration

`duration_minutes = Σ walk_duration_seconds / 60` (graph already carries 1.4 m/s durations);
rounded to one decimal in responses.

### 2.12 Future cache key (documented only)

`route:v1:{model_version_id}:{time_bucket}:{weekday_type}:{origin_cell}:{dest_cell}` with cells =
coordinates rounded to 4 decimals. Not implemented (no Redis in this milestone).

## 3. Pipeline execution order

```
psql -f migrations/000008…000010 (manual, in order — no migrate tool in this repo)
python -m etl.risk_network_kde.cli snap-crimes        --graph-version caba_walking_graph_v1 --max-distance-meters 80
python -m etl.risk_network_kde.cli build-neighborhoods --graph-version caba_walking_graph_v1 --bandwidth-meters 350
python -m etl.risk_network_kde.cli compute-scores      --model network_temporal_edge_risk_v1_eval_2022 --train-until 2022-12-31
python -m etl.risk_network_kde.cli evaluate            --model network_temporal_edge_risk_v1_eval_2022 --test-from 2023-01-01 --test-to 2023-12-31
python -m etl.risk_network_kde.cli evaluate-routes     --model network_temporal_edge_risk_v1_eval_2022 --test-from 2023-01-01 --test-to 2023-12-31
# only if the gate fails: calibrate
python -m etl.risk_network_kde.cli finalize            --base-model network_temporal_edge_risk_v1_eval_2022 --final-model network_temporal_edge_risk_v1 --train-until latest --activate
```

Python deps (`etl/risk_network_kde/requirements.txt`): `psycopg2-binary` (matches the existing ETL),
`python-dotenv`, `numpy`, `pandas`, `tqdm`. No shapely (geometry stays in PostGIS), no igraph.

## 4. Risks / trade-offs

- `edge_network_neighborhoods` at bandwidth 350 is ~10M rows (~1 GB with indexes) — acceptable;
  calibration bandwidth 500 roughly doubles it; rows are keyed by bandwidth so variants coexist.
- p95-clamp normalization saturates the top 5% of edges at 1.0 by construction — intended (we care
  about ranking and relative cost, not absolute magnitude).
- Snapping each crime to its single nearest edge ignores corner ambiguity (a crime at an
  intersection influences via one edge); network propagation immediately spreads it to the
  adjacent edges, so the error is bounded by `snap_distance_decay`.
- The `least_safe_candidate` KSP query runs K Dijkstras — the endpoint's worst-case latency driver.
  Acceptable for this milestone; cacheable later.

## 5. Evaluation audit — Spearman ~0.96 and leakage review (post-implementation)

The very high Spearman raised a leakage concern; audited 2026-06-12, **no leakage found**:

1. Eval-model scores filter `c.date <= train_until` (`2022-12-31`) — `compute_scores.build_params`
   passes `("1900-01-01", until)`; verified in `stage1_sql`.
2. Backtest labels filter `c.date BETWEEN test_from AND test_to` (2023 window only), recency
   forced to 1.0 (`with_recency=False`).
3. The evaluation joins `edge_risk_scores` by the **eval model's id** (resolved from the CLI name,
   never `is_active`); run 2 also executed before the final model existed.
4. `edge_risk_backtest_labels` is never read by `compute-scores` (docstring mention only).
5. All `risk_model_evaluation_runs` rows bind to `..._eval_2022`; the final model has its own
   score rows and no evaluation runs.

Why Spearman is legitimately that high: labels are built with the **same network-smoothing
machinery** as the scores (same snaps, neighborhoods, severity/temporal weights, per spec §9 of the
incoming spec), so both sides are heavily spatially smoothed crime-intensity fields; CABA crime is
strongly spatially persistent year over year; and the rank correlation runs over ~100k edges whose
ordering is dominated by the stable center–periphery gradient. Spearman is therefore reported as a
descriptive statistic only — the discriminative gate metrics are PAI@Top5, Recall@Top10, and
TopDecileLift (length-weighted).

Route testing also surfaced a graph defect (Plaza de Mayo → Constitución 404 via a 2-node island);
fixed in the follow-up change `add-disconnected-component-cleanup` (cleanup rule 5 +
pipeline re-run over the single-component graph).
