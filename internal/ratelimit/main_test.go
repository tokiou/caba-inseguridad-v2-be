package ratelimit

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain fails the package's test run if any goroutine outlives the tests —
// this package spawns go-redis pool goroutines, so leaks are plausible.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
