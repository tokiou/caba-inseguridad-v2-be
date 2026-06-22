package saferoutes

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubRepository struct {
	model         ModelVersion
	modelErr      error
	profiles      map[string]RouteProfile
	profilesErr   error
	snap          SnapResult
	snapErr       error
	routes        map[float64]RoutePath // keyed by safety multiplier
	routeErr      error
	candidates    []RoutePath
	candidatesErr error
	bucketRisk    []EdgeBucketRisk
	bucketRiskErr error

	routeRequests []RouteRequest
	bucketEdgeIDs [][]int64
}

func (s *stubRepository) ActiveModel(context.Context) (ModelVersion, error) {
	return s.model, s.modelErr
}

func (s *stubRepository) RouteProfiles(context.Context) (map[string]RouteProfile, error) {
	return s.profiles, s.profilesErr
}

func (s *stubRepository) SnapEndpoints(context.Context, SafeRoutesQuery) (SnapResult, error) {
	return s.snap, s.snapErr
}

func (s *stubRepository) FindRoute(_ context.Context, req RouteRequest) (RoutePath, error) {
	s.routeRequests = append(s.routeRequests, req)
	if s.routeErr != nil {
		return RoutePath{}, s.routeErr
	}
	return s.routes[req.SafetyMultiplier], nil
}

func (s *stubRepository) FindCandidateRoutes(context.Context, CandidateRouteRequest) ([]RoutePath, error) {
	return s.candidates, s.candidatesErr
}

func (s *stubRepository) RouteRiskByBucket(_ context.Context, edgeIDs []int64, _ int64, _ string) ([]EdgeBucketRisk, error) {
	s.bucketEdgeIDs = append(s.bucketEdgeIDs, edgeIDs)
	return s.bucketRisk, s.bucketRiskErr
}

func defaultProfiles() map[string]RouteProfile {
	return map[string]RouteProfile{
		"fastest":  {Name: "fastest", SafetyMultiplier: 0, MaxDetourRatio: 1},
		"balanced": {Name: "balanced", SafetyMultiplier: 1.5, MaxDetourRatio: 1.35},
		"safest":   {Name: "safest", SafetyMultiplier: 3, MaxDetourRatio: 1.75},
	}
}

func pathWithRisk(length, risk float64) RoutePath {
	return RoutePath{
		Edges:    []PathEdge{{EdgeID: 1, LengthMeters: length, DurationSeconds: length / 1.4, RiskScore: risk}},
		Geometry: GeoJSONLineString{Type: "LineString", Coordinates: [][]float64{{-58.4, -34.6}, {-58.39, -34.59}}},
	}
}

func validQuery() SafeRoutesQuery {
	return SafeRoutesQuery{
		OriginLat: -34.58, OriginLng: -58.42,
		DestLat: -34.60, DestLng: -58.38,
		At: time.Date(2026, 6, 12, 23, 0, 0, 0, time.FixedZone("-03", -3*3600)),
	}
}

func workingStub() *stubRepository {
	return &stubRepository{
		model:    ModelVersion{ID: 4, Name: "m", Type: "deterministic_network_kde", LowMax: 0.33, ModerateMax: 0.66},
		profiles: defaultProfiles(),
		snap:     SnapResult{OriginNodeID: 1, OriginDistance: 10, DestNodeID: 2, DestDistance: 20},
		routes: map[float64]RoutePath{
			0:   pathWithRisk(1000, 0.9),
			1.5: pathWithRisk(1200, 0.5),
			3:   pathWithRisk(1500, 0.2),
		},
		candidates: []RoutePath{pathWithRisk(1100, 0.95), pathWithRisk(3000, 1.0)},
	}
}

func TestSafeRoutesHappyPath(t *testing.T) {
	repo := workingStub()
	response, err := NewService(repo).SafeRoutes(context.Background(), validQuery())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response.TimeBucket != "night" {
		t.Fatalf("time bucket = %q, want night", response.TimeBucket)
	}
	if response.WeekdayType != "weekday" { // 2026-06-12 is a Friday
		t.Fatalf("weekday type = %q, want weekday", response.WeekdayType)
	}
	if len(response.Routes) != 4 {
		t.Fatalf("routes = %d, want 4", len(response.Routes))
	}
	kinds := []string{"fastest", "balanced", "safest", "least_safe_candidate"}
	for i, kind := range kinds {
		if response.Routes[i].Kind != kind {
			t.Fatalf("routes[%d].kind = %q, want %q", i, response.Routes[i].Kind, kind)
		}
	}
	// The 3000 m candidate exceeds 1000 * 1.75 and must have been discarded.
	leastSafe := response.Routes[3]
	if leastSafe.DistanceMeters != 1100 {
		t.Fatalf("least safe distance = %v, want the in-bound 1100 candidate", leastSafe.DistanceMeters)
	}
	if response.ModelVersion.ID != 4 {
		t.Fatalf("model id = %d, want 4", response.ModelVersion.ID)
	}
	// fastest/balanced/safest requested with their profile multipliers.
	if len(repo.routeRequests) != 3 {
		t.Fatalf("route requests = %d, want 3", len(repo.routeRequests))
	}
	for i, want := range []float64{0, 1.5, 3} {
		if repo.routeRequests[i].SafetyMultiplier != want {
			t.Fatalf("request %d multiplier = %v, want %v", i, repo.routeRequests[i].SafetyMultiplier, want)
		}
	}
}

