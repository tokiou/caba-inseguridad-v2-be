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

// WalkRoute is the shortest walkable path between two points over the local road
// graph: the snapped endpoints, total distance/duration, how many edges the path
// traverses, and the merged path geometry as a GeoJSON LineString.
type WalkRoute struct {
	From            RoutePoint        `json:"from"`
	To              RoutePoint        `json:"to"`
	DistanceMeters  float64           `json:"distance_meters"`
	DurationSeconds float64           `json:"duration_seconds"`
	EdgeCount       int               `json:"edge_count"`
	Geometry        GeoJSONLineString `json:"geometry"`
}

// RoutePoint echoes a requested coordinate alongside the graph node it was snapped
// to (the node the path actually started from / ended at).
type RoutePoint struct {
	Lat           float64 `json:"lat"`
	Lng           float64 `json:"lng"`
	SnappedNodeID int64   `json:"snapped_node_id"`
}

// GeoJSONLineString is the path geometry: ordered [longitude, latitude] pairs.
type GeoJSONLineString struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}
