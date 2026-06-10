package crimes

import (
	"context"
)

const (
	DefaultRadiusMeters = 300
	MaxRadiusMeters     = 2000
	DefaultLimit        = 100
	MaxLimit            = 500
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
	if query.Limit == 0 {
		query.Limit = DefaultLimit
	}

	if !isValidCABACoordinates(query.Lat, query.Lng) {
		return NearbyCrimesResponse{}, ErrInvalidCoordinates
	}
	if !isValidRadius(query.RadiusMeters) {
		return NearbyCrimesResponse{}, ErrInvalidRadius
	}
	if !isValidLimit(query.Limit) {
		return NearbyCrimesResponse{}, ErrInvalidLimit
	}

	page, err := s.repository.FindNearby(ctx, query)
	if err != nil {
		return NearbyCrimesResponse{}, err
	}

	response := NearbyCrimesResponse{
		Lat:          query.Lat,
		Lng:          query.Lng,
		RadiusMeters: query.RadiusMeters,
		Count:        len(page.Items),
		Items:        page.Items,
		HasMore:      page.HasMore,
	}
	if page.Next != nil {
		token := page.Next.Encode()
		response.NextCursor = &token
	}

	return response, nil
}

func isValidCABACoordinates(lat float64, lng float64) bool {
	return lat >= -35 && lat <= -34 && lng >= -59 && lng <= -58
}

func isValidRadius(radius int) bool {
	return radius >= 1 && radius <= MaxRadiusMeters
}

func isValidLimit(limit int) bool {
	return limit >= 1 && limit <= MaxLimit
}
