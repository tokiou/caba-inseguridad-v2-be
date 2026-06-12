package saferoutes

import "context"

// Repository is the routing-engine seam: everything the service needs from the
// datastore (active model, profiles, snapping, risk-weighted paths). A future
// non-pgRouting engine implements this same interface.
type Repository interface {
	// ActiveModel returns the single active risk model version, or
	// ErrNoActiveModel.
	ActiveModel(ctx context.Context) (ModelVersion, error)

	// RouteProfiles returns route_profiles keyed by name.
	RouteProfiles(ctx context.Context) (map[string]RouteProfile, error)

	// SnapEndpoints resolves origin/destination to routable graph nodes; the
	// caller validates the snap distances.
	SnapEndpoints(ctx context.Context, query SafeRoutesQuery) (SnapResult, error)

	// FindRoute returns the minimal-cost path for the request, with per-edge
	// risk and components for the request's context, or ErrNoRoute.
	FindRoute(ctx context.Context, req RouteRequest) (RoutePath, error)

	// FindCandidateRoutes returns up to K distance-ranked candidate paths
	// (least-safe-candidate pool). An empty slice is a valid result.
	FindCandidateRoutes(ctx context.Context, req CandidateRouteRequest) ([]RoutePath, error)
}
