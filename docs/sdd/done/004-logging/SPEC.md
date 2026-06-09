# Spec 004 — Structured Logging & Request Observability

## Goal

Make the API observable in development and production. Today, running `go run ./cmd/api` prints only
the `server listening` line and **nothing** when endpoints are hit, because there is no HTTP
request-logging middleware. This spec adds structured per-request logging, configurable log level and
format (colored console in dev, JSON in prod), panic recovery, and request-ID correlation — without
breaking the existing layered architecture or the dependency-injected logger.

## Branch

`feat/logging`

## Problem (current state)

- Logger is hardcoded in `cmd/api/main.go`: `slog.New(slog.NewJSONHandler(os.Stdout, nil))` — JSON
  only, fixed INFO level, not configurable.
- `internal/app/routes.go` registers only `cors.Handler` — no request logging, no RequestID, no
  Recoverer. Hitting an endpoint logs nothing.
- `internal/httpx/response.go` uses the global `slog.Error(...)`, inconsistent with the
  constructor-injected logger used everywhere else.
- The JSON handler is built with `nil` options, so it would ignore any level even if one were set.

## Scope

### Included

- HTTP request-logging middleware emitting `method`, `path`, `status`, `duration`, `request_id`.
- Configurable log level (`LOG_LEVEL`) and format (`LOG_FORMAT`) via env.
- Colored, human-readable console output in dev (`tint`); JSON in prod.
- Panic recovery middleware (panic → 500, logged at error level).
- Request-ID generation + correlation to the `X-Request-Id` response header.
- A logger factory under `internal/platform/logger/`.
- `slog.SetDefault` so the global logger in `httpx/response.go` honors the configured handler.

### Not Included

- Log shipping / aggregation (Loki, ELK, Sentry).
- Distributed tracing (OpenTelemetry spans).
- Request/response body logging.
- Per-domain or per-route log sampling.
- Audit logging or PII scrubbing rules.

---

## Dependencies

Two new Go modules (user-chosen "batteries included" approach):

```
github.com/lmittmann/tint        # colored slog console handler for dev
github.com/samber/slog-http      # structured HTTP request-logging middleware (pkg: sloghttp)
```

chi v5, `chi/v5/middleware`, and `go-chi/cors` are already present.

---

## Environment Variables

New variables in `internal/config/config.go` and `.env.example`:

```env
LOG_LEVEL=info     # debug | info | warn | error — default info
LOG_FORMAT=json    # json | text — code default json; .env template uses text for dev
```

---

## Architecture

```
HTTP request
   ↓
middleware.RequestID        (chi — generates id into request context)
   ↓
cors.Handler                (existing — short-circuits OPTIONS preflight)
   ↓
sloghttp.Recovery           (panic → 500, observed by the logger)
   ↓
sloghttp.New(logger)        (logs method/path/status/duration at response time)
   ↓
injectRequestID             (copies chi req id into the log line via AddCustomAttributes)
   ↓
chi router → handlers → services → repositories
```

Logger construction lives in `internal/platform/logger/` (infra concern, mirrors
`internal/platform/mongo`). It takes plain `format, level string` and does NOT import
`internal/config`, keeping the platform layer free of app config types.

**Middleware ordering rationale** (chi `Use` runs outer→inner, first-registered first inbound):

1. `RequestID` outermost — the id must exist before anything logs.
2. `cors` — cheap, short-circuits preflight before heavier middleware.
3. `sloghttp.Recovery` — wraps the inner handler so a panic becomes a 500 the logger still records.
4. `sloghttp.New` — captures the final status and duration.
5. `injectRequestID` — after `sloghttp.New` so the per-request log context exists when attrs are added.

---

## Logger Factory — `internal/platform/logger/logger.go`

```go
func New(format string, level string) *slog.Logger
```

- `format == "text"` → `tint.NewHandler(os.Stdout, &tint.Options{Level: lvl})`.
- otherwise → `slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})`.
- `parseLevel(level)`: `debug|info|warn|error` (case-insensitive), default `info`.

Note: the JSON branch MUST pass `&slog.HandlerOptions{Level: lvl}` — the current `nil` silently
ignores level.

---

## Config — `internal/config/config.go`

Add to `Config`:

```go
LogLevel  string
LogFormat string
```

Add to `Load()`:

```go
LogLevel:  getEnv("LOG_LEVEL", "info"),
LogFormat: getEnv("LOG_FORMAT", "json"),
```

---

## Wiring — `cmd/api/main.go`

Load `cfg` before the logger (the logger now depends on config):

```go
cfg := config.Load()
logger := loggerplatform.New(cfg.LogFormat, cfg.LogLevel)
slog.SetDefault(logger)
```

Import alias `loggerplatform` avoids colliding with the local `logger` variable. `app.New(startCtx,
cfg, logger)` is unchanged.

---

## Router — `internal/app/routes.go`

Change signature to:

