# Health Check Specification

## Purpose

Provide a simple liveness endpoint so operators and orchestrators can verify the API process is
up and serving HTTP.

## Requirements

### Requirement: Health endpoint

The API SHALL expose `GET /api/v1/health` returning HTTP 200 with a minimal status body.

#### Scenario: Service is up

- WHEN a client sends `GET /api/v1/health`
- THEN the response is HTTP 200
- AND the body is `{ "status": "ok" }`
