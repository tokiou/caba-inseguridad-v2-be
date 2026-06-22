package saferoutes

import (
	"math"
	"testing"
)

var testModel = ModelVersion{ID: 1, LowMax: 0.33, ModerateMax: 0.66}

func TestAggregateRouteBlendsAverageAndMax(t *testing.T) {
	// 20 low-risk edges + 1 maximal edge: the 0.25*max component must keep the
	// route risk well above the weighted average.
	edges := make([]PathEdge, 0, 21)
	for i := 0; i < 20; i++ {
		edges = append(edges, PathEdge{LengthMeters: 100, DurationSeconds: 71.4, RiskScore: 0.1})
	}
	edges = append(edges, PathEdge{LengthMeters: 100, DurationSeconds: 71.4, RiskScore: 1.0})

	route := aggregateRoute("fastest", RoutePath{Edges: edges}, testModel)

	wantAvg := (20*100*0.1 + 100*1.0) / 2100.0
	if math.Abs(route.AvgEdgeRisk-round3(wantAvg)) > 1e-9 {
		t.Fatalf("avg edge risk = %v, want %v", route.AvgEdgeRisk, round3(wantAvg))
	}
	wantRisk := round3(0.75*wantAvg + 0.25*1.0)
	if route.RiskScore != wantRisk {
		t.Fatalf("route risk = %v, want %v", route.RiskScore, wantRisk)
	}
	if route.RiskScore <= route.AvgEdgeRisk {
		t.Fatalf("route risk %v must exceed weighted average %v", route.RiskScore, route.AvgEdgeRisk)
	}
	if route.MaxEdgeRisk != 1.0 {
		t.Fatalf("max edge risk = %v, want 1.0", route.MaxEdgeRisk)
	}
}

func TestAggregateRouteHighRiskMetersAndLevel(t *testing.T) {
	route := aggregateRoute("safest", RoutePath{Edges: []PathEdge{
		{LengthMeters: 300, RiskScore: 0.1},
		{LengthMeters: 100, RiskScore: 0.8},  // above ModerateMax -> high-risk length
		{LengthMeters: 100, RiskScore: 0.66}, // exactly ModerateMax -> not high
	}}, testModel)

	if route.HighRiskEdgeMeters != 100 {
		t.Fatalf("high risk meters = %v, want 100", route.HighRiskEdgeMeters)
	}
	if route.HighRiskEdgePercent != 20 {
		t.Fatalf("high risk percent = %v, want 20", route.HighRiskEdgePercent)
	}
}

func TestAggregateRouteSumsCrimeMetrics(t *testing.T) {
	route := aggregateRoute("balanced", RoutePath{Edges: []PathEdge{
		{LengthMeters: 10, CrimeCount: 5, RobberyCount: 3, ArmedCount: 1, SameBucketCrimeCount: 2},
		{LengthMeters: 10, CrimeCount: 7, RobberyCount: 2, TheftCount: 4, MotorcycleCount: 1},
	}}, testModel)

	want := CrimeMetrics{CrimeCount: 12, RobberyCount: 5, TheftCount: 4,
		ArmedCount: 1, MotorcycleCount: 1, SameBucketCrimeCount: 2}
	if route.CrimeMetrics != want {
		t.Fatalf("crime metrics = %+v, want %+v", route.CrimeMetrics, want)
	}
}

func TestAggregateRouteEmptyPath(t *testing.T) {
	route := aggregateRoute("fastest", RoutePath{}, testModel)
	if route.DistanceMeters != 0 || route.RiskScore != 0 || route.RiskLevel != "low" {
		t.Fatalf("empty path should aggregate to zeros/low, got %+v", route)
	}
}

