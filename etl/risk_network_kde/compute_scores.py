"""Compute edge_risk_scores + edge_risk_score_components for a model version.

Set-based, two-stage (see design.md of the add-safe-route-scoring change):

1. Collapse (crime x reachable edge) pairs to (edge, crime_bucket,
   crime_weekday) groups carrying the context-independent base contribution.
   This is the only pass over the ~10^8 pair space, done entirely in SQL.
2. Cross the groups with the 9 contexts applying temporal/weekday weights,
   normalize per context with p95-clamp over ALL routable edges (zeros
   included), and insert scores + components.

Only crimes dated <= train_until contribute. Idempotent per model version.
"""

from __future__ import annotations

import time

from . import db
from .utils import (
    SQL_ADJACENT_PAIRS,
    SQL_CONTEXTS_VALUES,
    SQL_TIME_BUCKET,
    SQL_WEEKDAY_TYPE,
    severity_case_sql,
)


def stage1_sql(severity_case: str, with_recency: bool) -> str:
    """Build the collapse query. with_recency=False is used by the backtest
    labels builder (the future is not discounted)."""
    recency = (
        "power(0.5, GREATEST(%(ref_date)s::date - c.date, 0)::double precision"
        " / %(half_life)s)"
        if with_recency
        else "1.0"
    )
    return f"""
CREATE TEMP TABLE crime_edge_base AS
SELECT
    n.target_edge_id AS edge_id,
    {SQL_TIME_BUCKET} AS crime_bucket,
    {SQL_WEEKDAY_TYPE} AS crime_weekday,
    SUM(x.contribution) AS base_score,
    MAX(x.contribution) AS max_contribution,
    SUM(c.quantity) AS incident_count,
    SUM(CASE WHEN upper(c.crime_type) = 'ROBO' THEN c.quantity ELSE 0 END) AS robbery_count,
    SUM(CASE WHEN upper(c.crime_type) = 'HURTO' THEN c.quantity ELSE 0 END) AS theft_count,
    SUM(CASE WHEN upper(c.crime_type) = 'AMENAZAS' THEN c.quantity ELSE 0 END) AS threats_count,
    SUM(CASE WHEN c.weapon_used THEN c.quantity ELSE 0 END) AS armed_count,
    SUM(CASE WHEN c.motorcycle_used THEN c.quantity ELSE 0 END) AS motorcycle_count
FROM crime_network_snaps s
JOIN crimes c ON c.id = s.crime_id
JOIN edge_network_neighborhoods n
  ON n.graph_version_id = s.graph_version_id
 AND n.source_edge_id = s.snapped_edge_id
 AND n.bandwidth_meters = %(bandwidth)s
CROSS JOIN LATERAL (
    SELECT c.quantity
         * ({severity_case})
         * (CASE WHEN c.weapon_used THEN %(weapon_multiplier)s ELSE 1.0 END)
         * (CASE WHEN c.motorcycle_used THEN %(motorcycle_multiplier)s ELSE 1.0 END)
         * exp(-s.snap_distance_meters / %(snap_decay)s)
         * exp(-n.network_distance_meters / %(network_decay)s)
         * {recency} AS contribution
) x
WHERE s.graph_version_id = %(graph_version_id)s
  AND s.is_valid_for_network_scoring
  AND c.date >= %(date_from)s
  AND c.date <= %(date_to)s
GROUP BY 1, 2, 3
"""


