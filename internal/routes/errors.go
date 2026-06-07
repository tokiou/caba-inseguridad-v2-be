package routes

import "errors"

var (
	ErrInvalidCoordinates = errors.New("invalid coordinates")
	ErrSamePoint          = errors.New("origin and destination are the same point")
	ErrInvalidProfile     = errors.New("invalid transport profile")
	ErrRouteNotFound      = errors.New("no route found")
	ErrExternalService    = errors.New("external routing service error")
)
