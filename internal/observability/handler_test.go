package observability

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func testRouter(cache CacheStatsFunc) http.Handler {
	pool := func() PoolStats {
		return PoolStats{MaxConns: 10, IdleConns: 7, AcquiredConns: 3, AcquireCount: 100, EmptyAcquireCount: 4}
	}
	h := NewHandler(pool, cache, time.Now().Add(-time.Minute), slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) { h.Register(r) })
	return r
}

func get(t *testing.T, router http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/debug/stats", nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestStats_NonLoopbackForbidden(t *testing.T) {
	rec := get(t, testRouter(nil), "203.0.113.5:5555") // public IP
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestStats_LoopbackReturnsSnapshot(t *testing.T) {
	cache := func() CacheStats { return CacheStats{Hits: 8, Misses: 2, Sets: 2, HitRate: 0.8} }
	rec := get(t, testRouter(cache), "127.0.0.1:5555")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var snap snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.PgxPool.MaxConns != 10 || snap.PgxPool.AcquiredConns != 3 || snap.PgxPool.EmptyAcquireCount != 4 {
		t.Fatalf("pool stats not surfaced: %+v", snap.PgxPool)
	}
	if !snap.Cache.Enabled || snap.Cache.Hits != 8 || snap.Cache.HitRate != 0.8 {
		t.Fatalf("cache stats not surfaced: %+v", snap.Cache)
	}
	if snap.UptimeSeconds <= 0 {
		t.Fatalf("uptime = %v, want > 0", snap.UptimeSeconds)
	}
}

func TestStats_CacheDisabledReported(t *testing.T) {
	rec := get(t, testRouter(nil), "127.0.0.1:1") // loopback, no cache func
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var snap snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Cache.Enabled {
		t.Fatalf("cache should be reported disabled, got %+v", snap.Cache)
	}
}