func TestSafeRoutesTimeOfDayMetadata(t *testing.T) {
	repo := workingStub()
	// Edge 1 is the only edge in every stub path; night is the worst bucket.
	repo.bucketRisk = []EdgeBucketRisk{
		{EdgeID: 1, TimeBucket: "morning", RiskScore: 0.1},
		{EdgeID: 1, TimeBucket: "afternoon", RiskScore: 0.2},
		{EdgeID: 1, TimeBucket: "evening", RiskScore: 0.5},
		{EdgeID: 1, TimeBucket: "night", RiskScore: 0.9},
	}

	response, err := NewService(repo).SafeRoutes(context.Background(), validQuery())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, route := range response.Routes {
		if route.TimeOfDayRisk == nil {
			t.Fatalf("route %q missing time_of_day_risk", route.Kind)
		}
		if route.TimeOfDayRisk.PeakBucket != "night" {
			t.Errorf("route %q peak bucket = %q, want night", route.Kind, route.TimeOfDayRisk.PeakBucket)
		}
		// Single-edge path → night score = 0.75*0.9 + 0.25*0.9 = 0.9 (high).
		if route.TimeOfDayRisk.Night.RiskScore != 0.9 {
			t.Errorf("route %q night score = %v, want 0.9", route.Kind, route.TimeOfDayRisk.Night.RiskScore)
		}
		if route.TimeOfDayRisk.Night.RiskLevel != "high" {
			t.Errorf("route %q night level = %q, want high", route.Kind, route.TimeOfDayRisk.Night.RiskLevel)
		}
	}

	// Each of the four returned routes triggers exactly one bucket lookup.
	if len(repo.bucketEdgeIDs) != 4 {
		t.Fatalf("bucket lookups = %d, want 4", len(repo.bucketEdgeIDs))
	}
}

func TestSafeRoutesValidation(t *testing.T) {
	service := NewService(workingStub())
	cases := []struct {
		name  string
		query SafeRoutesQuery
	}{
		{"origin outside CABA", SafeRoutesQuery{OriginLat: -31, OriginLng: -58.42, DestLat: -34.6, DestLng: -58.38}},
		{"destination outside CABA", SafeRoutesQuery{OriginLat: -34.58, OriginLng: -58.42, DestLat: -34.6, DestLng: -57}},
		{"identical endpoints", SafeRoutesQuery{OriginLat: -34.58, OriginLng: -58.42, DestLat: -34.58, DestLng: -58.42}},
	}
	for _, c := range cases {
		if _, err := service.SafeRoutes(context.Background(), c.query); !errors.Is(err, ErrInvalidCoordinates) {
			t.Errorf("%s: err = %v, want ErrInvalidCoordinates", c.name, err)
		}
	}
}

func TestSafeRoutesSnapTooFar(t *testing.T) {
	repo := workingStub()
	repo.snap = SnapResult{OriginNodeID: 1, OriginDistance: 151, DestNodeID: 2, DestDistance: 5}
	if _, err := NewService(repo).SafeRoutes(context.Background(), validQuery()); !errors.Is(err, ErrPointOutsideGraph) {
		t.Fatalf("err = %v, want ErrPointOutsideGraph", err)
	}
}

func TestSafeRoutesErrorPropagation(t *testing.T) {
	t.Run("no active model", func(t *testing.T) {
		repo := workingStub()
		repo.modelErr = ErrNoActiveModel
		if _, err := NewService(repo).SafeRoutes(context.Background(), validQuery()); !errors.Is(err, ErrNoActiveModel) {
			t.Fatalf("err = %v, want ErrNoActiveModel", err)
		}
	})
	t.Run("no route", func(t *testing.T) {
		repo := workingStub()
		repo.routeErr = ErrNoRoute
		if _, err := NewService(repo).SafeRoutes(context.Background(), validQuery()); !errors.Is(err, ErrNoRoute) {
			t.Fatalf("err = %v, want ErrNoRoute", err)
		}
	})
	t.Run("missing profile seed", func(t *testing.T) {
		repo := workingStub()
		delete(repo.profiles, "balanced")
		if _, err := NewService(repo).SafeRoutes(context.Background(), validQuery()); err == nil {
			t.Fatal("want error for missing profile seed")
		}
	})
}

func TestSafeRoutesLeastSafeOmittedWhenNoCandidates(t *testing.T) {
	repo := workingStub()
	repo.candidates = nil
	response, err := NewService(repo).SafeRoutes(context.Background(), validQuery())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(response.Routes) != 3 {
		t.Fatalf("routes = %d, want 3 (least_safe omitted)", len(response.Routes))
	}
}

// TestTimeBucketBoundaries pins the exact boundaries shared with the Python
// pipeline (etl/risk_network_kde/utils.py) — change both or neither.
func TestTimeBucketBoundaries(t *testing.T) {
	cases := map[int]string{
		0: "night", 5: "night", 6: "morning", 11: "morning",
		12: "afternoon", 17: "afternoon", 18: "evening", 21: "evening",
		22: "night", 23: "night",
	}
	for hour, want := range cases {
		if got := hourToTimeBucket(hour); got != want {
			t.Errorf("hourToTimeBucket(%d) = %q, want %q", hour, got, want)
		}
	}
}

func TestWeekdayTypeBoundaries(t *testing.T) {
	loc := time.FixedZone("-03", -3*3600)
	cases := []struct {
		day  time.Time
		want string
	}{
		{time.Date(2026, 6, 12, 12, 0, 0, 0, loc), "weekday"}, // Friday
		{time.Date(2026, 6, 13, 12, 0, 0, 0, loc), "weekend"}, // Saturday
		{time.Date(2026, 6, 14, 12, 0, 0, 0, loc), "weekend"}, // Sunday
		{time.Date(2026, 6, 15, 12, 0, 0, 0, loc), "weekday"}, // Monday
	}
	for _, c := range cases {
		if got := dateToWeekdayType(c.day); got != c.want {
			t.Errorf("dateToWeekdayType(%v) = %q, want %q", c.day, got, c.want)
		}
	}
}
