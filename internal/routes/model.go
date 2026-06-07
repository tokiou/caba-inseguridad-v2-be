package routes

type Route struct {
	Distance float64           `json:"distance_meters"`
	Duration float64           `json:"duration_seconds"`
	Geometry GeoJSONLineString `json:"geometry"`
}

type GeoJSONLineString struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

type Waypoint struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}
