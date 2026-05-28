package crimes

import (
	"context"
)

const (
	DefaultRadiusMeters = 300
	MaxRadiusMeters     = 2000
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{
		repository: repository,
	}
}

func (s *Service) GetNearby(ctx context.Context, query NearbyCrimesQuery) (NearbyCrimesResponse, error) {
	if query.RadiusMeters == 0 {
		query.RadiusMeters = DefaultRadiusMeters
	}

	if !isValidCABACoordinates(query.Lat, query.Lng) {
		return NearbyCrimesResponse{}, ErrInvalidCoordinates
	}

	if !isValidRadius(query.RadiusMeters) {
		return NearbyCrimesResponse{}, ErrInvalidRadius
	}

	items, err := s.repository.FindNearby(ctx, query)
	if err != nil {
		return NearbyCrimesResponse{}, err
	}

	return NearbyCrimesResponse{
		Lat:          query.Lat,
		Lng:          query.Lng,
		RadiusMeters: query.RadiusMeters,
		Count:        len(items),
		Items:        items,
	}, nil
}

func isValidCABACoordinates(lat float64, lng float64) bool {
	return lat >= -35 && lat <= -34 && lng >= -59 && lng <= -58
}

func isValidRadius(radius int) bool {
	return radius >= 1 && radius <= MaxRadiusMeters
}
