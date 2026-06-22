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
		riskiestIdx                = -1
	)
	segments := make([]RouteSegment, 0, len(path.Edges))
	for i, edge := range path.Edges {
		totalLength += edge.LengthMeters
		totalDuration += edge.DurationSeconds
		weightedRisk += edge.LengthMeters * edge.RiskScore
		maxRisk = math.Max(maxRisk, edge.RiskScore)
		if riskiestIdx < 0 || edge.RiskScore > path.Edges[riskiestIdx].RiskScore {
			riskiestIdx = i
		}
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

		segments = append(segments, RouteSegment{
			RiskScore:    round3(edge.RiskScore),
			RobberyCount: edge.RobberyCount,
			LengthMeters: round1(edge.LengthMeters),
			Point:        LatLng{Lat: edge.PointLat, Lng: edge.PointLng},
		})
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

	var armedShare float64
	if metrics.CrimeCount > 0 {
		armedShare = 100 * float64(metrics.ArmedCount) / float64(metrics.CrimeCount)
	}

	var riskiest *RiskiestSegment
	if riskiestIdx >= 0 {
		e := path.Edges[riskiestIdx]
		riskiest = &RiskiestSegment{
			RiskScore:       round3(e.RiskScore),
			RiskLevel:       riskLevel(e.RiskScore, model),
			LengthMeters:    round1(e.LengthMeters),
			Point:           LatLng{Lat: e.PointLat, Lng: e.PointLng},
			CrimeCount:      e.CrimeCount,
			RobberyCount:    e.RobberyCount,
			ArmedCount:      e.ArmedCount,
			TheftCount:      e.TheftCount,
			ThreatsCount:    e.ThreatsCount,
			MotorcycleCount: e.MotorcycleCount,
		}
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
		RiskiestSegment:     riskiest,
		Segments:            segments,
		DominantFactor:      dominantFactor(metrics),
		ArmedSharePercent:   round1(armedShare),
		Geometry:            path.Geometry,
	}
}

// dominantFactor is the crime type with the largest count on the route, or
// "none" when there are no counts. The counts are equally-inflated exposure
// sums, so the comparison is a valid relative ranking.
func dominantFactor(m CrimeMetrics) string {
	switch {
	case m.RobberyCount == 0 && m.TheftCount == 0 && m.ThreatsCount == 0:
		return "none"
	case m.RobberyCount >= m.TheftCount && m.RobberyCount >= m.ThreatsCount:
		return "robbery"
	case m.TheftCount >= m.ThreatsCount:
		return "theft"
	default:
		return "threats"
	}
}

// aggregateBucketRisk recomputes the route's risk for each of the four time
// buckets from per-edge bucket scores, reusing the same 0.75*avg + 0.25*max
// blend and the route's own edge lengths. Edges missing a score for a bucket
// count as zero risk. Returns nil for an empty path.
func aggregateBucketRisk(edges []PathEdge, perBucket []EdgeBucketRisk, model ModelVersion) *TimeOfDayRisk {
	if len(edges) == 0 {
		return nil
	}

	type key struct {
		bucket string
		edge   int64
	}
	scores := make(map[key]float64, len(perBucket))
	for _, b := range perBucket {
		scores[key{b.TimeBucket, b.EdgeID}] = b.RiskScore
	}

	buckets := []string{"morning", "afternoon", "evening", "night"}
	result := make(map[string]BucketRisk, len(buckets))
	for _, bucket := range buckets {
		var totalLength, weighted, maxRisk float64
		for _, e := range edges {
			risk := scores[key{bucket, e.EdgeID}]
			totalLength += e.LengthMeters
			weighted += e.LengthMeters * risk
			maxRisk = math.Max(maxRisk, risk)
		}
		var avg float64
		if totalLength > 0 {
			avg = weighted / totalLength
		}
		score := 0.75*avg + 0.25*maxRisk
		result[bucket] = BucketRisk{RiskScore: round3(score), RiskLevel: riskLevel(score, model)}
	}

	peak := buckets[0]
	for _, bucket := range buckets[1:] {
		if result[bucket].RiskScore > result[peak].RiskScore {
			peak = bucket
		}
	}

	return &TimeOfDayRisk{
		Morning:    result["morning"],
		Afternoon:  result["afternoon"],
		Evening:    result["evening"],
		Night:      result["night"],
		PeakBucket: peak,
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
