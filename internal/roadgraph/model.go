package roadgraph

// GraphStats is a read-only status snapshot of the walkable road graph: how much
// has been imported, how much has been risk-scored, and the geographic bounds.
// An empty graph is a valid state (all counts zero, zero-valued bounding box).
type GraphStats struct {
	NodesCount      int64   `json:"nodes_count"`
	EdgesCount      int64   `json:"edges_count"`
	WalkableEdges   int64   `json:"walkable_edges"`
	RoutableEdges   int64   `json:"routable_edges"`
	ExcludedEdges   int64   `json:"excluded_edges"`
	RiskScoredEdges int64   `json:"risk_scored_edges"`
	MinLat          float64 `json:"min_lat"`
	MinLng          float64 `json:"min_lng"`
	MaxLat          float64 `json:"max_lat"`
	MaxLng          float64 `json:"max_lng"`
}
