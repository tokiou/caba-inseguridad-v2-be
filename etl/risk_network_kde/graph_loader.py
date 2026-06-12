"""Load the routable walkable graph into adjacency structures for Dijkstra."""

from __future__ import annotations

from collections import defaultdict

from . import db

EDGES_SQL = """
SELECT id, from_node_id, to_node_id, length_meters
FROM routable_road_edges
ORDER BY id
"""


class Graph:
    """Undirected walking graph over routable edges.

    edges: edge_id -> (from_node, to_node, length)
    adjacency: node_id -> list of (neighbor_node, length)
    incident: node_id -> list of edge_id
    """

    def __init__(self, edges):
        self.edges = {}
        self.adjacency = defaultdict(list)
        self.incident = defaultdict(list)
        for edge_id, from_node, to_node, length in edges:
            self.edges[edge_id] = (from_node, to_node, float(length))
            self.adjacency[from_node].append((to_node, float(length)))
            self.adjacency[to_node].append((from_node, float(length)))
            self.incident[from_node].append(edge_id)
            self.incident[to_node].append(edge_id)


def load_graph(conn) -> Graph:
    rows = db.fetch_all(conn, EDGES_SQL)
    print(f"loaded {len(rows)} routable edges")
    return Graph(rows)
