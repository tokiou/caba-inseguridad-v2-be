# Mark disconnected-component edges non-routable

## Why

Route testing of `/api/v1/routes/safe` (safe-route-scoring change, task 6.1 extension) found a real
failure: **Plaza de Mayo → Constitución returned 404 route_not_found**. The routable graph contains
a giant connected component (68,246 of ~71k nodes) plus ~100 small islands (isolated plaza
footpaths, gated passages). Nearest-edge snapping landed the origin on a 2-node island (edge 21113,
an unnamed plaza path 0.6 m from the request point), and `pgr_dijkstra` correctly found no path out
of it. Any client near a plaza interior could hit this.

## What changes

- **Rule 5 in `scripts/osm/cleanup_road_graph.sql`**: after rules 1–4, mark every edge outside the
  largest connected component of the surviving routable graph `is_routable = false` with
  `excluded_reason = 'disconnected_component'` (non-destructive, idempotent, runs last because
  components are computed over the survivors).
- **road-graph spec**: the quality-layer requirement gains the new exclusion reason and a
  connectivity scenario.
- **Pipeline re-run**: snapping, neighborhoods, and scores are recomputed over the shrunken
  routable set so crimes previously snapped to island edges re-snap to the main network and their
  influence reaches real streets.

## In scope

Connectivity-based exclusion, spec delta, pipeline re-run. **Out of scope**: bridging islands to the
main network (a future graph-quality improvement could connect plaza paths to surrounding
sidewalks instead of excluding them).
