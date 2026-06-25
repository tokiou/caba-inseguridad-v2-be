# Tasks — CI, coverage, goleak

## 1. goleak
- [x] 1.1 `go get go.uber.org/goleak`.
- [x] 1.2 Close Redis clients (and any httptest servers) in `internal/ratelimit` and
      `internal/saferoutes` tests so no goroutine outlives a test.
- [x] 1.3 Add `TestMain` with `goleak.VerifyTestMain(m)` to those packages.
- [x] 1.4 `go test ./internal/ratelimit/... ./internal/saferoutes/...` green (no leaks).

## 2. Coverage + Makefile
- [x] 2.1 `Makefile`: `test`, `test-race`, `cover`, `cover-html`, `vet`, `tidy`.
- [x] 2.2 `make cover` produces `coverage.out` + a total via `go tool cover -func`.

## 3. CI
- [x] 3.1 `.github/workflows/ci.yml` — push/PR: setup-go (1.25), module cache, build,
      vet, `go test -race -coverprofile=coverage.out ./...`, print total coverage.

## 4. Verify
- [x] 4.1 `go build ./...`, `go vet ./...`, `go test -race ./...` all green locally.
- [x] 4.2 `make cover` prints a total coverage number.
- [x] 4.3 `.github/workflows/ci.yml` parses (yaml) and steps are coherent.
