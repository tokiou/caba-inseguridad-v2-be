from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

import psycopg2
from dotenv import load_dotenv
from psycopg2.extras import execute_values

from common import PROCESSED_DIR

BATCH_SIZE = 5000

INSERT_SQL = """
INSERT INTO crimes (
    source_id,
    year,
    month,
    day,
    date,
    hour,
    crime_type,
    crime_subtype,
    weapon_used,
    motorcycle_used,
    neighborhood,
    commune,
    quantity,
    geom,
    raw_payload,
    updated_at
)
VALUES %s
ON CONFLICT (source_id)
DO UPDATE SET
    year = EXCLUDED.year,
    month = EXCLUDED.month,
    day = EXCLUDED.day,
    date = EXCLUDED.date,
    hour = EXCLUDED.hour,
    crime_type = EXCLUDED.crime_type,
    crime_subtype = EXCLUDED.crime_subtype,
    weapon_used = EXCLUDED.weapon_used,
    motorcycle_used = EXCLUDED.motorcycle_used,
    neighborhood = EXCLUDED.neighborhood,
    commune = EXCLUDED.commune,
    quantity = EXCLUDED.quantity,
    geom = EXCLUDED.geom,
    raw_payload = EXCLUDED.raw_payload,
    updated_at = now()
"""

# Each row maps to: source_id, year, month, day, date, hour, crime_type,
# crime_subtype, weapon_used, motorcycle_used, neighborhood, commune, quantity,
# longitude, latitude, raw_payload.
# geom is built from ST_MakePoint(longitude, latitude) — lng first, lat second.
ROW_TEMPLATE = (
    "(%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, "
    "ST_SetSRID(ST_MakePoint(%s, %s), 4326), %s::jsonb, now())"
)


def build_dsn() -> str:
    dsn = os.getenv("DATABASE_URL")
    if dsn:
        return dsn

    host = os.getenv("POSTGRES_HOST", "localhost")
    port = os.getenv("POSTGRES_PORT", "5432")
    database = os.getenv("POSTGRES_DB")
    user = os.getenv("POSTGRES_USER")
    password = os.getenv("POSTGRES_PASSWORD")

    missing = [
        name
        for name, value in (
            ("POSTGRES_DB", database),
            ("POSTGRES_USER", user),
            ("POSTGRES_PASSWORD", password),
        )
        if not value
    ]
    if missing:
        raise RuntimeError(
            "Missing DATABASE_URL and required POSTGRES_* variables: " + ", ".join(missing)
        )

    return f"postgres://{user}:{password}@{host}:{port}/{database}?sslmode=disable"


def build_row(document: dict[str, Any]) -> tuple[Any, ...]:
    coordinates = document["location"]["coordinates"]
    longitude = coordinates[0]
    latitude = coordinates[1]

    return (
        document["source_id"],
        document["year"],
        document["month"],
        document["day"],
        document["date"],
        document["hour"],
        document["crime_type"],
        document.get("crime_subtype"),
        document["weapon_used"],
        document["motorcycle_used"],
        document.get("neighborhood"),
        document.get("commune"),
        document["quantity"],
        longitude,
        latitude,
        json.dumps(document, ensure_ascii=False),
    )


def flush(cursor, batch: dict[str, tuple[Any, ...]]) -> int:
    if not batch:
        return 0
    execute_values(cursor, INSERT_SQL, list(batch.values()), template=ROW_TEMPLATE, page_size=len(batch))
    return len(batch)


def main() -> None:
    load_dotenv()

    input_path = Path(PROCESSED_DIR / "crimes_normalized.jsonl")
    if not input_path.exists():
        raise FileNotFoundError(f"Missing input file: {input_path}")

    rows_read = 0
    rows_upserted = 0
    rows_failed = 0

    connection = psycopg2.connect(build_dsn())
    try:
        with connection, connection.cursor() as cursor:
            # Dedup by source_id within each batch: ON CONFLICT cannot affect the
            # same row twice in a single statement.
            batch: dict[str, tuple[Any, ...]] = {}

            with input_path.open("r", encoding="utf-8") as file:
                for line in file:
                    stripped = line.strip()
                    if not stripped:
                        continue
                    rows_read += 1
                    try:
                        document = json.loads(stripped)
                        row = build_row(document)
                    except (ValueError, KeyError, IndexError, TypeError):
                        rows_failed += 1
                        continue

                    batch[row[0]] = row

                    if len(batch) >= BATCH_SIZE:
                        rows_upserted += flush(cursor, batch)
                        batch = {}

            rows_upserted += flush(cursor, batch)
    finally:
        connection.close()

    print(f"rows_read: {rows_read}")
    print(f"rows_inserted_or_updated: {rows_upserted}")
    print(f"rows_failed: {rows_failed}")
    print("Postgres load completed.")


if __name__ == "__main__":
    main()
