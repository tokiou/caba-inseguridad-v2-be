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

func (r *fakeRepository) FindNearby(ctx context.Context, query NearbyCrimesQuery) ([]Crime, error) {
	r.receivedQuery = query

	if r.err != nil {
		return nil, r.err
	}

	return r.items, nil
}

func TestServiceGetNearbyReturnsErrorForInvalidCoordinates(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)

	query := NearbyCrimesQuery{
		Lat:          0,
		Lng:          0,
		RadiusMeters: 300,
	}

	_, err := service.GetNearby(context.Background(), query)

	if !errors.Is(err, ErrInvalidCoordinates) {
		t.Fatalf("expected ErrInvalidCoordinates, got %v", err)
	}
}

func TestServiceGetNearbyReturnsErrorForInvalidRadius(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)

	query := NearbyCrimesQuery{
		Lat:          -34.5895,
		Lng:          -58.4201,
		RadiusMeters: 3000,
	}

	_, err := service.GetNearby(context.Background(), query)

	if !errors.Is(err, ErrInvalidRadius) {
		t.Fatalf("expected ErrInvalidRadius, got %v", err)
	}
}

func TestServiceGetNearbyAppliesDefaultRadius(t *testing.T) {
	repository := &fakeRepository{
		items: []Crime{},
	}
	service := NewService(repository)

	query := NearbyCrimesQuery{
		Lat: -34.5895,
		Lng: -58.4201,
	}

	response, err := service.GetNearby(context.Background(), query)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if response.RadiusMeters != DefaultRadiusMeters {
		t.Fatalf("expected radius %d, got %d", DefaultRadiusMeters, response.RadiusMeters)
	}

	if repository.receivedQuery.RadiusMeters != DefaultRadiusMeters {
		t.Fatalf("expected repository radius %d, got %d", DefaultRadiusMeters, repository.receivedQuery.RadiusMeters)
	}
}

func TestServiceGetNearbyReturnsResponseWithCount(t *testing.T) {
	repository := &fakeRepository{
		items: []Crime{
			{SourceID: "1", CrimeType: "ROBO", Quantity: 1},
			{SourceID: "2", CrimeType: "HURTO", Quantity: 1},
		},
	}
	service := NewService(repository)

	query := NearbyCrimesQuery{
		Lat:          -34.5895,
		Lng:          -58.4201,
		RadiusMeters: 300,
	}

	response, err := service.GetNearby(context.Background(), query)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if response.Count != 2 {
		t.Fatalf("expected count 2, got %d", response.Count)
	}
}
