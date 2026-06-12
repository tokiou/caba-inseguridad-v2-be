package saferoutes

import "math"

// aggregateRoute turns a raw path into a SafeRoute. route_risk blends the
// length-weighted average with the worst edge (0.75/0.25) so a single very
// risky stretch cannot be averaged away. Comparison fields vs fastest are
// filled later by setComparisons.
func aggregateRoute(kind string, path RoutePath, model ModelVersion) SafeRoute {
	var (
		totalLength, totalDuration float64
		weightedRisk, maxRisk      float64
		highRiskMeters             float64
		metrics                    CrimeMetrics
	)
	for _, edge := range path.Edges {
		totalLength += edge.LengthMeters
		totalDuration += edge.DurationSeconds
		weightedRisk += edge.LengthMeters * edge.RiskScore
		maxRisk = math.Max(maxRisk, edge.RiskScore)
		if edge.RiskScore > model.ModerateMax {
			highRiskMeters += edge.LengthMeters
		}
		metrics.CrimeCount += edge.CrimeCount
		metrics.RobberyCount += edge.RobberyCount
		metrics.TheftCount += edge.TheftCount
		metrics.ThreatsCount += edge.ThreatsCount
		metrics.ArmedCount += edge.ArmedCount
		metrics.MotorcycleCount += edge.MotorcycleCount
		metrics.SameBucketCrimeCount += edge.SameBucketCrimeCount
	}

	var avgRisk float64
	if totalLength > 0 {
		avgRisk = weightedRisk / totalLength
	}
	riskScore := 0.75*avgRisk + 0.25*maxRisk

	var highRiskPercent float64
	if totalLength > 0 {
		highRiskPercent = 100 * highRiskMeters / totalLength
	}

	return SafeRoute{
		Kind:                kind,
		DistanceMeters:      round1(totalLength),
		DurationMinutes:     round1(totalDuration / 60),
		RiskScore:           round3(riskScore),
		RiskLevel:           riskLevel(riskScore, model),
		HighRiskEdgeMeters:  round1(highRiskMeters),
		HighRiskEdgePercent: round1(highRiskPercent),
		MaxEdgeRisk:         round3(maxRisk),
		AvgEdgeRisk:         round3(avgRisk),
		CrimeMetrics:        metrics,
		Geometry:            path.Geometry,
	}
}

func riskLevel(score float64, model ModelVersion) string {
	switch {
	case score <= model.LowMax:
		return "low"
	case score <= model.ModerateMax:
		return "moderate"
	default:
		return "high"
	}
}

// setComparisons fills the vs-fastest fields. Risk reduction is relative
// (percent of fastest's risk removed); negative means riskier than fastest.
func setComparisons(route *SafeRoute, fastest SafeRoute) {
	route.ExtraDistanceVsFastestMeters = round1(route.DistanceMeters - fastest.DistanceMeters)
	route.ExtraDurationVsFastestMinutes = round1(route.DurationMinutes - fastest.DurationMinutes)
	if fastest.RiskScore > 0 {
		route.RiskReductionVsFastestPercent = round1(
			100 * (fastest.RiskScore - route.RiskScore) / fastest.RiskScore)
	}
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
