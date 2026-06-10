package roadgraph

import "context"

// Repository reads road-graph metadata. PostGIS access lives behind this
// interface (implemented by PostgresRepository); the service depends only on it.
type Repository interface {
	GetStats(ctx context.Context) (GraphStats, error)
}
