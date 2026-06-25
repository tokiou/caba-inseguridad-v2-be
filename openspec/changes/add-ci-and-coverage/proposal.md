# Add CI pipeline, coverage, and goroutine-leak detection

## Why

The project has solid tests but nothing runs them automatically, coverage is
never measured, and goroutine leaks (the API spawns pool/Redis/server goroutines)
go undetected. For the work to credibly demonstrate backend + testing maturity, a
reviewer cloning the repo should see a green CI badge, a coverage number, and
leak-checked tests — not have to run anything by hand.

## What changes

1. **goroutine-leak detection** — add `go.uber.org/goleak`; a `TestMain` with
   `goleak.VerifyTestMain` in the packages that spawn goroutines (`internal/ratelimit`,
   `internal/saferoutes`), with their Redis clients / httptest servers closed so the
   check is meaningful and green.
2. **coverage** — a `Makefile` with `test`, `test-race`, `cover` (`-coverprofile`),
   and `cover-html` targets; CI prints the total coverage via `go tool cover -func`.
3. **CI** — `.github/workflows/ci.yml`: on push/PR, set up Go, `go build`, `go vet`,
   and `go test -race -coverprofile` across all packages (leak checks run inside the
   suite), then report total coverage.

This is dev tooling — no runtime behavior changes, so no capability delta. New
dev/test dep: `go.uber.org/goleak`.

## In scope

- goleak wiring (+ closing leaked test resources), Makefile, GitHub Actions CI
  running build/vet/race-tests/coverage on the Go unit suite.

## Out of scope

- Running the `//go:build integration` tests in CI (they need a populated PostGIS +
  road-graph dataset that isn't in the repo) — documented as a local step.
- golangci-lint config (next easy add), coverage gates/badges, release automation,
  and Docker image builds in CI (the Dockerfile lands in a separate change).
