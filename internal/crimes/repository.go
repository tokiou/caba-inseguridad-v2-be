package crimes

import (
	"context"
)

type Repository interface {
	FindNearby(ctx context.Context, query NearbyCrimesQuery) ([]Crime, error)
}
