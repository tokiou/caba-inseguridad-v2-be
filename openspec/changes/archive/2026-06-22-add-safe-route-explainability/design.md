# Design — safe-route explainability metadata

## Constraints honored

- **Metric-only.** The existing `safe-routes` spec requires "metrics only — no narrative safety
  claims / no free-text explanations". Every new field is a number or a categorical enum; the
  frontend turns them into prose. `dominant_factor` is a fixed enum, not generated text.
- **No model change.** Scoring formula, normalization and parameters are untouched. We only read and
  reshape data already produced offline.
- **No migration.** `edge_risk_score_components` and `edge_risk_scores` already hold every value the
  9 contexts. The route SELECT already returns the per-edge crime counts; today they are summed and
  the per-edge detail is dropped.

## Decisions

### Reuse the per-edge data already fetched
`FindRoute` / `FindCandidateRoutes` already SELECT `risk_score` and the crime counts per edge into
the `PathEdge` list. `riskiest_segment`, `segments[]`, `dominant_factor` and `armed_share_percent`
are computed in `aggregateRoute` from that existing data — **no extra query**. The only SQL change is
one column: a per-edge representative point so each segment is mappable.

### Representative point per edge
Emit `point_lng = ST_X(ST_LineInterpolatePoint(e.geom, 0.5))` and `point_lat = ST_Y(...)` as plain
numbers in the per-edge `json_build_object` (not nested GeoJSON, to keep decoding trivial). The
midpoint is enough to locate the block on a map.

### time_of_day_risk = same path, recomputed per bucket
"Which hours are most unsafe" is answered for the **route the user is looking at**, not by
re-routing. After a route is built we take its edge ids and issue one query:

```sql
SELECT edge_id, time_bucket, risk_score
FROM edge_risk_scores
WHERE edge_id = ANY($1) AND model_version_id = $2 AND weekday_type = $3
  AND time_bucket IN ('morning','afternoon','evening','night')
```

Then a pure function re-applies the same `0.75·avg + 0.25·max` formula per bucket using the edge
lengths already on the path. `peak_bucket` is the argmax. Cost: one extra read per route (3–4 per
request) — acceptable; can be batched later if needed. Buckets with no rows for an edge contribute 0
(LEFT-join semantics via COALESCE / missing rows).

### Correctness caveat (documented in the spec)
Crime counts are **sums of per-edge estimated exposure**, not distinct incidents — one crime
influences several consecutive edges and is counted on each. `dominant_factor` (argmax over
`robbery_count` / `theft_count` / `threats_count`) and `armed_share_percent`
(`armed_count / crime_count`) are **relative** comparisons between quantities inflated the same way,
so they are valid as relative metrics, not as absolute incident counts. The frontend must present
them as relative exposure, never "N delitos en esta ruta".

## Layering

Unchanged: `handler → service → Repository → PostgresRepository`. The new repository method
`RouteRiskByBucket` joins the existing interface (now in the unified `repository.go`). Aggregation
stays in pure, DB-free functions in `risk_aggregation.go` so it remains unit-testable.
