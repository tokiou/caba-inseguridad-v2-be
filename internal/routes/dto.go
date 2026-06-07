package routes

type RouteQuery struct {
	OriginLat float64
	OriginLng float64
	DestLat   float64
	DestLng   float64
	Profile   string
}

type RouteResponse struct {
	Origin      Waypoint          `json:"origin"`
	Destination Waypoint          `json:"destination"`
	Profile     string            `json:"profile"`
	Distance    float64           `json:"distance_meters"`
	Duration    float64           `json:"duration_seconds"`
	Geometry    GeoJSONLineString `json:"geometry"`
}
