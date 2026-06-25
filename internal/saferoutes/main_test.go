package saferoutes

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package's test run on a leaked goroutine — the cache tests
// spin up go-redis clients and the handler tests spin up httptest servers, so
// missing a Close() would leak.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
