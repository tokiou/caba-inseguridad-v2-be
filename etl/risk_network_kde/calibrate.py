"""Parameter calibration: bounded grid -> candidate model versions -> ranking.

Only meant to run when the evaluation gate fails. Every candidate is persisted
as its own risk_model_versions row plus an evaluation run, so the search stays
reproducible and auditable. Selection score:
0.45*norm(PAI@5) + 0.35*norm(Recall@10) + 0.20*norm(TopDecileLift).
"""

from __future__ import annotations

import itertools
import json

from . import build_neighborhoods, compute_scores, db, evaluate
from .config import CALIBRATION_GRID, SELECTION_WEIGHTS


def _grid_combinations():
    keys = sorted(CALIBRATION_GRID)
    for values in itertools.product(*(CALIBRATION_GRID[k] for k in keys)):
        yield dict(zip(keys, values))


def _ensure_neighborhoods(conn, graph_version_name: str, gv_id: int, bandwidth: float):
    row = db.fetch_one(
        conn,
        """
        SELECT COUNT(*) FROM edge_network_neighborhoods
        WHERE graph_version_id = %s AND bandwidth_meters = %s
        """,
        (gv_id, bandwidth),
    )
    if row[0] == 0:
        print(f"neighborhoods missing for bandwidth {bandwidth}, building...")
        build_neighborhoods.run(graph_version_name, bandwidth)


def _mean_timed(per_context: dict, metric: str) -> float:
    values = [
        v[metric]
        for k, v in per_context.items()
        if not k.startswith("all_day") and v[metric] is not None
    ]
    return sum(values) / len(values) if values else 0.0


def run(base_model_name: str, train_until: str, test_from: str, test_to: str,
        max_candidates: int | None = None) -> dict:
    conn = db.connect()
    try:
        base = db.get_model(conn, base_model_name)
        gv_id = base["graph_version_id"]
        gv_name = db.fetch_one(
            conn, "SELECT name FROM road_graph_versions WHERE id = %s", (gv_id,)
        )[0]

        combos = list(_grid_combinations())
        if max_candidates:
            combos = combos[:max_candidates]
        print(f"calibrating {len(combos)} candidates "
              f"(train<={train_until}, test {test_from}..{test_to})")

        candidates = []
        for index, overrides in enumerate(combos, start=1):
            name = f"{base_model_name.replace('_eval_2022', '')}_candidate_{index:03d}"
            params = json.loads(json.dumps(base["parameters"]))
            params.update(overrides)

            _ensure_neighborhoods(conn, gv_name, gv_id, float(params["network_bandwidth_meters"]))

            with conn.cursor() as cur:
                cur.execute(
                    """
                    INSERT INTO risk_model_versions
                        (name, type, description, graph_version_id, parameters,
                         train_until, is_active)
                    VALUES (%s, %s, %s, %s, %s, %s, false)
                    ON CONFLICT (name) DO UPDATE SET
                        parameters = EXCLUDED.parameters,
                        train_until = EXCLUDED.train_until
                    """,
                    (name, base["type"], f"Calibration candidate {index} of {base_model_name}.",
                     gv_id, json.dumps(params), train_until),
                )
            conn.commit()

            print(f"[{index}/{len(combos)}] {name}: {overrides}")
            compute_scores.run(name, train_until)
            result = evaluate.run(name, test_from, test_to)
            per_context = result["metrics"]["per_context"]
            candidates.append({
                "name": name,
                "overrides": overrides,
                "pai_top5": _mean_timed(per_context, "pai_top5"),
                "recall_top10": _mean_timed(per_context, "recall_top10"),
                "top_decile_lift": _mean_timed(per_context, "top_decile_lift"),
                "passed": result["passed"],
            })

        def norm(metric):
            values = [c[metric] for c in candidates]
            low, high = min(values), max(values)
            spread = (high - low) or 1.0
            return {c["name"]: (c[metric] - low) / spread for c in candidates}

        n_pai, n_recall, n_lift = norm("pai_top5"), norm("recall_top10"), norm("top_decile_lift")
        for c in candidates:
            c["selection_score"] = round(
                SELECTION_WEIGHTS["pai_top5"] * n_pai[c["name"]]
                + SELECTION_WEIGHTS["recall_top10"] * n_recall[c["name"]]
                + SELECTION_WEIGHTS["top_decile_lift"] * n_lift[c["name"]],
                4,
            )

        candidates.sort(key=lambda c: c["selection_score"], reverse=True)
        print("\ncandidate ranking:")
        for c in candidates[:10]:
            print(f"  {c['selection_score']:.4f} {c['name']} "
                  f"PAI@5={c['pai_top5']:.2f} R@10={c['recall_top10']:.3f} "
                  f"Lift={c['top_decile_lift']:.2f} passed={c['passed']}")

        winner = candidates[0]
        print(f"\nwinner: {winner['name']} — verify route quality with evaluate-routes "
              "before finalizing (discard candidates producing absurd detours).")
        return {"winner": winner, "candidates": candidates}
    finally:
        conn.close()
