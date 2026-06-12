package roadgraph

import "context"

// Repository reads road-graph metadata and computes paths over the graph. PostGIS
// and pgRouting access live behind this interface (implemented by
// PostgresRepository); the service depends only on it.
type Repository interface {
	GetStats(ctx context.Context) (GraphStats, error)
	// FindWalkRoute returns the shortest walkable path between the query's
	// endpoints, or ErrNoRoute if the snapped nodes are not connected.
	FindWalkRoute(ctx context.Context, query WalkRouteQuery) (WalkRoute, error)
}
