package app

import (
	"testing"

	"github.com/tokiou/caba-inseguridad-routes-go/internal/config"
)

func TestValidateRedisConfig(t *testing.T) {
	cases := []struct {
		name    string
		cfg     config.Config
		wantErr bool
	}{
		{"all off (baseline)", config.Config{}, false},
		{"redis only", config.Config{RedisEnabled: true}, false},
		{"rate limit without redis", config.Config{RateLimitEnabled: true}, true},
		{"route cache without redis", config.Config{RouteCacheEnabled: true}, true},
		{"rate limit with redis", config.Config{RedisEnabled: true, RateLimitEnabled: true}, false},
		{"route cache with redis", config.Config{RedisEnabled: true, RouteCacheEnabled: true}, false},
		{"both with redis", config.Config{RedisEnabled: true, RateLimitEnabled: true, RouteCacheEnabled: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := validateRedisConfig(c.cfg); (err != nil) != c.wantErr {
				t.Fatalf("validateRedisConfig() err = %v, wantErr %v", err, c.wantErr)
			}
		})
	}
}
