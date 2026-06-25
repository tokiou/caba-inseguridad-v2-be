#!/usr/bin/env bash
# Snapshot server-side stats into a JSON file.
#   snapshot_stats.sh <base_url> <out_file>
# Captures GET /debug/stats (pgxpool + cache + runtime). If psql + DATABASE_URL
# are available, also appends a pg_stat_activity connection-state count.
set -euo pipefail

BASE="${1:-http://localhost:8080/api/v1}"
OUT="${2:-stats.json}"

curl -fsS "${BASE}/debug/stats" -o "${OUT}" 2>/dev/null || {
  echo '{"error":"could not reach /debug/stats (METRICS_ENABLED=true and loopback?)"}' > "${OUT}"
}

# Optional DB-side view (best-effort).
if command -v psql >/dev/null 2>&1 && [ -n "${DATABASE_URL:-}" ]; then
  psql "${DATABASE_URL}" -At -c \
    "select state, count(*) from pg_stat_activity group by state order by 1;" \
    > "${OUT}.pg_stat_activity.txt" 2>/dev/null || true
fi

echo "wrote ${OUT}"
