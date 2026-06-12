package saferoutes

import "time"

// SafeRoutesQuery is the parsed input for a safe-routes request. At is the
// moment the walk happens (zero value = "now", resolved by the service in
// Buenos Aires local time); it selects the risk context (time bucket + weekday
// type) the scores are read for.
type SafeRoutesQuery struct {
	OriginLat float64
	OriginLng float64
	DestLat   float64
	DestLng   float64
	At        time.Time
}

// SafeRoutesResponse is the endpoint payload: the echoed request, the resolved
// risk context, the active model metadata, and the computed route alternatives.
// Metrics only — no narrative safety claims.
type SafeRoutesResponse struct {
	Origin       LatLng           `json:"origin"`
	Destination  LatLng           `json:"destination"`
	Datetime     string           `json:"datetime"`
	TimeBucket   string           `json:"time_bucket"`
	WeekdayType  string           `json:"weekday_type"`
	ModelVersion ModelVersionInfo `json:"model_version"`
	Routes       []SafeRoute      `json:"routes"`
}

type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// ModelVersionInfo is the active model's identity as exposed to clients.
type ModelVersionInfo struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	TrainUntil string `json:"train_until"`
}