```go
func NewRouter(log *slog.Logger, registrars ...Registrar) http.Handler
```

Register middleware in the order above. Add a local shim:

```go
func injectRequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if id := middleware.GetReqID(r.Context()); id != "" {
            sloghttp.AddCustomAttributes(r, slog.String("request_id", id))
        }
        next.ServeHTTP(w, r)
    })
}
```

Do NOT use chi `middleware.Logger`/`middleware.Recoverer` (replaced by sloghttp). Do NOT set
`sloghttp` `WithRequestID:true` — it only reads the client's `X-Request-Id` request header, not the
server-generated id.

`internal/app/app.go`: pass the logger — `NewRouter(log, healthHandler, crimesHandler, routesHandler)`.

---

## httpx — `internal/httpx/response.go`

No functional change. The global `slog.Error` now honors the configured handler via
`slog.SetDefault(logger)`. Injecting a logger into `WriteJSON`/`WriteError`/... would touch every call
site for a single encode-failure line that virtually never fires — rejected.

---

## Acceptance Criteria

1. With `LOG_FORMAT=text LOG_LEVEL=debug`, hitting any endpoint prints one colored line with
   `method`, `path`, `status`, `duration`, and `request_id`.
2. With defaults (no `.env`), the same request emits a single JSON line with those fields.
3. The response carries an `X-Request-Id` header matching the logged `request_id`.
4. With `LOG_LEVEL=warn`, the INFO request line is suppressed.
5. A panic inside a handler is recovered, returns 500, and is logged at error level (process stays up).
6. `go build ./...` and `go test ./...` pass (existing crimes/routes handler tests inject the logger
   directly and are unaffected; the only `NewRouter` call site is `app.go`).

---

## Manual Test Commands

```bash
# DEV — colored, debug
LOG_FORMAT=text LOG_LEVEL=debug go run ./cmd/api
curl -i 'http://localhost:8080/api/v1/health'

# PROD — JSON, default
go run ./cmd/api
curl -s 'http://localhost:8080/api/v1/health' >/dev/null

# Level filtering — INFO request line suppressed
LOG_LEVEL=warn go run ./cmd/api
curl -s 'http://localhost:8080/api/v1/health' >/dev/null
```

---

## Error observability fix — ORS client status mapping

Folded into this branch because it directly affects what the logs say. `internal/routes/ors_client.go`
mapped **every** ORS 4xx to `ErrRouteNotFound` (→ 404 `route_not_found`), masking auth/rate-limit/bad-
request failures as "no route found". With an empty `ORS_API_KEY`, ORS returns **403** "Access to this
API has been disallowed", which surfaced to clients as a misleading 404.

Fix:
- Only ORS **404** → `ErrRouteNotFound` (genuine no-route).
- All other 4xx (401/403 auth, 429 rate limit, 400) and 5xx → `ErrExternalService` (→ 502).
- The wrapped error now carries the ORS status code + a bounded body snippet, so the cause is visible
  in logs.
- `routes/handler.go` now logs `ErrExternalService` at error level (previously only the 500 default was
  logged), so upstream/config failures appear in the logs.
- New `internal/routes/ors_client_test.go` covers the status→error mapping (200/empty/404/401/403/429/
  400/500).
- Also fixed an unrelated ORS bug surfaced while debugging: the client sent `Accept: application/json`,
  but ORS's GET directions endpoint only serves GeoJSON and returned 406. Changed to
  `Accept: application/geo+json`. Verified end-to-end (Obelisco→Chacarita returns 200).

## go-logging skill compliance

Audited against the `go-logging` skill. Normative rules all satisfied (slog, structured static
messages, log-or-return at the boundary, no secrets logged, level semantics). Three advisory fixes
applied:

1. **request_id on error logs** — handler error logs used the base injected logger and lacked
   `request_id`, so they couldn't be correlated to the request log line. Added `httpx.LogWith(log, r)`
   (new `internal/httpx/logging.go`) which enriches the logger with chi's request id; `writeError` now
   takes `r` and uses it. Both crimes and routes handlers updated.
2. **`err` key** — renamed the `"error"` attribute to `"err"` across all `slog.Error` call sites
   (main.go, httpx/response.go, both handlers) to match the skill convention.
3. **No client IP** — set `logConfig.WithClientIP = false` in the router (PII consideration).

Note on secrets: `sloghttp` runs with `WithRequestHeader=false`, so the `Authorization` header (the ORS
API key) is never logged, and the key is not part of the logged request URL.

## Files

- `internal/platform/logger/logger.go` (new)
- `internal/httpx/logging.go` (new — `LogWith` request-id enrichment)
- `internal/routes/ors_client.go`, `internal/routes/handler.go`, `internal/routes/ors_client_test.go` (new)
- `internal/crimes/handler.go` (request_id + err key)
- `internal/config/config.go`
- `cmd/api/main.go`
- `internal/app/routes.go`
- `internal/app/app.go`
- `.env.example`, `CLAUDE.md`
