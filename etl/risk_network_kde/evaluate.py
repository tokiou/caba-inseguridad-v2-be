"""Temporal backtest: do scores trained up to train_until rank future crime?

Builds edge_risk_backtest_labels from the test window (same snap/propagation
machinery, NO recency decay), then computes length-weighted ranking metrics per
context and records pass/fail against the acceptance gate.
"""

from __future__ import annotations

import json
import time

import numpy as np
import pandas as pd

from . import db
from .compute_scores import STAGE2_SQL, build_params, stage1_sql
from .config import EVALUATION_GATE
from .utils import CONTEXTS, severity_case_sql

LABELS_INSERT_SQL = """
INSERT INTO edge_risk_backtest_labels (
    edge_id, evaluation_run_id, time_bucket, weekday_type,
    future_weighted_crime_score, future_crime_count,
    future_robbery_count, future_armed_count, future_motorcycle_count
)
SELECT
    edge_id, %(run_id)s, time_bucket, weekday_type,
    raw_score, crime_count, robbery_count, armed_count, motorcycle_count
FROM ctx_scores
"""

CONTEXT_FRAME_SQL = """
SELECT
    e.id AS edge_id,
    e.length_meters,
    COALESCE(r.risk_score, 0) AS risk_score,
    COALESCE(l.future_weighted_crime_score, 0) AS future_score,
    COALESCE(l.future_crime_count, 0) AS future_crime_count
FROM routable_road_edges e
LEFT JOIN edge_risk_scores r
  ON r.edge_id = e.id AND r.model_version_id = %(model_id)s
 AND r.time_bucket = %(time_bucket)s AND r.weekday_type = %(weekday_type)s
LEFT JOIN edge_risk_backtest_labels l
  ON l.edge_id = e.id AND l.evaluation_run_id = %(run_id)s
 AND l.time_bucket = %(time_bucket)s AND l.weekday_type = %(weekday_type)s
"""


def build_labels(conn, model: dict, run_id: int, test_from: str, test_to: str) -> int:
    params = build_params(model, model["graph_version_id"], test_from, test_to, test_to)
    params["run_id"] = run_id
    severity_case = severity_case_sql(model["parameters"]["severity_base_weights"])
    with conn.cursor() as cur:
        cur.execute(stage1_sql(severity_case, with_recency=False), params)
        cur.execute(STAGE2_SQL, params)
        cur.execute(
            "DELETE FROM edge_risk_backtest_labels WHERE evaluation_run_id = %s", (run_id,)
        )
        cur.execute(LABELS_INSERT_SQL, params)
        rows = cur.rowcount
        cur.execute("DROP TABLE crime_edge_base, ctx_scores")
    return rows


def _share_captured(frame: pd.DataFrame, length_fraction: float) -> tuple[float, float]:
    """(future-score share, with-crime precision) captured by the top
    length_fraction of network length, ranking by risk DESC."""
    total_future = frame["future_score"].sum()
    cutoff = frame["length_meters"].sum() * length_fraction
    cumulative = frame["length_meters"].cumsum()
    top = frame[cumulative <= cutoff]
    if top.empty:
        top = frame.iloc[:1]
    share = top["future_score"].sum() / total_future if total_future > 0 else 0.0
    precision = (top["future_crime_count"] > 0).mean() if len(top) else 0.0
    return float(share), float(precision)


def _decile_densities(frame: pd.DataFrame) -> list[float]:
    """Future-score density (score per meter) of 10 length deciles, decile 1 =
    lowest predicted risk, decile 10 = highest."""
    ascending = frame.iloc[::-1].reset_index(drop=True)  # frame comes risk DESC
    total_length = ascending["length_meters"].sum()
    bounds = ascending["length_meters"].cumsum() / total_length
    densities = []
    for d in range(10):
        mask = (bounds > d / 10) & (bounds <= (d + 1) / 10)
        chunk = ascending[mask]
        length = chunk["length_meters"].sum()
        densities.append(float(chunk["future_score"].sum() / length) if length > 0 else 0.0)
    return densities


def context_metrics(frame: pd.DataFrame) -> dict:
    frame = frame.sort_values(
        ["risk_score", "edge_id"], ascending=[False, True]
    ).reset_index(drop=True)

    share5, precision5 = _share_captured(frame, 0.05)
    share10, _ = _share_captured(frame, 0.10)
    densities = _decile_densities(frame)
    median_density = densities[4]  # decile 5 = the median decile
    lift = densities[9] / median_density if median_density > 0 else float("inf")

    spearman = frame["risk_score"].corr(frame["future_score"], method="spearman")

    return {
        "pai_top5": round(share5 / 0.05, 3),
        "pai_top10": round(share10 / 0.10, 3),
        "recall_top10": round(share10, 4),
        "precision_top5": round(precision5, 4),
        "top_decile_lift": round(lift, 3) if np.isfinite(lift) else None,
        "spearman": round(float(spearman), 4) if pd.notna(spearman) else None,
        "decile_densities": [round(d, 6) for d in densities],
        "future_score_total": round(float(frame["future_score"].sum()), 2),
    }


