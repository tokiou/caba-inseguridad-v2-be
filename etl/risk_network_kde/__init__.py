"""Offline Network Temporal KDE risk pipeline.

Turns reported crimes into deterministic per-edge risk scores over the CABA
walkable graph (network distance, severity, weapon, motorcycle, recency, time
bucket, weekday type). Produces the tables the Go API consumes:
crime_network_snaps, edge_network_neighborhoods, edge_risk_scores,
edge_risk_score_components. No machine learning.
"""
