package routes

import "context"

type routingClient interface {
	GetRoute(ctx context.Context, query RouteQuery) (Route, error)
}

const DefaultProfile = "driving-car"

var allowedProfiles = map[string]bool{
	"driving-car":     true,
	"foot-walking":    true,
	"cycling-regular": true,
}

type Service struct {
	client routingClient
}

func NewService(client routingClient) *Service {
	return &Service{client: client}
}

func (s *Service) GetRoute(ctx context.Context, query RouteQuery) (RouteResponse, error) {
	if query.Profile == "" {
		query.Profile = DefaultProfile
	}

	if !allowedProfiles[query.Profile] {
		return RouteResponse{}, ErrInvalidProfile
	}

	if !isValidCABACoordinates(query.OriginLat, query.OriginLng) {
		return RouteResponse{}, ErrInvalidCoordinates
	}

	if !isValidCABACoordinates(query.DestLat, query.DestLng) {
		return RouteResponse{}, ErrInvalidCoordinates
	}

	if query.OriginLat == query.DestLat && query.OriginLng == query.DestLng {
		return RouteResponse{}, ErrSamePoint
	}

	route, err := s.client.GetRoute(ctx, query)
	if err != nil {
		return RouteResponse{}, err
	}

	return RouteResponse{
		Origin:      Waypoint{Lat: query.OriginLat, Lng: query.OriginLng},
		Destination: Waypoint{Lat: query.DestLat, Lng: query.DestLng},
		Profile:     query.Profile,
		Distance:    route.Distance,
		Duration:    route.Duration,
		Geometry:    route.Geometry,
	}, nil
}

func isValidCABACoordinates(lat, lng float64) bool {
	return lat >= -35 && lat <= -34 && lng >= -59 && lng <= -58
}
