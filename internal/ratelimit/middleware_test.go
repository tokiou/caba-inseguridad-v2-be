package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestClient(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// do sends one request from ip through h and returns the status code.
func do(h http.Handler, ip string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = ip + ":12345"
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestNewMiddleware_AllowsThenBlocks(t *testing.T) {
	mw, err := NewMiddleware(newTestClient(t), "3-M", "test:allow-block")
	if err != nil {
		t.Fatalf("NewMiddleware: %v", err)
	}
	h := mw(okHandler())

	for i := 1; i <= 4; i++ {
		want := http.StatusOK
		if i == 4 {
			want = http.StatusTooManyRequests
		}
		if got := do(h, "10.0.0.1"); got != want {
			t.Fatalf("request %d: status = %d, want %d", i, got, want)
		}
	}
}

func TestNewMiddleware_DistinctPrefixesDoNotShareCounters(t *testing.T) {
	client := newTestClient(t)
	a, err := NewMiddleware(client, "1-M", "test:prefix-a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewMiddleware(client, "1-M", "test:prefix-b")
	if err != nil {
		t.Fatal(err)
	}
	ha, hb := a(okHandler()), b(okHandler())
	const ip = "10.0.0.2"

	if got := do(ha, ip); got != http.StatusOK {
		t.Fatalf("A first: status = %d, want 200", got)
	}
	if got := do(ha, ip); got != http.StatusTooManyRequests {
		t.Fatalf("A second: status = %d, want 429", got)
	}
	// B has its own counter, so the same IP is still allowed once on B.
	if got := do(hb, ip); got != http.StatusOK {
		t.Fatalf("B first: status = %d, want 200 (separate counter)", got)
	}
}

func TestNewMiddleware_SeparateIPsSeparateQuotas(t *testing.T) {
	mw, err := NewMiddleware(newTestClient(t), "1-M", "test:per-ip")
	if err != nil {
		t.Fatal(err)
	}
	h := mw(okHandler())

	if got := do(h, "10.0.0.10"); got != http.StatusOK {
		t.Fatalf("IP A first: %d, want 200", got)
	}
	if got := do(h, "10.0.0.10"); got != http.StatusTooManyRequests {
		t.Fatalf("IP A second: %d, want 429", got)
	}
	if got := do(h, "10.0.0.11"); got != http.StatusOK {
		t.Fatalf("IP B first: %d, want 200 (independent of IP A)", got)
	}
}

func TestNewMiddleware_InvalidRate(t *testing.T) {
	if _, err := NewMiddleware(newTestClient(t), "not-a-rate", "test:bad"); err == nil {
		t.Fatal("expected an error for an invalid rate format")
	}
}

func TestNewMiddlewares_BuildsAllFour(t *testing.T) {
	m, err := NewMiddlewares(newTestClient(t))
	if err != nil {
		t.Fatalf("NewMiddlewares: %v", err)
	}
	for name, mw := range map[string]Middleware{
		"RoutesSafe":     m.RoutesSafe,
		"AuthLogin":      m.AuthLogin,
		"CrimesNearby":   m.CrimesNearby,
		"RoadgraphStats": m.RoadgraphStats,
	} {
		if mw == nil {
			t.Errorf("%s middleware is nil", name)
		}
	}
}

func TestPassthrough_NeverBlocks(t *testing.T) {
	h := Passthrough().RoutesSafe(okHandler())
	for i := range 50 {
		if got := do(h, "10.0.0.3"); got != http.StatusOK {
			t.Fatalf("passthrough request %d blocked with %d", i, got)
		}
	}
}
