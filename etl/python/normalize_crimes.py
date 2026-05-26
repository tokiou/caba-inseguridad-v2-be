from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from common import (
    PROCESSED_DIR,
    as_clean_str,
    as_int,
    iter_raw_xlsx_records,
    normalize_coordinate,
    parse_month,
    parse_bool_flag,
    parse_date_iso,
    parse_hour_from_franja,
)


def validate_and_normalize(row: dict[str, Any], source_file: str, row_number: int) -> tuple[dict[str, Any] | None, list[str]]:
    reasons: list[str] = []

    source_id = as_clean_str(row["id-mapa"])
    if not source_id:
        reasons.append("id-mapa is empty")

    year = as_int(row["anio"])
    month = parse_month(row["mes"])
    day = as_int(row["dia"])
    if year is None:
        reasons.append("anio is not numeric")
    if month is None or not (1 <= month <= 12):
        reasons.append("mes is invalid")
    date_iso = parse_date_iso(row["fecha"])
    if date_iso is None:
        reasons.append("fecha is invalid")
    elif day is None:
        day = int(date_iso.split("-")[2])

    if day is None or not (1 <= day <= 31):
        reasons.append("dia is invalid")

    hour = parse_hour_from_franja(row["franja"])
    if hour is None:
        reasons.append("franja does not contain a valid hour")

    weapon_used = parse_bool_flag(row["uso_arma"])
    if weapon_used is None:
        reasons.append("uso_arma cannot be mapped to boolean")

    motorcycle_used = parse_bool_flag(row["uso_moto"])
    if motorcycle_used is None:
        reasons.append("uso_moto cannot be mapped to boolean")

    neighborhood = as_clean_str(row["barrio"]).upper()
    if not neighborhood:
        reasons.append("barrio is empty")

    commune = as_int(row["comuna"])
    if commune is None:
        reasons.append("comuna is invalid")

    quantity = as_int(row["cantidad"])
    if quantity is None:
        reasons.append("cantidad is not numeric")
    elif quantity <= 0:
        reasons.append("cantidad must be greater than zero")

    latitude = normalize_coordinate(row["latitud"], -90, 90)
    longitude = normalize_coordinate(row["longitud"], -180, 180)
    if latitude is None or longitude is None:
        reasons.append("coordinates are missing or not numeric")

    crime_type = as_clean_str(row["tipo"]).upper()
    crime_subtype = as_clean_str(row["subtipo"]).upper()
    if not crime_type:
        reasons.append("tipo is empty")
    if not crime_subtype:
        reasons.append("subtipo is empty")

    if reasons:
        return None, reasons

    normalized = {
        "source_id": source_id,
        "year": year,
        "month": month,
        "day": day,
        "date": date_iso,
        "hour": hour,
        "crime_type": crime_type,
        "crime_subtype": crime_subtype,
        "weapon_used": weapon_used,
        "motorcycle_used": motorcycle_used,
        "neighborhood": neighborhood,
        "commune": commune,
        "quantity": quantity,
        "location": {
            "type": "Point",
            "coordinates": [longitude, latitude],
        },
    }
    return normalized, []


def main() -> None:
    _, header_results, records = iter_raw_xlsx_records()
    if not all(item["valid"] for item in header_results):
        raise RuntimeError("Cannot normalize: at least one input file has invalid headers")

    PROCESSED_DIR.mkdir(parents=True, exist_ok=True)
    normalized_path = Path(PROCESSED_DIR / "crimes_normalized.jsonl")
    rejected_path = Path(PROCESSED_DIR / "rejected_rows.jsonl")

    normalized_count = 0
    rejected_count = 0

    with normalized_path.open("w", encoding="utf-8") as normalized_file, rejected_path.open(
        "w", encoding="utf-8"
    ) as rejected_file:
        for record in records:
            normalized, reasons = validate_and_normalize(record.data, record.source_file, record.row_number)
            if normalized is not None:
                normalized_file.write(json.dumps(normalized, ensure_ascii=True) + "\n")
                normalized_count += 1
                continue

            rejected = {
                "source_file": record.source_file,
                "row_number": record.row_number,
                "raw_row": record.data,
                "reasons": reasons,
            }
            rejected_file.write(json.dumps(rejected, ensure_ascii=True, default=str) + "\n")
            rejected_count += 1

    print(f"Normalized rows: {normalized_count}")
    print(f"Rejected rows: {rejected_count}")
    print(f"Output: {normalized_path}")
    print(f"Output: {rejected_path}")


if __name__ == "__main__":
    main()
