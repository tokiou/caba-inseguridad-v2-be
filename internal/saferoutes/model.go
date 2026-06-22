package saferoutes

// ModelVersion is the active risk model as the service needs it: identity plus
// the risk-level thresholds from its parameters.
type ModelVersion struct {
	ID          int64
	Name        string
	Type        string
	TrainUntil  string
	LowMax      float64 // risk_score <= LowMax      -> "low"
	ModerateMax float64 // risk_score <= ModerateMax -> "moderate", else "high"
}

// RouteProfile is a row of route_profiles: how strongly risk inflates edge cost
// and how much detour the profile tolerates.
type RouteProfile struct {
	Name             string
	SafetyMultiplier float64
	MaxDetourRatio   float64
}

// SnapResult is where the requested endpoints entered the routable graph.
type SnapResult struct {
	OriginNodeID   int64
	OriginDistance float64
	DestNodeID     int64
	DestDistance   float64
}

// RouteRequest asks the repository for one risk-weighted shortest path.
// SafetyMultiplier 0 = plain distance.
type RouteRequest struct {
	FromNodeID       int64
	ToNodeID         int64
	ModelVersionID   int64
	TimeBucket       string
	WeekdayType      string
	SafetyMultiplier float64
}

// CandidateRouteRequest asks for the K shortest paths by plain distance
// (least-safe-candidate selection pool).
type CandidateRouteRequest struct {
	FromNodeID     int64
	ToNodeID       int64
	ModelVersionID int64
	TimeBucket     string
	WeekdayType    string
	K              int
}

// PathEdge is one traversed edge with its risk and precomputed crime components
// for the resolved context. PointLat/PointLng are the edge midpoint, used to
// place each segment on a map.
type PathEdge struct {
	EdgeID               int64   `json:"edge_id"`
	LengthMeters         float64 `json:"length_meters"`
	DurationSeconds      float64 `json:"duration_seconds"`
	RiskScore            float64 `json:"risk_score"`
	CrimeCount           int64   `json:"crime_count"`
	RobberyCount         int64   `json:"robbery_count"`
	TheftCount           int64   `json:"theft_count"`
	ThreatsCount         int64   `json:"threats_count"`
	ArmedCount           int64   `json:"armed_count"`
	MotorcycleCount      int64   `json:"motorcycle_count"`
	SameBucketCrimeCount int64   `json:"same_bucket_crime_count"`
	PointLat             float64 `json:"point_lat"`
	PointLng             float64 `json:"point_lng"`
}

// EdgeBucketRisk is one (edge, time_bucket) risk score, used to build the
// per-route time-of-day risk profile.
type EdgeBucketRisk struct {
	EdgeID     int64
	TimeBucket string
	RiskScore  float64
}

// RoutePath is a raw path as returned by the routing engine, before metric
// aggregation.
type RoutePath struct {
	Edges    []PathEdge
	Geometry GeoJSONLineString
}

// GeoJSONLineString is the path geometry: ordered [longitude, latitude] pairs.
type GeoJSONLineString struct {
	Type        string      `json:"type"`
	Coordinates [][]float64 `json:"coordinates"`
}

// CrimeMetrics aggregates per-edge component metrics over a route. Values are
// sums of edge-level estimated exposure, not distinct incidents: a crime that
// influences several consecutive edges counts in each.
type CrimeMetrics struct {
	CrimeCount           int64 `json:"crime_count"`
	RobberyCount         int64 `json:"robbery_count"`
	TheftCount           int64 `json:"theft_count"`
	ThreatsCount         int64 `json:"threats_count"`
	ArmedCount           int64 `json:"armed_count"`
	MotorcycleCount      int64 `json:"motorcycle_count"`
	SameBucketCrimeCount int64 `json:"same_bucket_crime_count"`
}

// RiskiestSegment is the single highest-risk edge on a route — the block that
// drives the route's max_edge_risk component. Metric-only; the client composes
// any prose ("riskier because of this block").
type RiskiestSegment struct {
	RiskScore       float64 `json:"risk_score"`
	RiskLevel       string  `json:"risk_level"`
	LengthMeters    float64 `json:"length_meters"`
	Point           LatLng  `json:"point"`
	CrimeCount      int64   `json:"crime_count"`
	RobberyCount    int64   `json:"robbery_count"`
	ArmedCount      int64   `json:"armed_count"`
	TheftCount      int64   `json:"theft_count"`
	ThreatsCount    int64   `json:"threats_count"`
	MotorcycleCount int64   `json:"motorcycle_count"`
}

// RouteSegment is the minimal per-edge view, in path order: enough to compare
// two routes block by block. Crime detail is intentionally limited to the
// robbery count.
type RouteSegment struct {
	RiskScore    float64 `json:"risk_score"`
	RobberyCount int64   `json:"robbery_count"`
	LengthMeters float64 `json:"length_meters"`
	Point        LatLng  `json:"point"`
}

// BucketRisk is a route's aggregated risk for one time bucket.
type BucketRisk struct {
	RiskScore float64 `json:"risk_score"`
	RiskLevel string  `json:"risk_level"`
}

// TimeOfDayRisk is the same route's risk across the four time buckets for the
// resolved weekday type, plus the worst (peak) bucket. Bucket granularity only.
type TimeOfDayRisk struct {
	Morning    BucketRisk `json:"morning"`
	Afternoon  BucketRisk `json:"afternoon"`
	Evening    BucketRisk `json:"evening"`
	Night      BucketRisk `json:"night"`
	PeakBucket string     `json:"peak_bucket"`
}

// SafeRoute is one fully aggregated route alternative.
type SafeRoute struct {
	Kind                          string            `json:"kind"`
	DistanceMeters                float64           `json:"distance_meters"`
	DurationMinutes               float64           `json:"duration_minutes"`
	RiskScore                     float64           `json:"risk_score"`
	RiskLevel                     string            `json:"risk_level"`
	ExtraDistanceVsFastestMeters  float64           `json:"extra_distance_vs_fastest_meters"`
	ExtraDurationVsFastestMinutes float64           `json:"extra_duration_vs_fastest_minutes"`
	RiskReductionVsFastestPercent float64           `json:"risk_reduction_vs_fastest_percent"`
	HighRiskEdgeMeters            float64           `json:"high_risk_edge_meters"`
	HighRiskEdgePercent           float64           `json:"high_risk_edge_percent"`
	MaxEdgeRisk                   float64           `json:"max_edge_risk"`
	AvgEdgeRisk                   float64           `json:"avg_edge_risk"`
	CrimeMetrics                  CrimeMetrics      `json:"crime_metrics"`
	RiskiestSegment               *RiskiestSegment  `json:"riskiest_segment,omitempty"`
	Segments                      []RouteSegment    `json:"segments,omitempty"`
	DominantFactor                string            `json:"dominant_factor"`
	ArmedSharePercent             float64           `json:"armed_share_percent"`
	TimeOfDayRisk                 *TimeOfDayRisk    `json:"time_of_day_risk,omitempty"`
	Geometry                      GeoJSONLineString `json:"geometry"`
}
