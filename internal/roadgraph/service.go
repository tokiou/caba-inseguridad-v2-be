package roadgraph

import "context"

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) GetStats(ctx context.Context) (GraphStats, error) {
	return s.repository.GetStats(ctx)
}

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

func isValidCABACoordinates(lat float64, lng float64) bool {
	return lat >= -35 && lat <= -34 && lng >= -59 && lng <= -58
}
