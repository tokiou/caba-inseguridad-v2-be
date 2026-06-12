package saferoutes

import "errors"

var (
	// ErrInvalidCoordinates marks origin/destination input that is missing,
	// unparseable, outside CABA, or where origin equals destination.
	ErrInvalidCoordinates = errors.New("invalid coordinates")

	// ErrPointOutsideGraph marks an origin or destination whose nearest routable
	// edge is farther than the snapping limit (the point is not walkable from).
	ErrPointOutsideGraph = errors.New("origin or destination outside walkable graph")

	// ErrNoRoute marks endpoints that snap fine but are not connected on the
	// routable graph.
	ErrNoRoute = errors.New("no route between points")

	// ErrNoActiveModel marks the absence of an active risk model version —
	// risk-aware routing cannot run.
	ErrNoActiveModel = errors.New("no active risk model")
)
