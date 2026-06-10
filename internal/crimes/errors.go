package crimes

import "errors"

var (
	ErrInvalidCoordinates = errors.New("invalid coordinates")
	ErrInvalidRadius      = errors.New("invalid radius")
	ErrInvalidLimit       = errors.New("invalid limit")
	ErrInvalidCursor      = errors.New("invalid cursor")
)
