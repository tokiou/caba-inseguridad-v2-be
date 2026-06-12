# Add Network Temporal KDE risk scoring + Safe Routes API

## Why

The road graph is imported, cleaned, and routable (`add-graph-walk-routing` proved `pgr_dijkstra`
end-to-end), but `edge_risk_scores` is still empty and the product's core promise — routes that
minimize exposure to reported crime — does not exist yet. The placeholder risk schema from the
foundation milestone (`risk_model_versions` with a `status` column, `edge_risk_scores` keyed only by
`(edge_id, model_version_id)`) cannot represent the model we actually need: risk varies by **time of
day** and **type of day**, and scores must be versioned, reproducible, and evaluable against held-out
future data before being trusted.

This change implements the deterministic core: **Network Temporal KDE**
(`network_temporal_edge_risk_v1`) — crime influence propagated over **walking-network distance**
(not euclidean `ST_DWithin`), weighted by severity, weapon, motorcycle, recency, time bucket, and
weekday type — plus the `GET /api/v1/routes/safe` endpoint that consumes the precomputed scores. No
machine learning in this milestone.

## What changes

- **New capability `risk-scoring`** — schema + offline Python pipeline:
  - New tables: `road_graph_versions`, `crime_network_snaps`, `edge_network_neighborhoods`,
    `edge_risk_score_components`, `risk_model_evaluation_runs`, `edge_risk_backtest_labels`.
  - **Restructured** `risk_model_versions` (adds `type`, `graph_version_id`, `parameters`,
    `train_until`, `is_active` with a partial unique index) and `edge_risk_scores` (adds
    `time_bucket` / `weekday_type` to the key, `raw_score`, `p95_reference`). Both tables are
    empty / placeholder-only today, so they are dropped and recreated (see design.md).
  - New Python package `etl/risk_network_kde/` with CLI commands: `snap-crimes`,
    `build-neighborhoods`, `compute-scores`, `evaluate`, `evaluate-routes`, `calibrate`, `finalize`.
  - Evaluation gate: a model trained on data ≤ 2022-12-31 must rank 2023 crime well
    (PAI@Top5 ≥ 3.0, Recall@Top10 ≥ 0.30, TopDecileLift ≥ 3.0) before the final model
    (trained on all data) is computed and activated.
- **New capability `safe-routes`** — Go API:
  - New table `route_profiles` (seeded: `fastest`, `balanced`, `safest`).
  - New domain `internal/saferoutes/` (handler → service → repository, pgx + pgRouting).
  - `GET /api/v1/routes/safe?origin_lat&origin_lng&dest_lat&dest_lng&datetime=` returns four routes
    — `fastest`, `balanced`, `safest`, `least_safe_candidate` — each with distance, duration,
    `risk_score`, `risk_level`, comparison-vs-fastest metrics, crime metrics, and GeoJSON geometry.
  - Routing cost: `length_meters * (1 + safety_multiplier * risk_score)` via `pgr_dijkstra`;
    `least_safe_candidate` picks the riskiest of K=10 `pgr_ksp` distance candidates within a 1.75
    detour ratio.
- **Modified capability `road-graph`** — the risk-schema requirements move out of `road-graph`
  into `risk-scoring` (they were placeholders pending this milestone).

## In scope

- Deterministic Network Temporal KDE scoring (9 contexts: 4 time buckets × weekday/weekend +
  `all_day`/`all`), fully precomputed offline.
- Temporal backtest (train ≤ 2022, test 2023) with edge-ranking metrics and route-level simulation.
- Parameter calibration grid (run only if the evaluation gate fails).
- Final model on all available data, activated only after the gate passes.
- Go endpoint consuming `edge_risk_scores` + `edge_risk_score_components` — no runtime score
  computation.

## Out of scope (non-goals)

- ML models (LightGBM/XGBoost), user avoid-points, community reports, auth, frontend, Redis cache
  (the cache key shape is documented for the future, not implemented), Valhalla/OSRM.
- Edge segmentation of over-long edges (marked for future work; the quality layer already excludes
  > 5 km edges from routing).
- Product language note: responses expose only metrics (estimated historical exposure), never
  narrative claims of safety.
