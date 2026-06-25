// Package redisplatform builds the shared Redis client used by distributed rate
// limiting and the route cache. It mirrors internal/platform/postgres: construct
// the client and verify connectivity with a ping before returning it.
package redisplatform

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewClient creates a go-redis v9 client and verifies connectivity with a ping
// before returning it. A failed ping is returned as an error so the caller can
// fail fast at startup rather than discover Redis is down on the first request.
func NewClient(ctx context.Context, addr, password string, db int) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}

	return client, nil
}
