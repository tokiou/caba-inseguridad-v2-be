package redisplatform

import (
	"context"
	"testing"
)

// TestNewClient_UnreachableFails verifies the Redis-down → controlled failure
// contract: NewClient pings and returns an error (rather than a usable client)
// when nothing is listening, letting app.New fail fast at startup.
func TestNewClient_UnreachableFails(t *testing.T) {
	// Port 1 is privileged and never has a Redis listening on it.
	client, err := NewClient(context.Background(), "127.0.0.1:1", "", 0)
	if err == nil {
		if client != nil {
			_ = client.Close()
		}
		t.Fatal("expected an error connecting to an unreachable Redis, got nil")
	}
	if client != nil {
		t.Fatal("expected a nil client on failure")
	}
}
