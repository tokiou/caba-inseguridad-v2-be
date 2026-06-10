package roadgraph

import (
	"context"
	"errors"
	"testing"
)

type fakeRepository struct {
	stats GraphStats
	err   error
}

func (r *fakeRepository) GetStats(_ context.Context) (GraphStats, error) {
	return r.stats, r.err
}

func TestServiceGetStats(t *testing.T) {
	t.Run("returns repository stats", func(t *testing.T) {
		want := GraphStats{
			NodesCount: 71314, EdgesCount: 104309, WalkableEdges: 104309,
			RoutableEdges: 103570, ExcludedEdges: 739, RiskScoredEdges: 0,
			MinLat: -34.72, MinLng: -58.54, MaxLat: -34.52, MaxLng: -58.33,
		}
		svc := NewService(&fakeRepository{stats: want})

		got, err := svc.GetStats(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != want {
			t.Errorf("want %+v, got %+v", want, got)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		svc := NewService(&fakeRepository{err: errors.New("db unavailable")})
		if _, err := svc.GetStats(context.Background()); err == nil {
			t.Fatal("want error, got nil")
		}
	})
}
