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
// context, snap, route per profile, aggregate. A RouteCache short-circuits the
// expensive routing work for identical requests (NoopRouteCache when disabled).
type Service struct {
	repository Repository
	cache      RouteCache
}

func NewService(repository Repository, cache RouteCache) *Service {
	return &Service{repository: repository, cache: cache}
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

	// Cache check happens after the (cheap) model lookup and before the expensive
	// snap + routing + aggregation. The key includes the model id, so a model
	// change naturally bypasses stale entries. A Get error is a miss (fail-open).
	cacheKey := routeCacheKey(query, timeBucket, weekdayType, model.ID)
	if cached, ok, getErr := s.cache.Get(ctx, cacheKey); getErr == nil && ok {
		cached.FromCache = true // surfaced as X-Cache: hit (never serialized/stored)
		return *cached, nil
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

		tod, err := s.timeOfDayRisk(ctx, path, model, weekdayType)
		if err != nil {
			return SafeRoutesResponse{}, err
		}
		route.TimeOfDayRisk = tod

		routes = append(routes, route)
	}

	if candidate, ok, err := s.leastSafeCandidate(ctx, baseRequest, fastest, model); err != nil {
		return SafeRoutesResponse{}, err
	} else if ok {
		routes = append(routes, candidate)
	}

	response := SafeRoutesResponse{
		Origin:      LatLng{Lat: query.OriginLat, Lng: query.OriginLng},
		Destination: LatLng{Lat: query.DestLat, Lng: query.DestLng},
		Datetime:    at.Format(time.RFC3339),
		TimeBucket:  timeBucket,
		WeekdayType: weekdayType,
		ModelVersion: ModelVersionInfo{
			ID: model.ID, Name: model.Name, Type: model.Type, TrainUntil: model.TrainUntil,
		},
		Routes: routes,
	}

	// Best-effort store; a Set failure is logged inside the cache, never surfaced.
	_ = s.cache.Set(ctx, cacheKey, response, routeCacheTTL)
	return response, nil
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
		worst     SafeRoute
		worstPath RoutePath
		found     bool
	)
	for _, path := range paths {
		route := aggregateRoute("least_safe_candidate", path, model)
		if fastest.DistanceMeters > 0 &&
			route.DistanceMeters > fastest.DistanceMeters*leastSafeCandidateDetourRatio {
			continue
		}
		if !found || route.RiskScore > worst.RiskScore {
			worst = route
			worstPath = path
			found = true
		}
	}
	if !found {
		return SafeRoute{}, false, nil
	}

	tod, err := s.timeOfDayRisk(ctx, worstPath, model, base.WeekdayType)
	if err != nil {
		return SafeRoute{}, false, err
	}
	worst.TimeOfDayRisk = tod

	setComparisons(&worst, fastest)
	return worst, true, nil
}

// timeOfDayRisk reads the path's per-edge risk across the four time buckets for
// the resolved weekday type and folds it into a per-route TimeOfDayRisk. Returns
// nil for an empty path.
func (s *Service) timeOfDayRisk(
	ctx context.Context, path RoutePath, model ModelVersion, weekdayType string,
) (*TimeOfDayRisk, error) {
	if len(path.Edges) == 0 {
		return nil, nil
	}
	edgeIDs := make([]int64, len(path.Edges))
	for i, e := range path.Edges {
		edgeIDs[i] = e.EdgeID
	}
	perBucket, err := s.repository.RouteRiskByBucket(ctx, edgeIDs, model.ID, weekdayType)
	if err != nil {
		return nil, err
	}
	return aggregateBucketRisk(path.Edges, perBucket, model), nil
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