# Stage 2: expand groups to the 9 contexts. all_day/all forces both weights to
# 1.0; for counting purposes every crime is 'same bucket' in all_day.
STAGE2_SQL = f"""
CREATE TEMP TABLE ctx_scores AS
SELECT
    b.edge_id,
    ctx.time_bucket,
    ctx.weekday_type,
    SUM(b.base_score * w.temporal_weight * w.weekday_weight) AS raw_score,
    MAX(b.max_contribution * w.temporal_weight * w.weekday_weight) AS max_single_crime_contribution,
    SUM(b.incident_count) AS crime_count,
    SUM(b.robbery_count) AS robbery_count,
    SUM(b.theft_count) AS theft_count,
    SUM(b.threats_count) AS threats_count,
    SUM(b.armed_count) AS armed_count,
    SUM(b.motorcycle_count) AS motorcycle_count,
    SUM(CASE WHEN w.bucket_class = 'same' THEN b.incident_count ELSE 0 END) AS same_bucket_crime_count,
    SUM(CASE WHEN w.bucket_class = 'adjacent' THEN b.incident_count ELSE 0 END) AS adjacent_bucket_crime_count,
    SUM(CASE WHEN w.bucket_class = 'other' THEN b.incident_count ELSE 0 END) AS other_bucket_crime_count
FROM crime_edge_base b
CROSS JOIN (VALUES {SQL_CONTEXTS_VALUES}) AS ctx(time_bucket, weekday_type)
CROSS JOIN LATERAL (
    SELECT
        CASE
            WHEN ctx.time_bucket = 'all_day' THEN 'same'
            WHEN b.crime_bucket = ctx.time_bucket THEN 'same'
            WHEN (b.crime_bucket, ctx.time_bucket) IN {SQL_ADJACENT_PAIRS} THEN 'adjacent'
            ELSE 'other'
        END AS bucket_class,
        CASE
            WHEN ctx.time_bucket = 'all_day' THEN 1.0
            WHEN b.crime_bucket = ctx.time_bucket THEN %(tw_same)s
            WHEN (b.crime_bucket, ctx.time_bucket) IN {SQL_ADJACENT_PAIRS} THEN %(tw_adjacent)s
            ELSE %(tw_other)s
        END AS temporal_weight,
        CASE
            WHEN ctx.weekday_type = 'all' THEN %(ww_all)s
            WHEN b.crime_weekday = ctx.weekday_type THEN %(ww_same)s
            ELSE %(ww_other)s
        END AS weekday_weight
) w
GROUP BY 1, 2, 3
"""

# Stage 3: every routable edge x every context (zeros where no crime reaches),
# p95 per context over that full surface, clamp, insert.
STAGE3_SCORES_SQL = f"""
WITH full_surface AS (
    SELECT e.id AS edge_id, ctx.time_bucket, ctx.weekday_type,
           COALESCE(s.raw_score, 0) AS raw_score
    FROM routable_road_edges e
    CROSS JOIN (VALUES {SQL_CONTEXTS_VALUES}) AS ctx(time_bucket, weekday_type)
    LEFT JOIN ctx_scores s
      ON s.edge_id = e.id
     AND s.time_bucket = ctx.time_bucket
     AND s.weekday_type = ctx.weekday_type
),
p95 AS (
    SELECT time_bucket, weekday_type,
           percentile_cont(0.95) WITHIN GROUP (ORDER BY raw_score) AS p95_reference
    FROM full_surface
    GROUP BY 1, 2
)
INSERT INTO edge_risk_scores (
    edge_id, model_version_id, time_bucket, weekday_type,
    raw_score, risk_score, p95_reference
)
SELECT
    f.edge_id, %(model_id)s, f.time_bucket, f.weekday_type,
    f.raw_score,
    CASE WHEN p.p95_reference > 0 THEN LEAST(f.raw_score / p.p95_reference, 1.0) ELSE 0 END,
    p.p95_reference
FROM full_surface f
JOIN p95 p USING (time_bucket, weekday_type)
"""

STAGE3_COMPONENTS_SQL = """
INSERT INTO edge_risk_score_components (
    edge_id, model_version_id, time_bucket, weekday_type,
    crime_count, weighted_crime_score,
    robbery_count, theft_count, threats_count,
    armed_count, motorcycle_count,
    same_bucket_crime_count, adjacent_bucket_crime_count, other_bucket_crime_count,
    max_single_crime_contribution
)
SELECT
    edge_id, %(model_id)s, time_bucket, weekday_type,
    crime_count, raw_score,
    robbery_count, theft_count, threats_count,
    armed_count, motorcycle_count,
    same_bucket_crime_count, adjacent_bucket_crime_count, other_bucket_crime_count,
    max_single_crime_contribution
FROM ctx_scores
"""


