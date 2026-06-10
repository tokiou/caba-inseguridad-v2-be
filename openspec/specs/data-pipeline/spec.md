# Data Pipeline Specification

## Purpose

Turn raw CABA crime XLSX files into a clean, canonical dataset and load it into the
geospatial datastore. The pipeline is an idempotent, repeatable ETL: analyze raw quality,
normalize valid rows into a domain schema, reject invalid rows with traceability, and upsert
into PostgreSQL + PostGIS.

> Current datastore: **PostgreSQL + PostGIS** (`etl/python/load_to_postgres.py`). The legacy
> MongoDB loader (`etl/python/load_to_mongo.py`) is retained for reference only and is not part
> of the active path.

## Requirements

### Requirement: Raw input discovery and header validation

The pipeline SHALL read every file matching `data/raw/*.xlsx` and validate that each file's
header row exactly matches the expected columns, in order:
`id-mapa, anio, mes, dia, fecha, franja, tipo, subtipo, uso_arma, uso_moto, barrio, comuna, latitud, longitud, cantidad`.

#### Scenario: All raw files discovered

- GIVEN one or more `.xlsx` files in `data/raw/`
- WHEN the analyzer runs
- THEN every matching file is read and counted in the report

#### Scenario: Header mismatch is reported

- GIVEN a file whose header row differs from the expected columns or order
- WHEN header validation runs
- THEN the file is marked invalid in the report with its expected vs found headers
- AND normalization SHALL NOT proceed for files with invalid headers

### Requirement: Raw quality report

The analyzer (`etl/python/analyze_raw_data.py`) SHALL produce
`data/processed/raw_data_report.json` summarizing dataset quality.

#### Scenario: Report contains required sections

- WHEN `analyze_raw_data.py` runs over `data/raw/*.xlsx`
- THEN the report includes: files count, rows per file, total rows, header validation result,
  null/empty counts per column, unique values for `tipo, subtipo, uso_arma, uso_moto, barrio,
  comuna, franja`, coordinate validation summary, date format summary, and `cantidad` numeric
  validation summary

### Requirement: Row normalization to canonical schema

The normalizer (`etl/python/normalize_crimes.py`) SHALL map each valid raw row to the canonical
JSONL schema in `data/processed/crimes_normalized.jsonl`.

The canonical record shape is:

```json
{
  "source_id": "string",
  "year": 2024, "month": 5, "day": 17,
  "date": "2024-05-17", "hour": 18,
  "crime_type": "ROBO", "crime_subtype": "ROBO TOTAL",
  "weapon_used": true, "motorcycle_used": false,
  "neighborhood": "PALERMO", "commune": 14, "quantity": 1,
  "location": { "type": "Point", "coordinates": [longitude, latitude] }
}
```

#### Scenario: Coordinate order is [longitude, latitude]

- GIVEN a raw row with `latitud` and `longitud`
- WHEN it is normalized
- THEN `location.coordinates[0]` is the longitude and `location.coordinates[1]` is the latitude
- AND this order MUST NOT be swapped anywhere downstream

#### Scenario: Boolean flag normalization

- GIVEN `uso_arma` / `uso_moto` values such as `SI/NO`, `S/N`, `1/0`, or empty
- WHEN normalized
- THEN they map to `weapon_used` / `motorcycle_used` booleans
- AND a value that cannot be mapped causes the row to be rejected with an explicit reason

### Requirement: Rejected row traceability

Invalid rows SHALL be written to `data/processed/rejected_rows.jsonl`, one JSON object per row,
including at least `source_file`, `row_number`, `raw_row`, and `reasons` (array of validation
errors).

#### Scenario: Invalid row is traceable

- GIVEN a raw row that fails one or more validations
- WHEN normalization runs
- THEN it is excluded from the normalized output
- AND a rejection record with its source file, row number, raw values, and reasons is written

### Requirement: Idempotent load into PostgreSQL + PostGIS

The loader (`etl/python/load_to_postgres.py`) SHALL read
`data/processed/crimes_normalized.jsonl` and upsert each record into the `crimes` table, keyed by
`source_id`, building `geom` from `ST_SetSRID(ST_MakePoint(longitude, latitude), 4326)`.

#### Scenario: Connection configuration

- WHEN the loader starts
- THEN it connects using `DATABASE_URL` if set, otherwise from `POSTGRES_HOST/PORT/DB/USER/PASSWORD`

#### Scenario: Upsert by source_id is idempotent

- GIVEN the loader has already imported the dataset
- WHEN it runs again over the same input
- THEN no duplicate rows are created (`ON CONFLICT (source_id) DO UPDATE`)
- AND the table row count remains stable

#### Scenario: Geometry stored as SRID 4326 Point

- WHEN a record is loaded
- THEN `geom` is a `GEOMETRY(Point, 4326)` such that `ST_X(geom)` returns longitude (≈ -58.x)
  and `ST_Y(geom)` returns latitude (≈ -34.x)

#### Scenario: Load metrics reported

- WHEN the loader finishes
- THEN it prints `rows_read`, `rows_inserted_or_updated`, and `rows_failed`

### Requirement: Pipeline runtime dependencies

`etl/python/requirements.txt` SHALL declare the loader runtime dependencies, including
`python-dotenv`, `openpyxl`, and `psycopg2-binary` (the legacy `pymongo` is retained while the
Mongo loader exists).

#### Scenario: Dependencies installable

- WHEN `pip install -r etl/python/requirements.txt` runs
- THEN the analyzer, normalizer, and Postgres loader can all execute
