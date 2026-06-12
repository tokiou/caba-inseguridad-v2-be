"""Precompute which edges are reachable from each edge by walking distance.

For each source edge: Dijkstra seeded at both endpoints with length/2 (entering
the network from the edge's midpoint), expanded up to the bandwidth. A target
edge's distance is min(dist[from], dist[to]) + target_length/2; pairs beyond
the bandwidth are not stored. The self pair is stored at distance 0.

Pure-Python heapq is enough: ~100k bounded searches over a city-block graph.
Rows stream to Postgres with COPY.
"""

from __future__ import annotations

import heapq
import io
import time

from . import db
from .graph_loader import load_graph

COPY_SQL = """
COPY edge_network_neighborhoods (
    graph_version_id, source_edge_id, target_edge_id,
    bandwidth_meters, network_distance_meters
) FROM STDIN
"""

FLUSH_ROWS = 200_000


def _bounded_dijkstra(graph, seeds, bound):
    """Distances from multi-source seeds, expanded while dist <= bound."""
    dist = {}
    heap = [(d, node) for node, d in seeds.items()]
    heapq.heapify(heap)
    while heap:
        d, node = heapq.heappop(heap)
        if node in dist:
            continue
        dist[node] = d
        for neighbor, length in graph.adjacency[node]:
            nd = d + length
            if nd <= bound and neighbor not in dist:
                heapq.heappush(heap, (nd, neighbor))
    return dist


def run(graph_version: str, bandwidth_meters: float) -> dict:
    started = time.time()
    conn = db.connect()
    try:
        gv_id = db.get_graph_version(conn, graph_version)
        graph = load_graph(conn)

        with conn.cursor() as cur:
            cur.execute(
                """
                DELETE FROM edge_network_neighborhoods
                WHERE graph_version_id = %s AND bandwidth_meters = %s
                """,
                (gv_id, bandwidth_meters),
            )
            print(f"cleared {cur.rowcount} previous rows for bandwidth {bandwidth_meters}")

        buffer = io.StringIO()
        buffered = 0
        total_rows = 0
        target_counts = []

        def flush():
            nonlocal buffered, total_rows
            if buffered == 0:
                return
            buffer.seek(0)
            with conn.cursor() as cur:
                cur.copy_expert(COPY_SQL, buffer)
            total_rows += buffered
            buffer.seek(0)
            buffer.truncate(0)
            buffered = 0

        edge_ids = sorted(graph.edges)
        report_every = max(1, len(edge_ids) // 20)
        for i, source_edge in enumerate(edge_ids):
            from_node, to_node, length = graph.edges[source_edge]
            half = length / 2.0
            seeds = {from_node: half}
            # A self-loop edge has one distinct endpoint; min() keeps the rule.
            seeds[to_node] = min(seeds.get(to_node, float("inf")), half)
            dist = _bounded_dijkstra(graph, seeds, bandwidth_meters)

            # Candidate targets: every edge incident to a reached node.
            seen = 0
            visited_targets = set()
            for node in dist:
                for target_edge in graph.incident[node]:
                    if target_edge in visited_targets:
                        continue
                    visited_targets.add(target_edge)
                    if target_edge == source_edge:
                        continue
                    t_from, t_to, t_length = graph.edges[target_edge]
                    best = min(dist.get(t_from, float("inf")), dist.get(t_to, float("inf")))
                    network_distance = best + t_length / 2.0
                    if network_distance <= bandwidth_meters:
                        buffer.write(
                            f"{gv_id}\t{source_edge}\t{target_edge}\t"
                            f"{bandwidth_meters}\t{network_distance:.3f}\n"
                        )
                        buffered += 1
                        seen += 1

            buffer.write(f"{gv_id}\t{source_edge}\t{source_edge}\t{bandwidth_meters}\t0\n")
            buffered += 1
            target_counts.append(seen + 1)

            if buffered >= FLUSH_ROWS:
                flush()
            if (i + 1) % report_every == 0:
                print(f"  {i + 1}/{len(edge_ids)} edges "
                      f"({total_rows + buffered} rows, {time.time() - started:.0f}s)")

        flush()
        conn.commit()

        target_counts.sort()
        n = len(target_counts)
        metrics = {
            "edges_total": n,
            "neighborhood_rows_created": total_rows,
            "avg_targets_per_source_edge": round(sum(target_counts) / n, 1) if n else 0,
            "p50_targets_per_source_edge": target_counts[n // 2] if n else 0,
            "p95_targets_per_source_edge": target_counts[int(n * 0.95)] if n else 0,
            "max_targets_per_source_edge": target_counts[-1] if n else 0,
            "duration_seconds": round(time.time() - started, 1),
        }
        for key, value in metrics.items():
            print(f"  {key}: {value}")
        return metrics
    finally:
        conn.close()