def build_params(model: dict, graph_version_id: int, date_from, date_to, ref_date) -> dict:
    p = model["parameters"]
    return {
        "graph_version_id": graph_version_id,
        "bandwidth": float(p["network_bandwidth_meters"]),
        "snap_decay": float(p["snap_distance_decay_meters"]),
        "network_decay": float(p["network_distance_decay_meters"]),
        "half_life": float(p["recency_half_life_days"]),
        "weapon_multiplier": float(p["weapon_multiplier"]),
        "motorcycle_multiplier": float(p["motorcycle_multiplier"]),
        "tw_same": float(p["temporal_weights"]["same_bucket"]),
        "tw_adjacent": float(p["temporal_weights"]["adjacent_bucket"]),
        "tw_other": float(p["temporal_weights"]["other_bucket"]),
        "ww_same": float(p["weekday_weights"]["same_type"]),
        "ww_other": float(p["weekday_weights"]["other_type"]),
        "ww_all": float(p["weekday_weights"]["all"]),
        "date_from": date_from,
        "date_to": date_to,
        "ref_date": ref_date,
    }


def run(model_name: str, train_until: str | None = None) -> dict:
    started = time.time()
    conn = db.connect()
    try:
        model = db.get_model(conn, model_name)
        until = train_until or (model["train_until"] and model["train_until"].isoformat())
        if not until:
            raise SystemExit(f"model {model_name} has no train_until; pass --train-until")
        if model["train_until"] and train_until and model["train_until"].isoformat() != train_until:
            raise SystemExit(
                f"--train-until {train_until} != model.train_until {model['train_until']}"
            )

        gv_id = model["graph_version_id"]
        params = build_params(model, gv_id, "1900-01-01", until, until)
        params["model_id"] = model["id"]
        severity_case = severity_case_sql(model["parameters"]["severity_base_weights"])

        print(f"computing scores for {model_name} (train_until {until})...")
        with conn.cursor() as cur:
            cur.execute(stage1_sql(severity_case, with_recency=True), params)
            cur.execute("SELECT COUNT(*) FROM crime_edge_base")
            groups = cur.fetchone()[0]
            print(f"  stage 1: {groups} (edge, bucket, weekday) groups "
                  f"({time.time() - started:.0f}s)")

            cur.execute(STAGE2_SQL, params)
            print(f"  stage 2: contexts expanded ({time.time() - started:.0f}s)")

            cur.execute("DELETE FROM edge_risk_scores WHERE model_version_id = %s", (model["id"],))
            cur.execute(
                "DELETE FROM edge_risk_score_components WHERE model_version_id = %s",
                (model["id"],),
            )
            cur.execute(STAGE3_SCORES_SQL, params)
            scores = cur.rowcount
            cur.execute(STAGE3_COMPONENTS_SQL, params)
            components = cur.rowcount
            cur.execute("DROP TABLE crime_edge_base, ctx_scores")
        conn.commit()

        # The delete + bulk insert churns millions of rows; without fresh
        # statistics the API's first post-recompute queries can pick terrible
        # plans (observed: a 1 s route request degrading to >60 s).
        with conn.cursor() as cur:
            cur.execute("ANALYZE edge_risk_scores")
            cur.execute("ANALYZE edge_risk_score_components")
        conn.commit()

        metrics = {
            "score_rows": scores,
            "component_rows": components,
            "duration_seconds": round(time.time() - started, 1),
        }
        for key, value in metrics.items():
            print(f"  {key}: {value}")
        return metrics
    finally:
        conn.close()
