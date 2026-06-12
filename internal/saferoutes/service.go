package saferoutes

import (
	"context"
	"fmt"
	"time"
)

const (
	// maxSnapToGraphMeters bounds how far an endpoint may be from the graph.
	maxSnapToGraphMeters = 150.0

	// least-safe candidates: pool size and detour bound relative to fastest.
	leastSafeCandidateK           = 10
	leastSafeCandidateDetourRatio = 1.75
)

// Service orchestrates the safe-routes flow: validate, resolve the risk
// context, snap, route per profile, aggregate.
type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) SafeRoutes(ctx context.Context, query SafeRoutesQuery) (SafeRoutesResponse, error) {
	if !isValidCABACoordinates(query.OriginLat, query.OriginLng) ||
		!isValidCABACoordinates(query.DestLat, query.DestLng) {
		return SafeRoutesResponse{}, ErrInvalidCoordinates
	}
	if query.OriginLat == query.DestLat && query.OriginLng == query.DestLng {
		return SafeRoutesResponse{}, ErrInvalidCoordinates
	}

	at := query.At
	if at.IsZero() {
		at = time.Now().In(buenosAires)
	}
	timeBucket := hourToTimeBucket(at.Hour())
	weekdayType := dateToWeekdayType(at)

	model, err := s.repository.ActiveModel(ctx)
	if err != nil {
		return SafeRoutesResponse{}, err
	}

	profiles, err := s.repository.RouteProfiles(ctx)
	if err != nil {
		return SafeRoutesResponse{}, err
	}
	for _, name := range []string{"fastest", "balanced", "safest"} {
		if _, ok := profiles[name]; !ok {
			return SafeRoutesResponse{}, fmt.Errorf("saferoutes: profile %q not seeded", name)
		}
	}

	snap, err := s.repository.SnapEndpoints(ctx, query)
	if err != nil {
		return SafeRoutesResponse{}, err
	}
	if snap.OriginDistance > maxSnapToGraphMeters || snap.DestDistance > maxSnapToGraphMeters {
		return SafeRoutesResponse{}, ErrPointOutsideGraph
	}

	baseRequest := RouteRequest{
		FromNodeID:     snap.OriginNodeID,
		ToNodeID:       snap.DestNodeID,
		ModelVersionID: model.ID,
		TimeBucket:     timeBucket,
		WeekdayType:    weekdayType,
	}

	routes := make([]SafeRoute, 0, 4)
	var fastest SafeRoute
	for _, name := range []string{"fastest", "balanced", "safest"} {
		request := baseRequest
		request.SafetyMultiplier = profiles[name].SafetyMultiplier
		path, err := s.repository.FindRoute(ctx, request)
		if err != nil {
			return SafeRoutesResponse{}, err
		}
		route := aggregateRoute(name, path, model)
		if name == "fastest" {
			fastest = route
		}
		setComparisons(&route, fastest)
		routes = append(routes, route)
	}

	if candidate, ok, err := s.leastSafeCandidate(ctx, baseRequest, fastest, model); err != nil {
		return SafeRoutesResponse{}, err
	} else if ok {
		routes = append(routes, candidate)
	}

	return SafeRoutesResponse{
		Origin:      LatLng{Lat: query.OriginLat, Lng: query.OriginLng},
		Destination: LatLng{Lat: query.DestLat, Lng: query.DestLng},
		Datetime:    at.Format(time.RFC3339),
		TimeBucket:  timeBucket,
		WeekdayType: weekdayType,
		ModelVersion: ModelVersionInfo{
			ID: model.ID, Name: model.Name, Type: model.Type, TrainUntil: model.TrainUntil,
		},
		Routes: routes,
	}, nil
}

// leastSafeCandidate picks the riskiest of K distance-ranked candidate paths
// within the detour bound — a debugging/contrast route, never an optimization
// target. ok=false when KSP yields no usable candidate (degraded, documented).
func (s *Service) leastSafeCandidate(
	ctx context.Context, base RouteRequest, fastest SafeRoute, model ModelVersion,
) (SafeRoute, bool, error) {
	paths, err := s.repository.FindCandidateRoutes(ctx, CandidateRouteRequest{
		FromNodeID:     base.FromNodeID,
		ToNodeID:       base.ToNodeID,
		ModelVersionID: base.ModelVersionID,
		TimeBucket:     base.TimeBucket,
		WeekdayType:    base.WeekdayType,
		K:              leastSafeCandidateK,
	})
	if err != nil {
		return SafeRoute{}, false, err
	}

	var (
		worst SafeRoute
		found bool
	)
	for _, path := range paths {
		route := aggregateRoute("least_safe_candidate", path, model)
		if fastest.DistanceMeters > 0 &&
			route.DistanceMeters > fastest.DistanceMeters*leastSafeCandidateDetourRatio {
			continue
		}
		if !found || route.RiskScore > worst.RiskScore {
			worst = route
			found = true
		}
	}
	if !found {
		return SafeRoute{}, false, nil
	}
	setComparisons(&worst, fastest)
	return worst, true, nil
}

// isValidCABACoordinates bounds inputs to the CABA bounding box (matches the
// crimes and roadgraph domains).
func isValidCABACoordinates(lat float64, lng float64) bool {
	return lat >= -35 && lat <= -34 && lng >= -59 && lng <= -58
}

// buenosAires resolves the default request time. Falls back to a fixed UTC-3
// (Argentina has no DST) if the tz database is unavailable.
var buenosAires = func() *time.Location {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		return time.FixedZone("-03", -3*60*60)
	}
	return loc
}()

// hourToTimeBucket and dateToWeekdayType mirror etl/risk_network_kde/utils.py
// exactly — change both or neither.
func hourToTimeBucket(hour int) string {
	switch {
	case hour >= 6 && hour <= 11:
		return "morning"
	case hour >= 12 && hour <= 17:
		return "afternoon"
	case hour >= 18 && hour <= 21:
		return "evening"
	default:
		return "night"
	}
}

func dateToWeekdayType(t time.Time) string {
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return "weekend"
	}
	return "weekday"
}
