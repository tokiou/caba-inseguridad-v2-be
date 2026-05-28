# CLAUDE.md — caba-inseguridad-v2-be

## Project

Backend for **CABA Rutas Seguras**: given two points in Buenos Aires (CABA), return a route that minimizes exposure to crime hotspots. Crime data comes from open CABA datasets loaded into MongoDB.

## Stack

- **Go 1.23** — HTTP API (`github.com/tokiou/caba-inseguridad-routes-go`)
- **Python 3** — ETL pipeline under `etl/python/`
- **MongoDB** — geospatial crime data (`2dsphere` index on `location`)
- **chi** — HTTP router
- **godotenv** — env loading

## Repository layout

```
cmd/api/main.go                   # entrypoint: wires deps, starts server
internal/
  app/app.go                      # App struct (router + mongo client)
  app/routes.go                   # chi route registration
  config/config.go                # Config struct loaded from env
  platform/mongo/client.go        # MongoDB connect + ping
  health/handler.go               # GET /api/v1/health
  crimes/
    model.go                      # Crime + GeoJSON structs
    dto.go                        # NearbyCrimesQuery + NearbyCrimesResponse
    repository.go                 # Repository interface
    mongo_repository.go           # $nearSphere implementation
    service.go                    # business validation
    handler.go                    # HTTP parsing + JSON responses
    service_test.go
    handler_test.go
  httpx/response.go               # shared JSON helpers
etl/python/
  analyze_raw_data.py             # raw quality report
  normalize_crimes.py             # XLSX → JSONL normalization
  load_to_mongo.py                # upsert into MongoDB
  requirements.txt
data/
  raw/                            # source XLSX files (not committed)
  processed/                      # generated JSONL/JSON artifacts
docs/sdd/
  done/                           # completed specs
  wip/                            # specs in progress
```

## Architecture rules

The request flow is strictly layered — do not skip layers:

```
HTTP request → chi router → handler → service → repository interface → MongoRepository → MongoDB
```

- **Handlers** parse HTTP params and return JSON. No MongoDB, no business logic.
- **Services** validate domain rules and call the repository. No HTTP, no MongoDB details.
- **Repositories** encapsulate all data access. No HTTP logic.
- **`main.go`** only wires dependencies and starts the server.

## Environment variables

```env
APP_ENV=development
HTTP_PORT=8080
MONGO_URI=mongodb://localhost:27017
MONGO_DATABASE=caba_routes
MONGO_CRIMES_COLLECTION=crimes
```

Copy `.env.example` to `.env` before running.

## Running

```bash
# Start the API
go run ./cmd/api

# Run tests
go test ./...
```

## ETL pipeline (Python)

```bash
cd etl/python
pip install -r requirements.txt
python analyze_raw_data.py    # → data/processed/raw_data_report.json
python normalize_crimes.py    # → data/processed/crimes_normalized.jsonl + rejected_rows.jsonl
python load_to_mongo.py       # upserts into MongoDB (idempotent)
```

Normalized schema includes `source_id` (unique key), `location` as GeoJSON Point with `[lng, lat]`, and boolean fields `weapon_used` / `motorcycle_used`.

## Completed milestones

| # | Spec | Status |
|---|------|--------|
| 001 | Initial data pipeline (ETL) | done |
| 002 | Go crimes API + MongoDB geospatial query | done |

## Not yet implemented

- OpenRouteService integration
- Safe route calculation / risk scoring
- Route endpoint
- Frontend
- Authentication
- Aggregated statistics
