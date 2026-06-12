"""Route-level simulation: do the scores improve routes, not just rankings?

Generates deterministic (seeded) OD pairs, computes fastest/balanced/safest
(and a least-safe KSP candidate) with the same cost function the API uses, and
measures future exposure (backtest labels of the model's latest evaluation run)
against extra distance. Context: all_day/all.
"""

from __future__ import annotations

import json
import math
import random
import time

from . import db

PAIR_SEED = 20230101
MIN_PAIR_METERS = 500
MAX_PAIR_METERS = 8000

NODES_SQL = """
SELECT n.id, ST_X(n.geom), ST_Y(n.geom)
FROM road_nodes n
WHERE EXISTS (
    SELECT 1 FROM routable_road_edges e
    WHERE e.from_node_id = n.id OR e.to_node_id = n.id
)
ORDER BY n.id
"""

LATEST_RUN_SQL = """
SELECT id FROM risk_model_evaluation_runs
WHERE model_version_id = %s AND status = 'completed'
  AND metrics->>'type' = 'edge_ranking_backtest'
ORDER BY id DESC LIMIT 1
"""

# The inner pgr_dijkstra SQL embeds multiplier/model id; both are validated
# numerics (never user input), mirroring the Go repository's approach.
ROUTE_SQL_TEMPLATE = """
WITH path AS (
    SELECT edge FROM pgr_dijkstra(
        $q$SELECT e.id, e.from_node_id AS source, e.to_node_id AS target,
                  e.length_meters * (1 + {multiplier} * COALESCE(r.risk_score, 0)) AS cost
           FROM routable_road_edges e
           LEFT JOIN edge_risk_scores r
             ON r.edge_id = e.id AND r.model_version_id = {model_id}
            AND r.time_bucket = 'all_day' AND r.weekday_type = 'all'$q$,
        %(src)s, %(dst)s, false)
    WHERE edge <> -1
)
SELECT
    COALESCE(SUM(e.length_meters), 0),
    COALESCE(SUM(e.walk_duration_seconds), 0),
    COALESCE(SUM(e.length_meters * COALESCE(r.risk_score, 0)), 0),
    COALESCE(MAX(COALESCE(r.risk_score, 0)), 0),
    COALESCE(SUM(COALESCE(l.future_weighted_crime_score, 0)), 0),
    COUNT(*)
FROM path
JOIN road_edges e ON e.id = path.edge
LEFT JOIN edge_risk_scores r
  ON r.edge_id = e.id AND r.model_version_id = %(model_id)s
 AND r.time_bucket = 'all_day' AND r.weekday_type = 'all'
LEFT JOIN edge_risk_backtest_labels l
  ON l.edge_id = e.id AND l.evaluation_run_id = %(run_id)s
 AND l.time_bucket = 'all_day' AND l.weekday_type = 'all'
"""

KSP_SQL_TEMPLATE = """
SELECT path_id, edge FROM pgr_ksp(
    $q$SELECT id, from_node_id AS source, to_node_id AS target,
              length_meters AS cost
       FROM routable_road_edges$q$,
    %(src)s, %(dst)s, {k}, directed := false)
WHERE edge <> -1
ORDER BY path_id, seq
"""

EDGE_METRICS_SQL = """
SELECT e.id, e.length_meters,
       COALESCE(r.risk_score, 0),
       COALESCE(l.future_weighted_crime_score, 0)
FROM road_edges e
LEFT JOIN edge_risk_scores r
  ON r.edge_id = e.id AND r.model_version_id = %(model_id)s
 AND r.time_bucket = 'all_day' AND r.weekday_type = 'all'
LEFT JOIN edge_risk_backtest_labels l
  ON l.edge_id = e.id AND l.evaluation_run_id = %(run_id)s
 AND l.time_bucket = 'all_day' AND l.weekday_type = 'all'
WHERE e.id = ANY(%(edge_ids)s)
"""


def _haversine_meters(lng1, lat1, lng2, lat2):
    r = 6371000.0
    p1, p2 = math.radians(lat1), math.radians(lat2)
    dp = p2 - p1
    dl = math.radians(lng2 - lng1)
    a = math.sin(dp / 2) ** 2 + math.cos(p1) * math.cos(p2) * math.sin(dl / 2) ** 2
    return 2 * r * math.asin(math.sqrt(a))


def _route_risk(weighted_len_risk, total_len, max_risk):
    if total_len <= 0:
        return 0.0
    return 0.75 * (weighted_len_risk / total_len) + 0.25 * max_risk


def _sample_pairs(nodes, count):
    rng = random.Random(PAIR_SEED)
    pairs = []
    attempts = 0
    while len(pairs) < count and attempts < count * 200:
        attempts += 1
        a = rng.choice(nodes)
        b = rng.choice(nodes)
        if a[0] == b[0]:
            continue
        d = _haversine_meters(a[1], a[2], b[1], b[2])
        if MIN_PAIR_METERS <= d <= MAX_PAIR_METERS:
            pairs.append((a[0], b[0]))
    return pairs


