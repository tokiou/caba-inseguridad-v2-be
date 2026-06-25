# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache module downloads on an unchanged go.mod/go.sum.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Static, stripped binary so it runs on a distroless/static base (no libc, no CGO).
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---- runtime stage ----
# distroless static: no shell, no package manager, runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/api /app/api
# openapi.yaml is served at /openapi.yaml and backs the /docs Swagger UI.
COPY --from=build /src/openapi.yaml /app/openapi.yaml

EXPOSE 8080
USER nonroot:nonroot
# Config comes from env vars (DATABASE_URL, REDIS_*, JWT_SECRET, …); no .env in the image.
ENTRYPOINT ["/app/api"]
