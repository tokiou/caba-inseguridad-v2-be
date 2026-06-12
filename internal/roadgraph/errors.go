package roadgraph

import "errors"

var (
	// ErrInvalidCoordinates marks origin/destination input that is missing,
	// unparseable, outside CABA, or where origin equals destination.
	ErrInvalidCoordinates = errors.New("invalid coordinates")

	// ErrNoRoute marks a request whose snapped endpoints are not connected on the
	// routable graph (no walkable path exists between them).
	ErrNoRoute = errors.New("no route between points")
)
