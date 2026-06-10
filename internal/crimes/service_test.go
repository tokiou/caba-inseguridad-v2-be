package crimes

import (
	"context"
	"errors"
	"testing"
)

type fakeRepository struct {
	page          CrimePage
	err           error
	receivedQuery NearbyCrimesQuery
}

func (r *fakeRepository) FindNearby(_ context.Context, query NearbyCrimesQuery) (CrimePage, error) {
	r.receivedQuery = query
	return r.page, r.err
}

func TestServiceGetNearby(t *testing.T) {
	validQuery := NearbyCrimesQuery{Lat: -34.5895, Lng: -58.4201, RadiusMeters: 300, Limit: 100}

	t.Run("error for coordinates outside CABA", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		_, err := svc.GetNearby(context.Background(), NearbyCrimesQuery{Lat: 0, Lng: 0, RadiusMeters: 300})
		if !errors.Is(err, ErrInvalidCoordinates) {
			t.Fatalf("want ErrInvalidCoordinates, got %v", err)
		}
	})

	t.Run("error for radius above max", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		_, err := svc.GetNearby(context.Background(), NearbyCrimesQuery{Lat: -34.5895, Lng: -58.4201, RadiusMeters: MaxRadiusMeters + 1})
		if !errors.Is(err, ErrInvalidRadius) {
			t.Fatalf("want ErrInvalidRadius, got %v", err)
		}
	})

	t.Run("error for limit above max", func(t *testing.T) {
		svc := NewService(&fakeRepository{})
		_, err := svc.GetNearby(context.Background(), NearbyCrimesQuery{Lat: -34.5895, Lng: -58.4201, Limit: MaxLimit + 1})
		if !errors.Is(err, ErrInvalidLimit) {
			t.Fatalf("want ErrInvalidLimit, got %v", err)
		}
	})

	t.Run("applies default radius and limit when zero", func(t *testing.T) {
		repo := &fakeRepository{page: CrimePage{Items: []Crime{}}}
		svc := NewService(repo)
		resp, err := svc.GetNearby(context.Background(), NearbyCrimesQuery{Lat: -34.5895, Lng: -58.4201})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.RadiusMeters != DefaultRadiusMeters {
			t.Errorf("want radius %d, got %d", DefaultRadiusMeters, resp.RadiusMeters)
		}
		if repo.receivedQuery.RadiusMeters != DefaultRadiusMeters {
			t.Errorf("want repo radius %d, got %d", DefaultRadiusMeters, repo.receivedQuery.RadiusMeters)
		}
		if repo.receivedQuery.Limit != DefaultLimit {
			t.Errorf("want repo limit %d, got %d", DefaultLimit, repo.receivedQuery.Limit)
		}
	})

	t.Run("count matches page items and no cursor on last page", func(t *testing.T) {
		repo := &fakeRepository{page: CrimePage{Items: []Crime{{SourceID: "1"}, {SourceID: "2"}}}}
		svc := NewService(repo)
		resp, err := svc.GetNearby(context.Background(), validQuery)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Count != 2 {
			t.Errorf("want count 2, got %d", resp.Count)
		}
		if resp.HasMore {
			t.Error("want has_more false")
		}
		if resp.NextCursor != nil {
			t.Errorf("want nil next_cursor, got %v", *resp.NextCursor)
		}
	})

	t.Run("surfaces has_more and encoded next_cursor", func(t *testing.T) {
		next := &Cursor{Distance: 84.2, ID: 123}
		repo := &fakeRepository{page: CrimePage{Items: []Crime{{SourceID: "1"}}, HasMore: true, Next: next}}
		svc := NewService(repo)
		resp, err := svc.GetNearby(context.Background(), validQuery)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.HasMore {
			t.Error("want has_more true")
		}
		if resp.NextCursor == nil {
			t.Fatal("want non-nil next_cursor")
		}
		if *resp.NextCursor != next.Encode() {
			t.Errorf("want cursor %q, got %q", next.Encode(), *resp.NextCursor)
		}
	})

	t.Run("passes cursor through to repository", func(t *testing.T) {
		cursor := &Cursor{Distance: 10, ID: 5}
		repo := &fakeRepository{page: CrimePage{Items: []Crime{}}}
		svc := NewService(repo)
		q := validQuery
		q.Cursor = cursor
		if _, err := svc.GetNearby(context.Background(), q); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo.receivedQuery.Cursor != cursor {
			t.Errorf("want cursor passed through, got %v", repo.receivedQuery.Cursor)
		}
	})

	t.Run("propagates repository error", func(t *testing.T) {
		svc := NewService(&fakeRepository{err: errors.New("db unavailable")})
		if _, err := svc.GetNearby(context.Background(), validQuery); err == nil {
			t.Fatal("want error, got nil")
		}
	})
}
