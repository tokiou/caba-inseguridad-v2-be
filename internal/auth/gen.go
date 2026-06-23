package auth

// Regenerate the sqlc-backed relational data access in internal/auth/db.
// sqlc resolves schema/query paths relative to the config file, so we point at
// the repo-root sqlc.yaml. Run with `go generate ./internal/auth/...`.
//
//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate -f ../../sqlc.yaml
