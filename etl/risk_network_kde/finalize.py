"""Create + activate the production model from an evaluated base model.

Refuses to finalize a base model without a passed edge-ranking evaluation run.
The final model copies the approved parameters, trains on ALL available data
(train_until = max crime date), and is activated atomically.
"""

from __future__ import annotations

import json

from . import compute_scores, db

PASSED_RUN_SQL = """
SELECT id FROM risk_model_evaluation_runs
WHERE model_version_id = %s AND status = 'completed' AND passed = true
  AND metrics->>'type' = 'edge_ranking_backtest'
ORDER BY id DESC LIMIT 1
"""


def run(base_model_name: str, final_model_name: str,
        train_until: str = "latest", activate: bool = False) -> dict:
    conn = db.connect()
    try:
        base = db.get_model(conn, base_model_name)

        passed_run = db.fetch_one(conn, PASSED_RUN_SQL, (base["id"],))
        if not passed_run:
            raise SystemExit(
                f"base model {base_model_name} has no passed evaluation run — "
                "not finalizing (run `evaluate`, or `calibrate` if it failed)"
            )

        if train_until == "latest":
            train_until = db.fetch_one(conn, "SELECT MAX(date) FROM crimes")[0].isoformat()
        print(f"finalizing {final_model_name} (train_until {train_until}, "
              f"base run {passed_run[0]})")

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
                (final_model_name, base["type"],
                 f"Production Network Temporal KDE model (parameters approved via "
                 f"{base_model_name}, evaluation run {passed_run[0]}).",
                 base["graph_version_id"], json.dumps(base["parameters"]), train_until),
            )
        conn.commit()

        compute_scores.run(final_model_name, train_until)

        if activate:
            with conn.cursor() as cur:
                cur.execute("UPDATE risk_model_versions SET is_active = false WHERE is_active")
                cur.execute(
                    "UPDATE risk_model_versions SET is_active = true WHERE name = %s",
                    (final_model_name,),
                )
            conn.commit()
            print(f"{final_model_name} is now the active model")

        final = db.get_model(conn, final_model_name)
        return {"model_id": final["id"], "train_until": train_until, "active": activate}
    finally:
        conn.close()
