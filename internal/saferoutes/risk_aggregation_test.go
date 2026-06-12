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
