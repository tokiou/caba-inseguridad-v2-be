package roadgraph

// WalkRouteQuery is the parsed input for a walkable-route request: the origin and
// destination coordinates (WGS84) to route between over the local road graph.
type WalkRouteQuery struct {
	FromLat float64
	FromLng float64
	ToLat   float64
	ToLng   float64
}
