"""Snap every scoreable crime to its nearest routable edge (set-based)."""

from __future__ import annotations

from . import db

# Every crime gets a snap row against its nearest routable edge; crimes farther
# than max_distance are kept but flagged out of scoring, so rejections stay
# auditable. KNN runs on the geometry GiST index; the precise distance is
# geography (meters). Idempotent: re-runs upsert.
SNAP_SQL = """
INSERT INTO crime_network_snaps (
    crime_id, graph_version_id, snapped_edge_id,
    snap_distance_meters, snap_fraction, snapped_geom,
    is_valid_for_network_scoring, rejection_reason
)
SELECT
    c.id,
    %(graph_version_id)s,
    nearest.edge_id,
    nearest.distance_meters,
    nearest.snap_fraction,
    nearest.snapped_geom,
    nearest.distance_meters <= %(max_distance)s,
    CASE WHEN nearest.distance_meters <= %(max_distance)s
         THEN NULL ELSE 'no_edge_within_snap_distance' END
FROM crimes c
CROSS JOIN LATERAL (
    SELECT
        e.id AS edge_id,
        ST_Distance(c.geom::geography, e.geom::geography) AS distance_meters,
        ST_LineLocatePoint(e.geom, c.geom) AS snap_fraction,
        ST_LineInterpolatePoint(e.geom, ST_LineLocatePoint(e.geom, c.geom)) AS snapped_geom
    FROM routable_road_edges e
    ORDER BY e.geom <-> c.geom
    LIMIT 1
) nearest
WHERE c.geom IS NOT NULL
  AND c.date IS NOT NULL
  AND c.hour BETWEEN 0 AND 23
  AND c.quantity > 0
ON CONFLICT (crime_id, graph_version_id) DO UPDATE SET
    snapped_edge_id = EXCLUDED.snapped_edge_id,
    snap_distance_meters = EXCLUDED.snap_distance_meters,
    snap_fraction = EXCLUDED.snap_fraction,
    snapped_geom = EXCLUDED.snapped_geom,
    is_valid_for_network_scoring = EXCLUDED.is_valid_for_network_scoring,
    rejection_reason = EXCLUDED.rejection_reason
"""

METRICS_SQL = """
SELECT
    (SELECT COUNT(*) FROM crimes) AS crimes_total,
    (SELECT COUNT(*) FROM crimes c
     WHERE c.geom IS NOT NULL AND c.date IS NOT NULL
       AND c.hour BETWEEN 0 AND 23 AND c.quantity > 0) AS crimes_with_valid_geom,
    COUNT(*) FILTER (WHERE s.is_valid_for_network_scoring) AS crimes_snapped,
    COUNT(*) FILTER (WHERE NOT s.is_valid_for_network_scoring) AS crimes_rejected_no_edge,
    percentile_cont(0.50) WITHIN GROUP (ORDER BY s.snap_distance_meters)
        FILTER (WHERE s.is_valid_for_network_scoring) AS snap_distance_p50,
    percentile_cont(0.95) WITHIN GROUP (ORDER BY s.snap_distance_meters)
        FILTER (WHERE s.is_valid_for_network_scoring) AS snap_distance_p95,
    MAX(s.snap_distance_meters)
        FILTER (WHERE s.is_valid_for_network_scoring) AS max_snap_distance
FROM crime_network_snaps s
WHERE s.graph_version_id = %(graph_version_id)s
"""


def run(graph_version: str, max_distance_meters: float) -> dict:
    conn = db.connect()
    try:
        gv_id = db.get_graph_version(conn, graph_version)
        print(f"snapping crimes to graph version {graph_version!r} "
              f"(id={gv_id}, max {max_distance_meters} m)...")
        with conn.cursor() as cur:
            cur.execute(SNAP_SQL, {"graph_version_id": gv_id, "max_distance": max_distance_meters})
            inserted = cur.rowcount
        conn.commit()

        row = db.fetch_one(conn, METRICS_SQL, {"graph_version_id": gv_id})
        metrics = {
            "crimes_total": row[0],
            "crimes_with_valid_geom": row[1],
            "crimes_snapped": row[2],
            "crimes_rejected_no_edge": row[3],
            "snap_distance_p50": round(row[4], 2) if row[4] is not None else None,
            "snap_distance_p95": round(row[5], 2) if row[5] is not None else None,
            "max_snap_distance": round(row[6], 2) if row[6] is not None else None,
            "rows_upserted": inserted,
        }
        for key, value in metrics.items():
            print(f"  {key}: {value}")
        return metrics
    finally:
        conn.close()
