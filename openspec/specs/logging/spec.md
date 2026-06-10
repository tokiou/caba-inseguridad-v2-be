# Logging & Observability Specification

## Purpose

Make the API observable in development and production through structured per-request logging,
configurable level and format, panic recovery, and request-ID correlation — without coupling the
platform logger to application config or breaking the layered architecture.

## Requirements

### Requirement: Structured per-request logging

The API SHALL log one structured line per HTTP request including `method`, `path`, `status`,
`duration`, and `request_id`.

#### Scenario: Request is logged

- WHEN any endpoint is hit
- THEN exactly one request log line is emitted with `method`, `path`, `status`, `duration`, and
  `request_id`

### Requirement: Configurable level and format

Log level and format SHALL be configurable via `LOG_LEVEL` (`debug|info|warn|error`, default `info`)
and `LOG_FORMAT` (`text` → colored console for dev, otherwise JSON; code default `json`).

#### Scenario: Colored dev output

- GIVEN `LOG_FORMAT=text` and `LOG_LEVEL=debug`
- WHEN an endpoint is hit
- THEN a single colored console line is printed with the request fields

#### Scenario: JSON production output

- GIVEN no overrides (defaults)
- WHEN an endpoint is hit
- THEN a single JSON log line is emitted

#### Scenario: Level filtering

- GIVEN `LOG_LEVEL=warn`
- WHEN an endpoint is hit and the request line is INFO level
- THEN the request line is suppressed

### Requirement: Request-ID correlation

The server SHALL generate a request ID, attach it to the request log line, and return it in the
`X-Request-Id` response header.

#### Scenario: Response carries matching request id

- WHEN a request is served
- THEN the `X-Request-Id` response header matches the `request_id` in the log line

#### Scenario: Error logs are correlated

- WHEN a handler logs an error
- THEN the error log includes the same `request_id` as the request line

### Requirement: Panic recovery

A panic inside a handler SHALL be recovered, returning HTTP 500 logged at error level, with the
process staying up.

#### Scenario: Handler panic becomes 500

- GIVEN a handler that panics
- WHEN the endpoint is hit
- THEN the response is HTTP 500
- AND the panic is logged at error level
- AND the server process keeps running

### Requirement: No secrets in logs

Sensitive data SHALL NOT be logged. Request headers (including `Authorization`, which carries the
ORS API key) and client IP are excluded from request logging.

#### Scenario: Authorization header not logged

- WHEN a request with an `Authorization` header is served
- THEN the header value does not appear in any log line
