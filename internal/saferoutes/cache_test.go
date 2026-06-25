package saferoutes

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNoopRouteCache_AlwaysMisses(t *testing.T) {
	var c NoopRouteCache
	if err := c.Set(context.Background(), "k", SafeRoutesResponse{TimeBucket: "night"}, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, ok, err := c.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if ok || got != nil {
		t.Fatalf("noop Get returned a hit (%v, %v), want miss", got, ok)
	}
}

func TestRedisRouteCache_RoundTrip(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	cache := NewRedisRouteCache(redis.NewClient(&redis.Options{Addr: mr.Addr()}), discardLogger())

	if _, ok, err := cache.Get(context.Background(), "route:x"); err != nil || ok {
		t.Fatalf("pre-set Get = (ok %v, err %v), want miss", ok, err)
	}

	want := SafeRoutesResponse{
		TimeBucket:   "night",
		WeekdayType:  "weekday",
		ModelVersion: ModelVersionInfo{ID: 7, Name: "m"},
		Routes:       []SafeRoute{{Kind: "fastest", DistanceMeters: 1000}},
	}
	if err := cache.Set(context.Background(), "route:x", want, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, ok, err := cache.Get(context.Background(), "route:x")
	if err != nil || !ok {
		t.Fatalf("post-set Get = (ok %v, err %v), want hit", ok, err)
	}
	if got.TimeBucket != want.TimeBucket || got.ModelVersion.ID != 7 || len(got.Routes) != 1 {
		t.Fatalf("round-trip mismatch: got %+v", got)
	}
}

func TestRedisRouteCache_StatsCounters(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer mr.Close()
	c := NewRedisRouteCache(redis.NewClient(&redis.Options{Addr: mr.Addr()}), discardLogger())

	c.Get(context.Background(), "a") // miss
	c.Get(context.Background(), "b") // miss
	if err := c.Set(context.Background(), "a", SafeRoutesResponse{TimeBucket: "x"}, time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	c.Get(context.Background(), "a") // hit

	s := c.Stats()
	if s.Hits != 1 || s.Misses != 2 || s.Sets != 1 || s.Errors != 0 {
		t.Fatalf("stats = %+v, want hits=1 misses=2 sets=1 errors=0", s)
	}
	if s.HitRate < 0.33 || s.HitRate > 0.34 { // 1/(1+2)
		t.Fatalf("hit_rate = %v, want ~0.333", s.HitRate)
	}
}

func TestRouteCacheKey_Deterministic(t *testing.T) {
	q := SafeRoutesQuery{OriginLat: -34.58, OriginLng: -58.42, DestLat: -34.60, DestLng: -58.38}
	a := routeCacheKey(q, "night", "weekday", 4)
	if b := routeCacheKey(q, "night", "weekday", 4); a != b {
		t.Fatalf("key not deterministic: %q vs %q", a, b)
	}
	if c := routeCacheKey(q, "night", "weekday", 5); c == a {
		t.Fatal("model id is not part of the key")
	}
	if c := routeCacheKey(q, "morning", "weekday", 4); c == a {
		t.Fatal("time bucket is not part of the key")
	}
}

// fakeCache records calls and can be primed with a hit.
type fakeCache struct {
	hit      *SafeRoutesResponse
	getCalls int
	setCalls int
	setKey   string
	setVal   SafeRoutesResponse
}

func (f *fakeCache) Get(context.Context, string) (*SafeRoutesResponse, bool, error) {
	f.getCalls++
	if f.hit != nil {
		return f.hit, true, nil
	}
	return nil, false, nil
}

func (f *fakeCache) Set(_ context.Context, key string, v SafeRoutesResponse, _ time.Duration) error {
	f.setCalls++
	f.setKey = key
	f.setVal = v
	return nil
}

func TestSafeRoutes_CacheHitSkipsRouting(t *testing.T) {
	repo := workingStub()
	cache := &fakeCache{hit: &SafeRoutesResponse{TimeBucket: "cached", Routes: []SafeRoute{{Kind: "fastest"}}}}

	resp, err := NewService(repo, cache).SafeRoutes(context.Background(), validQuery())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TimeBucket != "cached" {
		t.Fatalf("expected the cached response, got bucket %q", resp.TimeBucket)
	}
	if len(repo.routeRequests) != 0 {
		t.Fatalf("routing repo queried on a cache hit: %d FindRoute calls", len(repo.routeRequests))
	}
	if cache.setCalls != 0 {
		t.Fatalf("Set called on a cache hit: %d", cache.setCalls)
	}
}

func TestSafeRoutes_CacheMissComputesAndStores(t *testing.T) {
	repo := workingStub()
	cache := &fakeCache{} // always miss

	resp, err := NewService(repo, cache).SafeRoutes(context.Background(), validQuery())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cache.getCalls != 1 {
		t.Fatalf("Get calls = %d, want 1", cache.getCalls)
	}
	if cache.setCalls != 1 {
		t.Fatalf("Set calls = %d, want 1 (store on miss)", cache.setCalls)
	}
	if len(repo.routeRequests) != 3 {
		t.Fatalf("FindRoute calls = %d, want 3 (computed on miss)", len(repo.routeRequests))
	}
	if cache.setVal.TimeBucket != resp.TimeBucket || len(cache.setVal.Routes) != len(resp.Routes) {
		t.Fatal("stored value differs from the returned response")
	}
	// Key uses the resolved context + the stub's active model id (4).
	if want := routeCacheKey(validQuery(), "night", "weekday", 4); cache.setKey != want {
		t.Fatalf("set key = %q, want %q", cache.setKey, want)
	}
}
