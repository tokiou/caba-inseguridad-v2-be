package crimes

type NearbyCrimesQuery struct {
	Lat          float64
	Lng          float64
	RadiusMeters int
	Limit        int
	Cursor       *Cursor
}

type NearbyCrimesResponse struct {
	Lat          float64 `json:"lat"`
	Lng          float64 `json:"lng"`
	RadiusMeters int     `json:"radius_meters"`
	Count        int     `json:"count"`
	Items        []Crime `json:"items"`
	NextCursor   *string `json:"next_cursor"`
	HasMore      bool    `json:"has_more"`
}
