from __future__ import annotations

import json
import os
from pathlib import Path

from dotenv import load_dotenv
from pymongo import MongoClient, UpdateOne

from common import PROCESSED_DIR


def require_env(name: str) -> str:
    value = os.getenv(name)
    if not value:
        raise RuntimeError(f"Missing required environment variable: {name}")
    return value


def main() -> None:
    load_dotenv()

    mongo_uri = require_env("MONGO_URI")
    mongo_database = require_env("MONGO_DATABASE")
    mongo_collection = require_env("MONGO_CRIMES_COLLECTION")

    input_path = Path(PROCESSED_DIR / "crimes_normalized.jsonl")
    if not input_path.exists():
        raise FileNotFoundError(f"Missing input file: {input_path}")

    client = MongoClient(mongo_uri)
    collection = client[mongo_database][mongo_collection]

    collection.create_index("source_id", unique=True)
    collection.create_index([("location", "2dsphere")])

    operations: list[UpdateOne] = []
    total_rows = 0

    with input_path.open("r", encoding="utf-8") as file:
        for line in file:
            stripped = line.strip()
            if not stripped:
                continue
            document = json.loads(stripped)
            source_id = document["source_id"]
            operations.append(UpdateOne({"source_id": source_id}, {"$set": document}, upsert=True))
            total_rows += 1

    if not operations:
        print("No rows to load.")
        return

    result = collection.bulk_write(operations, ordered=False)
    print(f"Total input rows: {total_rows}")
    print(f"Upserted: {result.upserted_count}")
    print(f"Modified: {result.modified_count}")
    print("Mongo load completed.")


if __name__ == "__main__":
    main()
