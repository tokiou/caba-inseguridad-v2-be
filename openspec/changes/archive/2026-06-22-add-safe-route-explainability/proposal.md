# Add safe-route explainability metadata

## Why

`GET /api/v1/routes/safe` returns an aggregated `risk_score` / `risk_level` per route plus total
`crime_metrics`, but it does not explain **why** a route is rated more dangerous. In practice two
near-parallel routes that differ by a single block can land on different risk levels, and the
response gives no way to see what drove that.

The cause is by design: the route score is `0.75·length_weighted_avg + 0.25·max_edge_risk`
(`internal/saferoutes/risk_aggregation.go`). The `0.25·max` term is length-agnostic, so one block
with a high edge risk lifts the whole route and can cross a level threshold. We are **not** changing
the scoring formula — instead we surface structured metadata that explains the difference, turning an
opaque number into actionable information.

## What changes

Add per-route, **metric-only** explainability metadata to the safe-routes response (no free-text;
the frontend composes any prose):

1. `riskiest_segment` — the single edge driving the score: its risk, level, length, a representative
   point, and its crime breakdown.
2. `segments[]` — a minimal per-edge list: `risk_score`, `robbery_count`, `length_meters`, and a
   representative `point`. Lets clients compare block-by-block against a neighbouring route.
3. `dominant_factor` + `armed_share_percent` — the predominant crime type on the route and the share
   of incidents involving a weapon.
4. `time_of_day_risk` — the same route's risk recomputed across the four time buckets for the
   resolved `weekday_type`, plus the `peak_bucket`.

All values come from data that already exists in `edge_risk_scores` /
`edge_risk_score_components` (the per-edge crime counts are already SELECTed today but discarded
after summing). **No migration and no model/scoring change.**

## In scope

- New response fields on each `SafeRoute` (all four routes, including `least_safe_candidate`).
- One new repository read for the time-of-day profile; one new SELECT column (a per-edge midpoint).
- Updated frontend integration doc and tests.

## Out of scope

- Changing the risk aggregation formula, normalization, or any model parameter (explicit user
  decision: keep as-is, only explain).
- Hourly granularity — risk is only stored at the four coarse buckets; `time_of_day_risk` is
  bucket-level.
- Per-segment full crime-type breakdown (kept minimal: robbery count only, per user).
