# Road Graph — Delta Specification

## REMOVED Requirements

### Requirement: Risk model versioning

**Reason**: superseded by the `risk-scoring` capability. The placeholder `status`-based
`risk_model_versions` (and its `v1_crime_density_distance_decay` seed) is replaced by the
restructured table (`type`, `graph_version_id`, `parameters`, `train_until`, `is_active` with a
partial unique index) defined in `risk-scoring`. The table held no production data.

### Requirement: Per-edge risk score storage

**Reason**: superseded by the `risk-scoring` capability. `edge_risk_scores` is restructured with a
temporal key (`time_bucket`, `weekday_type`) plus `raw_score` / `p95_reference`; it had never been
populated. The road-graph capability keeps only the graph itself (nodes, edges, quality layer,
stats endpoint); risk storage now belongs to `risk-scoring`.
