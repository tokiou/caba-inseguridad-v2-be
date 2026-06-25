.PHONY: build vet test test-race cover cover-html tidy

# Build all packages.
build:
	go build ./...

vet:
	go vet ./...

# Unit tests.
test:
	go test ./...

# Unit tests with the race detector + goroutine-leak checks (goleak runs in the
# TestMain of internal/ratelimit and internal/saferoutes).
test-race:
	go test -race -count=1 ./...

# Coverage profile + total. covermode=atomic so it composes with -race in CI.
cover:
	go test -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -1

cover-html: cover
	go tool cover -html=coverage.out -o coverage.html
	@echo "wrote coverage.html"

tidy:
	go mod tidy

# Integration tests need a populated PostGIS + road-graph DB (and Redis); run
# locally, not in CI. Requires DATABASE_URL.
.PHONY: test-integration
test-integration:
	go test -tags=integration ./...
