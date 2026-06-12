package roadgraph

import "context"

// Service holds the road-graph business logic. For this milestone it only
// surfaces graph status; routing/scoring are future capabilities.
type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) GetStats(ctx context.Context) (GraphStats, error) {
	return s.repository.GetStats(ctx)
}

// WalkRoute validates the request and returns the shortest walkable path between
// the two points over the local road graph. Endpoints must be within CABA and
// distinct; otherwise it returns ErrInvalidCoordinates.
func (s *Service) WalkRoute(ctx context.Context, query WalkRouteQuery) (WalkRoute, error) {
	if !isValidCABACoordinates(query.FromLat, query.FromLng) ||
		!isValidCABACoordinates(query.ToLat, query.ToLng) {
		return WalkRoute{}, ErrInvalidCoordinates
	}
	if query.FromLat == query.ToLat && query.FromLng == query.ToLng {
		return WalkRoute{}, ErrInvalidCoordinates
	}

	return s.repository.FindWalkRoute(ctx, query)
}

// isValidCABACoordinates bounds inputs to the CABA bounding box (matches the
// crimes domain's check).
func isValidCABACoordinates(lat float64, lng float64) bool {
	return lat >= -35 && lat <= -34 && lng >= -59 && lng <= -58
}