def run(model_name: str, test_from: str, test_to: str,
        pairs: int = 100, ksp_k: int = 10, skip_least_safe: bool = False) -> dict:
    started = time.time()
    conn = db.connect()
    try:
        model = db.get_model(conn, model_name)
        run_row = db.fetch_one(conn, LATEST_RUN_SQL, (model["id"],))
        if not run_row:
            raise SystemExit("no completed edge-ranking evaluation run found; run `evaluate` first")
        label_run_id = run_row[0]

        nodes = db.fetch_all(conn, NODES_SQL)
        od_pairs = _sample_pairs(nodes, pairs)
        print(f"route simulation: {len(od_pairs)} OD pairs, labels from run {label_run_id}")

        profiles = {"fastest": 0.0, "balanced": 1.5, "safest": 3.0}
        results = {name: [] for name in profiles}
        results["least_safe_candidate"] = []

        for i, (src, dst) in enumerate(od_pairs):
            per_profile = {}
            for name, multiplier in profiles.items():
                sql = ROUTE_SQL_TEMPLATE.format(
                    multiplier=float(multiplier), model_id=int(model["id"])
                )
                row = db.fetch_one(conn, sql, {
                    "src": src, "dst": dst, "model_id": model["id"], "run_id": label_run_id,
                })
                length, duration, weighted, max_risk, exposure, edge_count = row
                if edge_count == 0:
                    per_profile = None
                    break
                per_profile[name] = {
                    "distance": float(length),
                    "duration_min": float(duration) / 60.0,
                    "risk": _route_risk(float(weighted), float(length), float(max_risk)),
                    "exposure": float(exposure),
                }
            if not per_profile:
                continue

            if not skip_least_safe:
                ksp_rows = db.fetch_all(conn, KSP_SQL_TEMPLATE.format(k=int(ksp_k)), {
                    "src": src, "dst": dst,
                })
                paths = {}
                for path_id, edge in ksp_rows:
                    paths.setdefault(path_id, []).append(edge)
                worst = None
                max_detour = per_profile["fastest"]["distance"] * 1.75
                for edge_ids in paths.values():
                    metrics_rows = db.fetch_all(conn, EDGE_METRICS_SQL, {
                        "model_id": model["id"], "run_id": label_run_id, "edge_ids": edge_ids,
                    })
                    length = sum(r[1] for r in metrics_rows)
                    if length > max_detour:
                        continue
                    weighted = sum(r[1] * r[2] for r in metrics_rows)
                    max_risk = max((r[2] for r in metrics_rows), default=0.0)
                    exposure = sum(r[3] for r in metrics_rows)
                    risk = _route_risk(weighted, length, max_risk)
                    if worst is None or risk > worst["risk"]:
                        worst = {"distance": length, "risk": risk, "exposure": exposure}
                if worst:
                    fastest = per_profile["fastest"]
                    results["least_safe_candidate"].append({
                        "extra_distance_pct": 100 * (worst["distance"] / fastest["distance"] - 1),
                        "exposure_reduction_pct":
                            100 * (1 - worst["exposure"] / fastest["exposure"])
                            if fastest["exposure"] > 0 else 0.0,
                        "risk": worst["risk"],
                    })

            fastest = per_profile["fastest"]
            for name in ("balanced", "safest"):
                p = per_profile[name]
                results[name].append({
                    "extra_distance_pct": 100 * (p["distance"] / fastest["distance"] - 1)
                        if fastest["distance"] > 0 else 0.0,
                    "exposure_reduction_pct": 100 * (1 - p["exposure"] / fastest["exposure"])
                        if fastest["exposure"] > 0 else 0.0,
                    "risk": p["risk"],
                })
            results["fastest"].append({"risk": fastest["risk"],
                                       "distance": fastest["distance"],
                                       "duration_min": fastest["duration_min"]})

            if (i + 1) % 20 == 0:
                print(f"  {i + 1}/{len(od_pairs)} pairs ({time.time() - started:.0f}s)")

        def mean(rows, key):
            values = [r[key] for r in rows if key in r]
            return round(sum(values) / len(values), 2) if values else None

        summary = {
            "type": "route_simulation",
            "pairs_evaluated": len(results["fastest"]),
            "context": "all_day/all",
            "balanced": {
                "mean_extra_distance_pct": mean(results["balanced"], "extra_distance_pct"),
                "mean_exposure_reduction_pct": mean(results["balanced"], "exposure_reduction_pct"),
            },
            "safest": {
                "mean_extra_distance_pct": mean(results["safest"], "extra_distance_pct"),
                "mean_exposure_reduction_pct": mean(results["safest"], "exposure_reduction_pct"),
            },
            "least_safe_candidate": {
                "mean_extra_distance_pct": mean(results["least_safe_candidate"], "extra_distance_pct"),
                "mean_exposure_reduction_pct": mean(
                    results["least_safe_candidate"], "exposure_reduction_pct"),
            },
        }

        balanced_ok = (
            summary["balanced"]["mean_extra_distance_pct"] is not None
            and summary["balanced"]["mean_extra_distance_pct"] <= 15
            and summary["balanced"]["mean_exposure_reduction_pct"] >= 10
        )
        safest_ok = (
            summary["safest"]["mean_extra_distance_pct"] is not None
            and summary["safest"]["mean_extra_distance_pct"] <= 35
            and summary["safest"]["mean_exposure_reduction_pct"] >= 20
        )
        passed = bool(balanced_ok and safest_ok)
        summary["balanced"]["passed"] = balanced_ok
        summary["safest"]["passed"] = safest_ok

        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO risk_model_evaluation_runs
                    (model_version_id, train_until, test_from, test_to, status,
                     metrics, parameters, passed, completed_at)
                VALUES (%s, %s, %s, %s, 'completed', %s, %s, %s, now())
                RETURNING id
                """,
                (model["id"], model["train_until"], test_from, test_to,
                 json.dumps(summary), json.dumps(model["parameters"]), passed),
            )
            sim_run_id = cur.fetchone()[0]
        conn.commit()

        print(json.dumps(summary, indent=2))
        print(f"route simulation run {sim_run_id} passed={passed} "
              f"({time.time() - started:.0f}s)")
        return {"run_id": sim_run_id, "passed": passed, "metrics": summary}
    finally:
        conn.close()
