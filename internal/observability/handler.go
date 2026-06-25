// Package observability exposes GET /api/v1/debug/stats: a JSON snapshot of
// runtime internals (pgxpool saturation, route-cache counters, process info) for
// the benchmark harness. It leaks internals, so it is registered only when
// METRICS_ENABLED=true and rejects non-loopback clients.
//
// It is a thin introspection handler (no service/repository layer, like health):
// it reads stats through plain-struct closures the wiring supplies, which keeps
// it decoupled from pgxpool and the saferoutes cache and makes it unit-testable.
package observability

import (
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/tokiou/caba-inseguridad-routes-go/internal/httpx"
)

// PoolStats is the pgxpool view surfaced by the endpoint.
type PoolStats struct {
	AcquiredConns        int32   `json:"acquired_conns"`
	IdleConns            int32   `json:"idle_conns"`
	TotalConns           int32   `json:"total_conns"`
	MaxConns             int32   `json:"max_conns"`
	AcquireCount         int64   `json:"acquire_count"`
	EmptyAcquireCount    int64   `json:"empty_acquire_count"`
	CanceledAcquireCount int64   `json:"canceled_acquire_count"`
	NewConnsCount        int64   `json:"new_conns_count"`
	AcquireDurationMS    float64 `json:"acquire_duration_ms"`
}

// CacheStats is the route-cache view surfaced by the endpoint.
type CacheStats struct {
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	Errors  int64   `json:"errors"`
	Sets    int64   `json:"sets"`
	HitRate float64 `json:"hit_rate"`
}

// PoolStatsFunc returns a current pgxpool snapshot.
type PoolStatsFunc func() PoolStats

// CacheStatsFunc returns a current cache snapshot. Nil when the cache is disabled.
type CacheStatsFunc func() CacheStats

type Handler struct {
	poolStats  PoolStatsFunc
	cacheStats CacheStatsFunc // nil when the route cache is disabled
	started    time.Time
	log        *slog.Logger
}

// NewHandler builds the stats handler. cacheStats may be nil (cache disabled).
func NewHandler(poolStats PoolStatsFunc, cacheStats CacheStatsFunc, started time.Time, log *slog.Logger) *Handler {
	return &Handler{poolStats: poolStats, cacheStats: cacheStats, started: started, log: log}
}

func (h *Handler) Register(r chi.Router) {
	r.Get("/debug/stats", h.stats)
}

type cacheBlock struct {
	Enabled bool    `json:"enabled"`
	Hits    int64   `json:"hits"`
	Misses  int64   `json:"misses"`
	Errors  int64   `json:"errors"`
	Sets    int64   `json:"sets"`
	HitRate float64 `json:"hit_rate"`
}

type snapshot struct {
	UptimeSeconds float64    `json:"uptime_seconds"`
	Goroutines    int        `json:"goroutines"`
	PgxPool       PoolStats  `json:"pgxpool"`
	Cache         cacheBlock `json:"cache"`
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		httpx.WriteError(w, http.StatusForbidden, "forbidden", "debug stats are loopback-only")
		return
	}

	snap := snapshot{
		UptimeSeconds: time.Since(h.started).Seconds(),
		Goroutines:    runtime.NumGoroutine(),
		PgxPool:       h.poolStats(),
	}
	if h.cacheStats != nil {
		cs := h.cacheStats()
		snap.Cache = cacheBlock{
			Enabled: true,
			Hits:    cs.Hits,
			Misses:  cs.Misses,
			Errors:  cs.Errors,
			Sets:    cs.Sets,
			HitRate: cs.HitRate,
		}
	}
	httpx.WriteJSON(w, http.StatusOK, snap)
}

// isLoopback reports whether the request originated from the local host.
func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
