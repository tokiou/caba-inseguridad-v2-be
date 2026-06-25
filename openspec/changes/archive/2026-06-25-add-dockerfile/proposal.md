# Add a Dockerfile for the API

## Why

The API only runs from source on the host today; the compose stack has Postgres
and Redis but no app image. A reviewer (or a deploy) expects to build and run the
backend as a container. A small, secure image makes the project deployable and
completes the "containerized stack" story.

## What changes

1. **Dockerfile** — multi-stage: build with `golang:1.25-alpine` (static
   `CGO_ENABLED=0` binary, `-trimpath -ldflags="-s -w"`), final stage
   `gcr.io/distroless/static-debian12:nonroot` (no shell, runs as non-root,
   minimal attack surface). Copies the binary + `openapi.yaml` (for `/docs`),
   exposes 8080.
2. **.dockerignore** — keep the build context to Go sources + `go.mod/sum` +
   `openapi.yaml`; exclude VCS, data, docs, benchmark output, specs, secrets.

Config still comes entirely from env vars (the app reads no `.env` in the image).
No code changes.

## In scope

- `Dockerfile`, `.dockerignore`, and verifying `docker build` + `docker run`
  against the real Postgres/Redis (health + a DB endpoint + `/debug/stats`).

## Out of scope

- An `api` service in docker-compose / CI image publishing (follow-up; this env's
  host Redis occupies 6379 so a full `docker compose up` isn't exercised here).
- Running DB migrations from the container (kept a separate step).
