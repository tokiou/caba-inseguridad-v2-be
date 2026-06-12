# Design — Walkable routing over the local road graph

## Context

`road-graph` left us a clean, routable graph (`road_nodes`, `routable_road_edges`) on
PostgreSQL + PostGIS + pgRouting, plus an empty `edge_risk_scores`. This capability adds the first
path query over that graph. It is deliberately *unweighted by risk* — it computes the plain shortest
walkable path so we can validate topology, snapping, and geometry reconstruction before the scoring
milestone changes the cost function.

## Decisions

### 1. `pgr_dijkstra` over `routable_road_edges`, undirected

We use `pgr_dijkstra` (already available via the `pgrouting` extension, migration `000003`) rather
than hand-rolling Dijkstra in Go. The inner edge SQL reads the **view** `routable_road_edges`, never
raw `road_edges`, so excluded/non-walkable edges are filtered automatically.

- `cost = length_meters` — shortest path by walking distance. (The future risk milestone replaces
  this expression with a risk-weighted cost; the surrounding query is unchanged.)
- `directed := false` — walking is bidirectional; a single `cost` column applies both ways, so no
  `reverse_cost` is needed.

### 2. Coordinate snapping to the nearest *routable edge*

Arbitrary input coordinates won't sit exactly on a graph node, so origin and destination are snapped
into the graph. We snap to the nearest **routable edge** (KNN `geom <-> point` over
`routable_road_edges`, using `road_edges_geom_gist_idx`) and enter the graph at that edge's nearer
endpoint — not to the nearest `road_nodes` row directly.

Snapping to the nearest *node* is buggy here: the quality cleanup marks anomalous edges
non-routable, which can leave a node whose every edge was excluded — an orphan in the routable
subgraph. A nearest-node snap can land on such an orphan and `pgr_dijkstra` then finds no path even
though the point sits on a perfectly routable street. Snapping to the nearest routable edge
guarantees the entry node participates in the routable graph. (This was caught by the integration
test: Obelisco→Palermo returned "no route" until the snap was changed.)

The chosen entry node ids are returned in the response (`snapped_node_id`). No max-snap distance is
enforced in this milestone — the service already constrains inputs to the CABA bounding box, and the
graph covers CABA.

### 3. Single round-trip query

Snap + route + aggregation run in one SQL statement using CTEs:

```sql
WITH src AS (SELECT id FROM road_nodes ORDER BY geom <-> ST_SetSRID(ST_MakePoint($1,$2),4326) LIMIT 1),
     dst AS (SELECT id FROM road_nodes ORDER BY geom <-> ST_SetSRID(ST_MakePoint($3,$4),4326) LIMIT 1),
     path AS (
       SELECT seq, edge FROM pgr_dijkstra(
         'SELECT id, from_node_id AS source, to_node_id AS target, length_meters AS cost FROM routable_road_edges',
         (SELECT id FROM src), (SELECT id FROM dst), false))
SELECT (SELECT id FROM src), (SELECT id FROM dst),
       COALESCE(SUM(re.length_meters),0), COALESCE(SUM(re.walk_duration_seconds),0),
       COUNT(re.id),
       ST_AsGeoJSON(ST_LineMerge(ST_Collect(re.geom ORDER BY path.seq)))
FROM path JOIN road_edges re ON re.id = path.edge
WHERE path.edge <> -1;
```

Aggregation over an empty path yields one row with `edge_count = 0` and a NULL geometry; the
repository maps that to `ErrNoRoute`.

### 4. Path geometry: merged `LineString`

The path edges are collected in `seq` order and merged with `ST_LineMerge`. Because consecutive
edges share identical topology vertices (from the osm2pgrouting import), the merge yields a single
connected `LineString`. The repository unmarshals the `ST_AsGeoJSON` result into `[][]float64`; if
the geometry is NULL (no route) it returns `ErrNoRoute`, and if it is not a `LineString` it returns a
wrapped error (treated as 500) rather than silently degrading.

### 5. Package placement: `internal/roadgraph`

Routing over the graph is a pure graph concern and shares the repository/pool with the stats path, so
it lives in the existing `roadgraph` package (extending its `Repository`/`service` interfaces) rather
than a new directory. This is distinct from `internal/routes` (ORS delegation). Layer rules are
preserved: handler parses HTTP, service validates, repository owns all PostGIS/pgRouting SQL.

## Error mapping

| Condition | Domain error | HTTP |
|---|---|---|
| Missing/unparseable params, out-of-CABA, origin == destination | `ErrInvalidCoordinates` | 400 |
| No path between snapped nodes (disconnected) | `ErrNoRoute` | 404 |
| Query failure / non-LineString geometry | (wrapped) | 500 (generic body) |

## Risks / trade-offs

- **`ST_LineMerge` could return a `MultiLineString`** if the path were disconnected; for a valid
  `pgr_dijkstra` result the edges are contiguous, so this is not expected. The repository fails loudly
  (500) rather than returning a malformed geometry.
- **Snapping far points**: with no max-snap distance, a CABA-bounds point in a park/river with no
  nearby node could snap to a distant node. Acceptable for this milestone; a `max_snap_meters` guard
  is a candidate follow-up.
