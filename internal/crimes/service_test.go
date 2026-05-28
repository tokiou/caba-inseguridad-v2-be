package crimes

import (
	"context"
	"errors"
	"testing"
)

type fakeRepository struct {
	items         []Crime
	err           error
	receivedQuery NearbyCrimesQuery
}

func (r *fakeRepository) FindNearby(_ context.Context, query NearbyCrimesQuery) ([]Crime, error) {
	r.receivedQuery = query
	return r.items, r.err
}

func TestServiceGetNearby(t *testing.T) {
	validQuery := NearbyCrimesQuery{Lat: -34.5895, Lng: -58.4201, RadiusMeters: 300}

	tests := []struct {
		name       string
		repo       *fakeRepository
		query      NearbyCrimesQuery
		wantErr    error
		wantCount  int
		wantRadius int
	}{
		{
			name:    "error for coordinates outside CABA",
			repo:    &fakeRepository{},
			query:   NearbyCrimesQuery{Lat: 0, Lng: 0, RadiusMeters: 300},
			wantErr: ErrInvalidCoordinates,
		},
		{
			name:    "error for radius above max",
			repo:    &fakeRepository{},
			query:   NearbyCrimesQuery{Lat: -34.5895, Lng: -58.4201, RadiusMeters: MaxRadiusMeters + 1},
			wantErr: ErrInvalidRadius,
		},
		{
			name:       "applies default radius when zero",
			repo:       &fakeRepository{items: []Crime{}},
			query:      NearbyCrimesQuery{Lat: -34.5895, Lng: -58.4201},
			wantRadius: DefaultRadiusMeters,
		},
		{
			name:  "count matches repository results",
			repo:  &fakeRepository{items: []Crime{{SourceID: "1"}, {SourceID: "2"}}},
			query: validQuery,
			wantCount: 2,
		},
		{
			name:  "propagates repository error",
			repo:  &fakeRepository{err: errors.New("db unavailable")},
			query: validQuery,
			wantErr: errors.New("db unavailable"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(tt.repo)

			resp, err := svc.GetNearby(context.Background(), tt.query)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				if tt.wantErr == ErrInvalidCoordinates || tt.wantErr == ErrInvalidRadius {
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("want error %v, got %v", tt.wantErr, err)
					}
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantRadius != 0 && resp.RadiusMeters != tt.wantRadius {
				t.Errorf("want radius %d, got %d", tt.wantRadius, resp.RadiusMeters)
			}
			if tt.wantRadius != 0 && tt.repo.receivedQuery.RadiusMeters != tt.wantRadius {
				t.Errorf("want repository radius %d, got %d", tt.wantRadius, tt.repo.receivedQuery.RadiusMeters)
			}
			if tt.wantCount != 0 && resp.Count != tt.wantCount {
				t.Errorf("want count %d, got %d", tt.wantCount, resp.Count)
			}
		})
	}
}
