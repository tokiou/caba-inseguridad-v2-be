# Tasks — safe-route explainability metadata

## 1. Spec
- [x] 1.1 Write delta `specs/safe-routes/spec.md` (MODIFIED "Route metadata is metric-only").
- [x] 1.2 `openspec validate --all --strict` passes.

## 2. Repository (`internal/saferoutes/repository.go`)
- [x] 2.1 Add `point_lng` / `point_lat` (`ST_LineInterpolatePoint(e.geom, 0.5)`) to the per-edge
      `json_build_object` in both `routeQueryTemplate` and `kspQueryTemplate`.
- [x] 2.2 Add `RouteRiskByBucket(ctx, edgeIDs, modelID, weekdayType)` to the `Repository` interface
      and implement it (single `edge_id = ANY($1)` query over the four buckets).

## 3. Model (`internal/saferoutes/model.go`)
- [x] 3.1 `PathEdge`: add `PointLat` / `PointLng` (json `point_lat` / `point_lng`).
- [x] 3.2 New types: `RiskiestSegment`, `RouteSegment`, `BucketRisk`, `TimeOfDayRisk`,
      `EdgeBucketRisk`, `LatLng` reuse.
- [x] 3.3 `SafeRoute`: add `RiskiestSegment`, `Segments`, `DominantFactor`, `ArmedSharePercent`,
      `TimeOfDayRisk`.

## 4. Aggregation (`internal/saferoutes/risk_aggregation.go`)
- [x] 4.1 In `aggregateRoute`: track riskiest edge → `riskiest_segment`; build `segments[]`
      (risk + robbery_count + length + point); compute `dominant_factor` and `armed_share_percent`.
- [x] 4.2 New pure func `aggregateBucketRisk(edges, perBucket, model) *TimeOfDayRisk` reusing the
      `0.75·avg + 0.25·max` formula and `riskLevel`; set `peak_bucket`.

## 5. Service (`internal/saferoutes/service.go`)
- [x] 5.1 After aggregating each route, call `RouteRiskByBucket` with the path's edge ids and attach
      `time_of_day_risk`.

## 6. Tests
- [x] 6.1 `risk_aggregation_test.go`: riskiest_segment / segments / dominant_factor /
      armed_share_percent / aggregateBucketRisk (peak_bucket).
- [x] 6.2 `service_test.go`: extend the fake repository with the new method; assert new fields.
- [x] 6.3 Integration test for `RouteRiskByBucket` (`//go:build integration`).

## 7. Docs
- [x] 7.1 Update `docs/api/safe-routes-frontend-integration.md` (§4 structure, §6 TS types, §8 how
      the FE composes the explanation).

## 8. Verify & archive
- [x] 8.1 `go build ./...`, `go test ./internal/saferoutes/...`, end-to-end curl.
- [x] 8.2 Archive the change and merge the delta into `openspec/specs/safe-routes/spec.md`.
