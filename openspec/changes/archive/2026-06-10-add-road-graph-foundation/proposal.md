# Proposal — CABA road graph + edge-risk foundation

## Why

The product goal is a **safe walking route** across CABA: given two points, return a path that
minimizes exposure to crime hotspots. We already have crimes in PostgreSQL + PostGIS and a
`/crimes/nearby` read path. To compute routes we need a **walkable road graph** (nodes + edges) and a
place to attach **per-edge risk** so a future cost function can trade distance against crime exposure.

This milestone builds only that foundation: import OpenStreetMap walking data for CABA, turn it into
a clean internal graph, and create the risk schema (model versions + per-edge scores). It does **not**
implement routing, scoring, or the `/safe-routes` endpoint — those are explicit future milestones. The
deliverable is: *a real, queryable walkable graph of CABA in Postgres, plus the tables routing/scoring
will write to*.

## What

- **DB schema** (migrations): `road_nodes`, `road_edges`, `risk_model_versions`, `edge_risk_scores`,
  with the GiST/B-tree indexes and check constraints needed for routing and geospatial work, plus a
  seed `active` risk model `v1_crime_density_distance_decay`.
- **Offline OSM import pipeline** (`scripts/osm/`): download the CABA `.pbf`, run `osm2pgrouting` into
  raw tables, and normalize those raw tables into our clean `road_nodes` / `road_edges`. The `.pbf`
  and the raw `osm_*` tables are **offline build inputs** — the Go API never queries them directly.
- **`internal/roadgraph` Go package**: a read-only **graph-status** capability following the project's
  `handler → service → repository` layering, exposing `GET /api/v1/roadgraph/stats` (node/edge/walkable
  /risk-scored counts + graph bounds). This is how we *prove* the graph imported correctly.

## In scope

- Migrations for the four tables + indexes + constraints + seed risk model.
- `scripts/osm/download_caba_osm.sh`, `import_osm_graph.sh`, `normalize_osm_graph.sql`.
- `internal/roadgraph/` (model, dto, repository interface, postgres repository, service, handler,
  errors) + unit tests (service, handler) and a build-tagged integration test for the repository.
- App wiring (`internal/app`) and route registration under `/api/v1`.
- Docs: `openspec/project.md` (new capability + toolchain), `CLAUDE.md` (new domain + scripts).

## Out of scope (explicitly deferred)

- `/safe-routes` endpoint, Dijkstra / A* / pgRouting path queries.
- The edge-risk scoring worker and the risk formula itself (only the **tables** are created now;
  `edge_risk_scores` starts empty, `risk_scored_edges = 0`).
- Redis cache, route alternatives, frontend map rendering.
- Backfilling `road_edges.highway_type` from OSM way classes (left `NULL` this milestone — see design).
- Auth on the `/roadgraph/stats` endpoint (no auth exists in the project yet).

## Acceptance

1. The four tables + indexes + constraints exist; seed model `v1_crime_density_distance_decay` is
   `active`.
2. `scripts/osm/` can download the `.pbf`, import via `osm2pgrouting`, and normalize into the clean
   tables idempotently.
3. After import+normalize, `GET /api/v1/roadgraph/stats` returns 200 with **non-zero** `nodes_count`,
   `edges_count`, `walkable_edges`, and a bounding box inside CABA; `risk_scored_edges = 0`.
4. No `/safe-routes` / routing code is added. Existing `/crimes/nearby` keeps working;
   `go build ./...` and `go test ./...` stay green.
