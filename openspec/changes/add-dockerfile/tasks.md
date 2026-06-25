# Tasks — Dockerfile

- [x] 1. Write `Dockerfile` (multi-stage: golang:1.25-alpine build → distroless
      static nonroot; static binary, copy openapi.yaml, EXPOSE 8080).
- [x] 2. Write `.dockerignore` (Go sources + go.mod/sum + openapi.yaml only).
- [x] 3. `docker build` succeeds.
- [x] 4. `docker run` against real Postgres (5434) + Redis: `/health` 200,
      `/roadgraph/stats` 200, `/debug/stats` 200.
- [x] 5. Bring the image/container down so nothing keeps running.
- [x] 6. Note how to build/run in CLAUDE.md.
- [x] 7. Merge to main, push.
