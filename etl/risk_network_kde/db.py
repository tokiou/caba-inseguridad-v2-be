"""Database access for the risk pipeline (DATABASE_URL, like the Go API)."""

from __future__ import annotations

import json
import os

import psycopg2
import psycopg2.extras
from dotenv import load_dotenv


def connect():
    load_dotenv()
    url = os.environ.get("DATABASE_URL")
    if not url:
        raise SystemExit("DATABASE_URL is not set (copy .env.example to .env)")
    conn = psycopg2.connect(url)
    # The scoring aggregations chew through tens of millions of intermediate
    # rows; give the session room before spilling to disk. Parallel gather is
    # disabled because parallel hash joins allocate /dev/shm segments, and the
    # Postgres container runs with Docker's small default shm size.
    with conn.cursor() as cur:
        cur.execute("SET work_mem = '512MB'")
        cur.execute("SET max_parallel_workers_per_gather = 0")
    conn.commit()
    return conn


def fetch_one(conn, sql: str, params=None):
    with conn.cursor() as cur:
        cur.execute(sql, params or ())
        return cur.fetchone()


def fetch_all(conn, sql: str, params=None):
    with conn.cursor() as cur:
        cur.execute(sql, params or ())
        return cur.fetchall()


def get_graph_version(conn, name: str) -> int:
    row = fetch_one(conn, "SELECT id FROM road_graph_versions WHERE name = %s", (name,))
    if not row:
        raise SystemExit(f"road graph version not found: {name}")
    return row[0]


def get_model(conn, name: str) -> dict:
    row = fetch_one(
        conn,
        """
        SELECT id, name, type, graph_version_id, parameters, train_until, is_active
        FROM risk_model_versions WHERE name = %s
        """,
        (name,),
    )
    if not row:
        raise SystemExit(f"risk model version not found: {name}")
    params = row[4] if isinstance(row[4], dict) else json.loads(row[4])
    return {
        "id": row[0],
        "name": row[1],
        "type": row[2],
        "graph_version_id": row[3],
        "parameters": params,
        "train_until": row[5],
        "is_active": row[6],
    }