func TestAggregateRouteRiskiestSegment(t *testing.T) {
	route := aggregateRoute("balanced", RoutePath{Edges: []PathEdge{
		{EdgeID: 10, LengthMeters: 100, RiskScore: 0.2, PointLat: -34.1, PointLng: -58.1},
		{EdgeID: 20, LengthMeters: 100, RiskScore: 0.9, RobberyCount: 4, ArmedCount: 2, PointLat: -34.2, PointLng: -58.2},
		{EdgeID: 30, LengthMeters: 100, RiskScore: 0.5, PointLat: -34.3, PointLng: -58.3},
	}}, testModel)

	if route.RiskiestSegment == nil {
		t.Fatal("riskiest_segment is nil")
	}
	if route.RiskiestSegment.RiskScore != route.MaxEdgeRisk {
		t.Errorf("riskiest risk = %v, want route max %v", route.RiskiestSegment.RiskScore, route.MaxEdgeRisk)
	}
	if route.RiskiestSegment.RiskScore != 0.9 {
		t.Errorf("riskiest risk = %v, want 0.9", route.RiskiestSegment.RiskScore)
	}
	if route.RiskiestSegment.Point != (LatLng{Lat: -34.2, Lng: -58.2}) {
		t.Errorf("riskiest point = %+v, want edge 20 midpoint", route.RiskiestSegment.Point)
	}
	if route.RiskiestSegment.RobberyCount != 4 || route.RiskiestSegment.ArmedCount != 2 {
		t.Errorf("riskiest crime breakdown wrong: %+v", route.RiskiestSegment)
	}
	if route.RiskiestSegment.RiskLevel != "high" {
		t.Errorf("riskiest level = %q, want high", route.RiskiestSegment.RiskLevel)
	}
}

func TestAggregateRouteSegments(t *testing.T) {
	route := aggregateRoute("safest", RoutePath{Edges: []PathEdge{
		{EdgeID: 1, LengthMeters: 120, RiskScore: 0.2, RobberyCount: 1, PointLat: -34.1, PointLng: -58.1},
		{EdgeID: 2, LengthMeters: 80, RiskScore: 0.7, RobberyCount: 5, PointLat: -34.2, PointLng: -58.2},
	}}, testModel)

	if len(route.Segments) != 2 {
		t.Fatalf("segments = %d, want 2", len(route.Segments))
	}
	// Order preserved; robbery count and risk surfaced per segment.
	if route.Segments[1].RobberyCount != 5 || route.Segments[1].RiskScore != 0.7 {
		t.Errorf("segment[1] = %+v", route.Segments[1])
	}
	if route.Segments[0].Point != (LatLng{Lat: -34.1, Lng: -58.1}) {
		t.Errorf("segment[0] point = %+v", route.Segments[0].Point)
	}
	var sum float64
	for _, s := range route.Segments {
		sum += s.LengthMeters
	}
	if sum != route.DistanceMeters {
		t.Errorf("segment length sum = %v, want distance %v", sum, route.DistanceMeters)
	}
}

func TestAggregateRouteDominantFactorAndArmedShare(t *testing.T) {
	route := aggregateRoute("balanced", RoutePath{Edges: []PathEdge{
		{LengthMeters: 10, CrimeCount: 10, RobberyCount: 6, TheftCount: 3, ThreatsCount: 1, ArmedCount: 2},
	}}, testModel)
	if route.DominantFactor != "robbery" {
		t.Errorf("dominant factor = %q, want robbery", route.DominantFactor)
	}
	if route.ArmedSharePercent != 20 {
		t.Errorf("armed share = %v, want 20", route.ArmedSharePercent)
	}

	none := aggregateRoute("fastest", RoutePath{Edges: []PathEdge{{LengthMeters: 10}}}, testModel)
	if none.DominantFactor != "none" {
		t.Errorf("dominant factor = %q, want none", none.DominantFactor)
	}
	if none.ArmedSharePercent != 0 {
		t.Errorf("armed share = %v, want 0", none.ArmedSharePercent)
	}
}

func TestDominantFactorTieBreaks(t *testing.T) {
	cases := []struct {
		m    CrimeMetrics
		want string
	}{
		{CrimeMetrics{}, "none"},
		{CrimeMetrics{RobberyCount: 5, TheftCount: 5, ThreatsCount: 1}, "robbery"}, // tie -> robbery
		{CrimeMetrics{TheftCount: 5, ThreatsCount: 5}, "theft"},                    // tie -> theft
		{CrimeMetrics{ThreatsCount: 3}, "threats"},
	}
	for _, c := range cases {
		if got := dominantFactor(c.m); got != c.want {
			t.Errorf("dominantFactor(%+v) = %q, want %q", c.m, got, c.want)
		}
	}
}

