from __future__ import annotations

import json
from collections import defaultdict
from pathlib import Path

from common import (
    EXPECTED_HEADERS,
    PROCESSED_DIR,
    UNIQUE_COLUMNS,
    as_clean_str,
    as_int,
    is_empty,
    iter_raw_xlsx_records,
    normalize_coordinate,
    parse_date_iso,
)


def main() -> None:
    files, header_results, records = iter_raw_xlsx_records()

    rows_per_file = {path.name: 0 for path in files}
    for record in records:
        rows_per_file[record.source_file] += 1

    null_counts = {column: 0 for column in EXPECTED_HEADERS}
    uniques: dict[str, set[str]] = {column: set() for column in UNIQUE_COLUMNS}

    coordinates = {
        "valid": 0,
        "invalid": 0,
        "missing": 0,
        "out_of_range": 0,
    }
    date_summary = {
        "valid": 0,
        "invalid": 0,
        "missing": 0,
        "raw_patterns": defaultdict(int),
    }
    quantity_summary = {
        "valid_numeric": 0,
        "invalid_numeric": 0,
        "missing": 0,
        "non_positive": 0,
    }

    for record in records:
        row = record.data

        for column in EXPECTED_HEADERS:
            value = row[column]
            if is_empty(value):
                null_counts[column] += 1

        for column in UNIQUE_COLUMNS:
            value = row[column]
            if not is_empty(value):
                uniques[column].add(as_clean_str(value))

        lat = normalize_coordinate(row["latitud"], -90, 90)
        lng = normalize_coordinate(row["longitud"], -180, 180)
        if lat is None or lng is None:
            coordinates["missing"] += 1
            coordinates["invalid"] += 1
        else:
            coordinates["valid"] += 1

        date_raw = row["fecha"]
        if is_empty(date_raw):
            date_summary["missing"] += 1
        else:
            raw_text = as_clean_str(date_raw)
            date_summary["raw_patterns"][raw_text[:10]] += 1
            if parse_date_iso(date_raw) is None:
                date_summary["invalid"] += 1
            else:
                date_summary["valid"] += 1

        quantity = as_int(row["cantidad"])
        if quantity is None:
            quantity_summary["missing"] += 1
            quantity_summary["invalid_numeric"] += 1
        else:
            quantity_summary["valid_numeric"] += 1
            if quantity <= 0:
                quantity_summary["non_positive"] += 1

    report = {
        "files_count": len(files),
        "rows_per_file": rows_per_file,
        "total_rows": sum(rows_per_file.values()),
        "header_validation": {
            "expected_headers": EXPECTED_HEADERS,
            "all_valid": all(item["valid"] for item in header_results),
            "files": header_results,
        },
        "null_or_empty_counts": null_counts,
        "unique_values": {column: sorted(values) for column, values in uniques.items()},
        "coordinate_validation_summary": coordinates,
        "date_format_summary": {
            "valid": date_summary["valid"],
            "invalid": date_summary["invalid"],
            "missing": date_summary["missing"],
            "raw_patterns": dict(sorted(date_summary["raw_patterns"].items())),
        },
        "cantidad_numeric_validation_summary": quantity_summary,
    }

    PROCESSED_DIR.mkdir(parents=True, exist_ok=True)
    output_path = Path(PROCESSED_DIR / "raw_data_report.json")
    output_path.write_text(json.dumps(report, ensure_ascii=True, indent=2), encoding="utf-8")
    print(f"Report created: {output_path}")


if __name__ == "__main__":
    main()
    normalize_coordinate,
