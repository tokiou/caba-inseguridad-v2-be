# Tasks — Network Temporal KDE risk scoring + Safe Routes API

## 1. Migrations (apply with psql, in order)
- [x] 1.1 `000008_create_road_graph_versions`: table + partial unique active index + seed
      `caba_walking_graph_v1` (active).
- [x] 1.2 `000009_restructure_risk_model_tables`: drop placeholder `edge_risk_scores` /
      `risk_model_versions`; recreate per new schema (`type`, `graph_version_id`, `parameters`,
      `train_until`, `is_active`; scores keyed by `(edge_id, model_version_id, time_bucket,
      weekday_type)` with `raw_score` / `p95_reference`); create `crime_network_snaps`,
      `edge_network_neighborhoods`, `edge_risk_score_components`, `risk_model_evaluation_runs`,
      `edge_risk_backtest_labels`; seed `network_temporal_edge_risk_v1_eval_2022` (inactive,
      `train_until = 2022-12-31`). Down migration restores the 000005/000006 shape + seed.
- [x] 1.3 `000010_create_route_profiles`: table + upsert seed (`fastest` 0.0/1.0, `balanced`
      1.5/1.35, `safest` 3.0/1.75).
- [x] 1.4 Apply all three to the dev DB; verify graph pre-checks (edge orphans = 0, length
      percentiles) per spec.

## 2. Python pipeline (`etl/risk_network_kde/`)
- [x] 2.1 Package skeleton: `config.py` (DEFAULT_MODEL_PARAMETERS), `db.py` (DATABASE_URL via
      dotenv), `utils.py` (`hour_to_time_bucket`, `date_to_weekday_type`, bucket adjacency,
      context list), `cli.py` (argparse subcommands).
- [x] 2.2 `snap_crimes.py`: set-based nearest-routable-edge snap (LATERAL KNN ≤ 80 m geography),
      upsert `crime_network_snaps`, reject rows beyond max distance
      (`no_edge_within_snap_distance`), report metrics (totals, p50/p95/max snap distance).
- [x] 2.3 `graph_loader.py` + `build_neighborhoods.py`: adjacency list from `routable_road_edges`,
      per-edge bounded Dijkstra (endpoints seeded at length/2), self-pair at distance 0, COPY into
      `edge_network_neighborhoods`, report target-count percentiles + duration.
- [x] 2.4 `compute_scores.py`: two-stage set-based SQL (collapse to (edge, crime_bucket,
      crime_weekday) → expand to 9 contexts), p95-clamp normalization, populate
      `edge_risk_scores` + `edge_risk_score_components` (idempotent per model).
- [x] 2.5 `evaluate.py`: build `edge_risk_backtest_labels` from the test window (no recency decay);
      compute per-context PAI@Top5/Top10, Recall@Top10, Precision@Top5, TopDecileLift, Spearman;
      persist run + pass/fail against the gate.
- [x] 2.6 `evaluate_routes.py`: seeded OD pairs, profile routes via pgr_dijkstra cost function,
      future-exposure comparison vs fastest; persist as route-simulation run.
- [x] 2.7 `calibrate.py`: parameter grid → candidate model versions + evaluation runs + weighted
      selection score (implemented; run only if 2.5 gate fails).
- [x] 2.8 `finalize.py`: copy approved params, `train_until = max(crimes.date)`, recompute scores +
      components, activate atomically (deactivate-all + activate in one tx).
- [x] 2.9 `requirements.txt` for the package.

## 3. Run the pipeline
- [x] 3.1 `snap-crimes` (graph v1, 80 m) — record metrics.
- [x] 3.2 `build-neighborhoods` (bandwidth 350) — record metrics.
- [x] 3.3 `compute-scores` for `network_temporal_edge_risk_v1_eval_2022` (train ≤ 2022-12-31);
      verify `edge_risk_scores ≈ routable_edges × 9` and scores ∈ [0,1].
- [x] 3.4 `evaluate` against 2023; record metrics; gate decision.
- [x] 3.5 `evaluate-routes` against 2023; record detour/exposure trade-off.
- [x] 3.6 If gate failed: `calibrate`, pick winner, re-evaluate. *(Not needed — gate passed on first evaluation.)*
- [x] 3.7 `finalize` → `network_temporal_edge_risk_v1` active, trained on all data.

## 4. Go safe-routes domain (`internal/saferoutes/`)
- [x] 4.1 `model.go` / `dto.go` / `errors.go`: response shapes (routes array with metrics +
      GeoJSON), query DTO, sentinel errors.
- [x] 4.2 `repository.go` + `postgres_repository.go`: active-model lookup, route profiles, snap
      validation (≤150 m), `pgr_dijkstra` per profile with parameterized risk cost, `pgr_ksp`
      candidates, per-edge risk + component fetch, merged LineString geometry.
- [x] 4.3 `risk_aggregation.go`: weighted avg edge risk, `route_risk = 0.75·wavg + 0.25·max`,
      risk level thresholds, high-risk meters/percent, vs-fastest comparisons.
- [x] 4.4 `service.go`: validation (CABA bounds, distinct points, datetime parsing/default),
      time-bucket + weekday-type resolution (mirror Python), orchestration of 4 profiles.
- [x] 4.5 `handler.go`: parse params, error mapping (400 invalid / outside-graph, 404 no route,
      503 no active model, 500 generic), register `GET /routes/safe`.
- [x] 4.6 Wire into `internal/app/app.go`.

## 5. Tests
- [x] 5.1 Unit: time bucket/weekday boundary table (Go) mirroring Python; risk aggregation math;
      service validation paths; handler status codes with a stub service.
- [x] 5.2 Integration (`//go:build integration`): live `/routes/safe` query returns 4 routes with
      monotonic risk ordering sanity (safest.risk ≤ fastest.risk) and valid geometry.
- [x] 5.3 Python: pytest-less sanity (assert-based `--self-test` or doctest) for bucket helpers and
      weight functions.
- [x] 5.4 `go build ./...` + `go test ./...` green.

## 6. Verify & archive
- [x] 6.1 End-to-end: `go run ./cmd/api` + curl `/api/v1/routes/safe` with night-time datetime;
      verify all four kinds, metrics fields, and that no narrative text is returned.
- [x] 6.2 Validation queries from the spec (active model, riskiest night edges, risk distribution,
      unsnapped crimes).
- [x] 6.3 Update `openspec/project.md` capabilities list; archive change into
      `openspec/specs/{risk-scoring,safe-routes}/` + road-graph delta merge.
