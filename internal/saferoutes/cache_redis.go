package saferoutes

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRouteCache stores SafeRoutesResponse JSON in Redis. It is fail-open: any
// Redis or (de)serialization error is logged and reported as a miss / no-op, so a
// cache problem never fails a request. Startup already pinged Redis, so a
// request-time error means a transient blip rather than misconfiguration.
//
// It also counts hits/misses/errors/sets (atomic) for the stats endpoint.
type RedisRouteCache struct {
	client *redis.Client
	log    *slog.Logger

	hits   atomic.Int64
	misses atomic.Int64
	errors atomic.Int64
	sets   atomic.Int64
}

// NewRedisRouteCache builds the Redis-backed route cache.
func NewRedisRouteCache(client *redis.Client, log *slog.Logger) *RedisRouteCache {
	return &RedisRouteCache{client: client, log: log}
}

func (c *RedisRouteCache) Get(ctx context.Context, key string) (*SafeRoutesResponse, bool, error) {
	raw, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		c.misses.Add(1)
		return nil, false, nil // miss
	}
	if err != nil {
		c.errors.Add(1)
		c.misses.Add(1)
		c.log.Warn("route cache get failed", "err", err)
		return nil, false, nil // fail-open: treat as a miss
	}

	var resp SafeRoutesResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		c.errors.Add(1)
		c.misses.Add(1)
		c.log.Warn("route cache decode failed", "err", err)
		return nil, false, nil
	}
	c.hits.Add(1)
	return &resp, true, nil
}

func (c *RedisRouteCache) Set(ctx context.Context, key string, value SafeRoutesResponse, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		c.errors.Add(1)
		c.log.Warn("route cache encode failed", "err", err)
		return nil
	}
	if err := c.client.Set(ctx, key, raw, ttl).Err(); err != nil {
		c.errors.Add(1)
		c.log.Warn("route cache set failed", "err", err)
		return nil
	}
	c.sets.Add(1)
	return nil
}

// Stats returns a snapshot of the cache counters (implements CacheStatsProvider).
func (c *RedisRouteCache) Stats() CacheStats {
	hits, misses := c.hits.Load(), c.misses.Load()
	var hitRate float64
	if total := hits + misses; total > 0 {
		hitRate = float64(hits) / float64(total)
	}
	return CacheStats{
		Hits:    hits,
		Misses:  misses,
		Errors:  c.errors.Load(),
		Sets:    c.sets.Load(),
		HitRate: hitRate,
	}
}