func TestAggregateBucketRisk(t *testing.T) {
	edges := []PathEdge{
		{EdgeID: 1, LengthMeters: 100},
		{EdgeID: 2, LengthMeters: 100},
	}
	perBucket := []EdgeBucketRisk{
		{EdgeID: 1, TimeBucket: "morning", RiskScore: 0.1},
		{EdgeID: 2, TimeBucket: "morning", RiskScore: 0.1},
		{EdgeID: 1, TimeBucket: "night", RiskScore: 0.8},
		{EdgeID: 2, TimeBucket: "night", RiskScore: 0.4},
		// afternoon / evening intentionally missing -> treated as 0.
	}

	tod := aggregateBucketRisk(edges, perBucket, testModel)
	if tod == nil {
		t.Fatal("time of day risk is nil")
	}
	if tod.PeakBucket != "night" {
		t.Errorf("peak bucket = %q, want night", tod.PeakBucket)
	}
	if tod.Morning.RiskScore != 0.1 { // avg 0.1, max 0.1
		t.Errorf("morning = %v, want 0.1", tod.Morning.RiskScore)
	}
	// night: avg = (0.8+0.4)/2 = 0.6, max = 0.8 -> 0.75*0.6 + 0.25*0.8 = 0.65
	if tod.Night.RiskScore != 0.65 {
		t.Errorf("night = %v, want 0.65", tod.Night.RiskScore)
	}
	if tod.Afternoon.RiskScore != 0 || tod.Afternoon.RiskLevel != "low" {
		t.Errorf("afternoon = %+v, want zero/low", tod.Afternoon)
	}

	if aggregateBucketRisk(nil, perBucket, testModel) != nil {
		t.Error("empty path should yield nil time of day risk")
	}
}

func TestRiskLevelThresholds(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0, "low"}, {0.33, "low"}, {0.331, "moderate"},
		{0.66, "moderate"}, {0.661, "high"}, {1, "high"},
	}
	for _, c := range cases {
		if got := riskLevel(c.score, testModel); got != c.want {
			t.Errorf("riskLevel(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}

func TestSetComparisons(t *testing.T) {
	fastest := SafeRoute{DistanceMeters: 1000, DurationMinutes: 12, RiskScore: 0.8}
	route := SafeRoute{DistanceMeters: 1300, DurationMinutes: 15.5, RiskScore: 0.4}

	setComparisons(&route, fastest)

	if route.ExtraDistanceVsFastestMeters != 300 {
		t.Fatalf("extra distance = %v, want 300", route.ExtraDistanceVsFastestMeters)
	}
	if route.ExtraDurationVsFastestMinutes != 3.5 {
		t.Fatalf("extra duration = %v, want 3.5", route.ExtraDurationVsFastestMinutes)
	}
	if route.RiskReductionVsFastestPercent != 50 {
		t.Fatalf("risk reduction = %v, want 50", route.RiskReductionVsFastestPercent)
	}

	// Riskier than fastest -> negative reduction.
	riskier := SafeRoute{DistanceMeters: 1100, RiskScore: 1.0}
	setComparisons(&riskier, fastest)
	if riskier.RiskReductionVsFastestPercent != -25 {
		t.Fatalf("risk reduction = %v, want -25", riskier.RiskReductionVsFastestPercent)
	}

	// Zero fastest risk -> reduction stays 0 (no division by zero).
	zero := SafeRoute{RiskScore: 0.2}
	setComparisons(&zero, SafeRoute{RiskScore: 0})
	if zero.RiskReductionVsFastestPercent != 0 {
		t.Fatalf("risk reduction vs zero-risk fastest = %v, want 0", zero.RiskReductionVsFastestPercent)
	}
}
