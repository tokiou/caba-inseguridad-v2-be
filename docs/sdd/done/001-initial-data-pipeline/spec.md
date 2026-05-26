# Initial data pipeline for CABA crime dataset

## Problem
The project needs a first end-to-end ETL baseline for CABA crime raw files, focused on:
1) raw dataset quality analysis,
2) row-level normalization into a consistent domain schema, and
3) idempotent loading into MongoDB.

At this stage there is no implemented pipeline under `etl/python/`, and no generated processed artifacts in `data/processed/`.

## Scope
### In scope
- Read **all `.xlsx` files** from `data/raw/` (clarification: previous “CSV” wording was an error).
- Validate expected input headers.
- Produce a raw analysis report JSON.
- Normalize valid rows into a canonical JSONL schema.
- Reject invalid rows into a rejection JSONL with traceability.
- Load normalized rows into MongoDB with upsert semantics and required indexes.
- Add Python requirements for loader runtime.
- Add `.env.example` for Mongo connection variables.

### Out of scope
- Go API implementation.
- Routing implementation.
- OpenRouteService integration.
- Frontend implementation.
- Pre-load aggregation by neighborhood.

## Requirements
1. **Input discovery**
   - Read every file matching `data/raw/*.xlsx`.

2. **Header validation**
   - Expected header list and order:
     - `id-mapa, anio, mes, dia, fecha, franja, tipo, subtipo, uso_arma, uso_moto, barrio, comuna, latitud, longitud, cantidad`
   - Validation result must be included in the report.

3. **Raw analyzer script**
   - Create `etl/python/analyze_raw_data.py`.
   - Generate `data/processed/raw_data_report.json` with:
     - files count,
     - row count per file,
     - total rows,
     - header validation result,
     - null/empty counts per column,
     - unique values for `tipo, subtipo, uso_arma, uso_moto, barrio, comuna, franja`,
     - coordinate validation summary,
     - date format summary,
     - `cantidad` numeric validation summary.

4. **Normalizer script**
   - Create `etl/python/normalize_crimes.py`.
   - Generate `data/processed/crimes_normalized.jsonl`.
   - Generate `data/processed/rejected_rows.jsonl`.

5. **Normalized output schema**
   - Each valid row must map to:
```json
{
  "source_id": "...",
  "year": 2024,
  "month": 5,
  "day": 17,
  "date": "2024-05-17",
  "hour": 18,
  "crime_type": "ROBO",
  "crime_subtype": "ROBO TOTAL",
  "weapon_used": true,
  "motorcycle_used": false,
  "neighborhood": "PALERMO",
  "commune": 14,
  "quantity": 1,
  "location": {
    "type": "Point",
    "coordinates": [longitude, latitude]
  }
}
```

6. **Boolean normalization rule (approved clarification)**
   - `uso_arma` and `uso_moto` must be normalized with robust mappings, including at least equivalent representations of yes/no such as `SI/NO`, `S/N`, `1/0`, and empty/null handling.
   - Unmappable values must cause row rejection with explicit reason.

7. **Rejected row traceability (approved clarification)**
   - `rejected_rows.jsonl` must include, per rejected record, at least:
     - `source_file`,
     - `row_number`,
     - `raw_row`,
     - `reasons` (array of validation/normalization errors).

8. **Mongo loader script**
   - Create `etl/python/load_to_mongo.py`.
   - Loader must:
     - read `data/processed/crimes_normalized.jsonl`,
     - connect using `MONGO_URI`, `MONGO_DATABASE`, `MONGO_CRIMES_COLLECTION`,
     - create unique index on `source_id`,
     - create `2dsphere` index on `location`,
     - upsert by `source_id`,
     - avoid duplicates when run multiple times.

9. **Upsert strategy (approved clarification)**
   - Prefer batched `bulk_write` + `UpdateOne(..., upsert=True)` for idempotency and performance.

10. **Dependencies and env template**
    - Add `etl/python/requirements.txt` with:
      - `pymongo`
      - `python-dotenv`
    - Add `.env.example` with:
      - `MONGO_URI=mongodb://localhost:27017`
      - `MONGO_DATABASE=caba_routes`
      - `MONGO_CRIMES_COLLECTION=crimes`

## Data impact
- New generated artifacts in `data/processed/`:
  - `raw_data_report.json`
  - `crimes_normalized.jsonl`
  - `rejected_rows.jsonl`
- New/updated MongoDB documents in crimes collection.
- New MongoDB indexes:
  - unique `source_id`
  - `2dsphere` on `location`

## API changes
- None in this iteration.
- No Go API or endpoint surface modifications are included.

## Permissions
- Filesystem read/write in repository paths:
  - `data/raw/`, `data/processed/`, `etl/python/`, project root (`.env.example`).
- Network access to MongoDB endpoint provided by `MONGO_URI`.
- No external third-party API permissions required.

## Acceptance criteria
1. Running `analyze_raw_data.py` over `data/raw/*.xlsx` creates `data/processed/raw_data_report.json` with all required sections.
2. Header validation checks exact expected columns and reports pass/fail per file.
3. Running `normalize_crimes.py` creates both JSONL outputs:
   - valid normalized rows (`crimes_normalized.jsonl`),
   - rejected rows with traceability fields and reasons (`rejected_rows.jsonl`).
4. Normalized records match the required schema and types (including GeoJSON point, date, numeric, and booleans).
5. `uso_arma` and `uso_moto` normalization supports expected yes/no variants and rejects unknown values explicitly.
6. Running `load_to_mongo.py` creates required indexes and upserts by `source_id` without creating duplicates across repeated runs.
7. `etl/python/requirements.txt` and `.env.example` exist with required entries.
8. No code related to API/routing/ORS/frontend/aggregation-before-load is added.