def apply_gate(per_context: dict) -> tuple[bool, list[str]]:
    """Gate over the 8 timed contexts (all_day reported but not gated)."""
    timed = {k: v for k, v in per_context.items() if not k.startswith("all_day")}
    failures = []

    def mean_of(metric):
        values = [v[metric] for v in timed.values() if v[metric] is not None]
        return sum(values) / len(values) if values else 0.0

    if mean_of("pai_top5") < EVALUATION_GATE["pai_top5_min"]:
        failures.append(f"mean PAI@Top5 {mean_of('pai_top5'):.2f} < {EVALUATION_GATE['pai_top5_min']}")
    if mean_of("recall_top10") < EVALUATION_GATE["recall_top10_min"]:
        failures.append(
            f"mean Recall@Top10 {mean_of('recall_top10'):.3f} < {EVALUATION_GATE['recall_top10_min']}"
        )
    if mean_of("top_decile_lift") < EVALUATION_GATE["top_decile_lift_min"]:
        failures.append(
            f"mean TopDecileLift {mean_of('top_decile_lift'):.2f} < {EVALUATION_GATE['top_decile_lift_min']}"
        )
    for name, m in timed.items():
        d = m["decile_densities"]
        if not (d[9] > d[4] and d[9] > d[0]):
            failures.append(f"context {name}: decile 10 not above deciles 5 and 1")
    return (len(failures) == 0), failures


def run(model_name: str, test_from: str, test_to: str) -> dict:
    started = time.time()
    conn = db.connect()
    try:
        model = db.get_model(conn, model_name)
        if not model["train_until"]:
            raise SystemExit(f"model {model_name} has no train_until")
        if model["train_until"].isoformat() >= test_from:
            raise SystemExit("test window must start after train_until (no leakage)")

        with conn.cursor() as cur:
            cur.execute(
                """
                INSERT INTO risk_model_evaluation_runs
                    (model_version_id, train_until, test_from, test_to, status, parameters)
                VALUES (%s, %s, %s, %s, 'running', %s)
                RETURNING id
                """,
                (model["id"], model["train_until"], test_from, test_to,
                 json.dumps(model["parameters"])),
            )
            run_id = cur.fetchone()[0]
        conn.commit()

        print(f"evaluation run {run_id}: building labels {test_from}..{test_to}...")
        label_rows = build_labels(conn, model, run_id, test_from, test_to)
        conn.commit()
        print(f"  {label_rows} label rows ({time.time() - started:.0f}s)")

        per_context = {}
        for bucket, weekday in CONTEXTS:
            rows = db.fetch_all(conn, CONTEXT_FRAME_SQL, {
                "model_id": model["id"], "run_id": run_id,
                "time_bucket": bucket, "weekday_type": weekday,
            })
            frame = pd.DataFrame(
                rows,
                columns=["edge_id", "length_meters", "risk_score",
                         "future_score", "future_crime_count"],
            ).astype(float)
            key = f"{bucket}/{weekday}"
            per_context[key] = context_metrics(frame)
            print(f"  {key}: PAI@5={per_context[key]['pai_top5']} "
                  f"Recall@10={per_context[key]['recall_top10']} "
                  f"Lift={per_context[key]['top_decile_lift']} "
                  f"Spearman={per_context[key]['spearman']}")

        passed, failures = apply_gate(per_context)
        metrics = {
            "type": "edge_ranking_backtest",
            "per_context": per_context,
            "gate": EVALUATION_GATE,
            "failures": failures,
        }
        with conn.cursor() as cur:
            cur.execute(
                """
                UPDATE risk_model_evaluation_runs
                SET status = 'completed', metrics = %s, passed = %s,
                    failure_reason = %s, completed_at = now()
                WHERE id = %s
                """,
                (json.dumps(metrics), passed, "; ".join(failures) or None, run_id),
            )
        conn.commit()

        print(f"PASSED: {passed}" + (f" — {failures}" if failures else ""))
        print(f"duration: {time.time() - started:.0f}s")
        return {"run_id": run_id, "passed": passed, "metrics": metrics}
    finally:
        conn.close()
