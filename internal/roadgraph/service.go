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
