#!/usr/bin/env bash
# Benchmark orchestrator. For each scenario it boots the API in the right mode,
# warms up, snapshots /debug/stats, runs the k6 script, snapshots again, and tears
# the API down. Results land in bench/results/<timestamp>/<scenario>/.
#
# Prereqs: go, k6, curl; Postgres (DATABASE_URL) and Redis up.
#
#   bench/run.sh                 # all scenarios
#   bench/run.sh 02 03           # only cache hit + miss
#   VUS=20 DURATION=1m bench/run.sh 02
#
# Env: DATABASE_URL, REDIS_ADDR (default localhost:6379), REDIS_DB (default 0),
#      PORT (default 8090), VUS, DURATION, MAX_VUS (scenario 7), K6_EMAIL/K6_PASSWORD.
set -uo pipefail

cd "$(dirname "$0")/.."   # repo root
ROOT="$(pwd)"

PORT="${PORT:-8090}"
BASE="http://localhost:${PORT}/api/v1"
DATABASE_URL="${DATABASE_URL:-postgres://postgres:postgres@localhost:5434/caba_routes?sslmode=disable}"
REDIS_ADDR="${REDIS_ADDR:-localhost:6379}"
REDIS_DB="${REDIS_DB:-0}"
EMAIL="${K6_EMAIL:-k6@example.com}"
PASSWORD="${K6_PASSWORD:-password123}"

command -v k6 >/dev/null 2>&1 || { echo "k6 not found — install k6 (https://k6.io)"; exit 1; }

TS="$(date +%Y-%m-%dT%H-%M-%S)"
SHA="$(git rev-parse --short HEAD 2>/dev/null || echo nogit)"
OUTDIR="${ROOT}/bench/results/${TS}"
mkdir -p "${OUTDIR}"
echo "results → ${OUTDIR} (commit ${SHA})"

echo "building API binary…"
go build -o "${ROOT}/bench/bin/api" ./cmd/api || { echo "build failed"; exit 1; }

API_PID=""
stop_api() { [ -n "${API_PID}" ] && kill "${API_PID}" 2>/dev/null; API_PID=""; sleep 1; }
trap stop_api EXIT

# start_api ENV_KEY=VAL … — launches the binary with the given extra env, waits /health.
# Uses `env` so KEY=VAL words coming from "$@" are parsed as assignments (a bash
# command would instead try to *run* the first KEY=VAL word).
start_api() {
  env APP_ENV=development HTTP_PORT="${PORT}" METRICS_ENABLED=true \
    REDIS_ADDR="${REDIS_ADDR}" REDIS_DB="${REDIS_DB}" \
    "$@" "${ROOT}/bench/bin/api" >"${OUTDIR}/api.log" 2>&1 &
  API_PID=$!
  for _ in $(seq 1 40); do
    if curl -fsS "${BASE}/health" >/dev/null 2>&1; then return 0; fi
    if ! kill -0 "${API_PID}" 2>/dev/null; then echo "API died on boot (see api.log):"; tail -5 "${OUTDIR}/api.log"; return 1; fi
    sleep 0.5
  done
  echo "API did not become healthy"; return 1
}

register_user() {
  curl -fsS -X POST "${BASE}/auth/register" -H 'Content-Type: application/json' \
    -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}" >/dev/null 2>&1 || true
}

# run_scenario <id> <script> <db_url> ENV_KEY=VAL …
run_scenario() {
  local id="$1" script="$2" dburl="$3"; shift 3
  local dir="${OUTDIR}/${id}"; mkdir -p "${dir}"
  echo ""; echo "▶ scenario ${id}"

  start_api "$@" "DATABASE_URL=${dburl}" || { echo "  skipped (API boot failed)"; return; }
  register_user
  bash "${ROOT}/bench/snapshot_stats.sh" "${BASE}" "${dir}/server_before.json" >/dev/null

  k6 run \
    --summary-trend-stats='avg,min,med,p(90),p(95),p(99),max' \
    -e "BASE_URL=${BASE}" -e "K6_EMAIL=${EMAIL}" -e "K6_PASSWORD=${PASSWORD}" \
    -e "SUMMARY_OUT=${dir}/summary.json" \
    ${VUS:+-e VUS=${VUS}} ${DURATION:+-e DURATION=${DURATION}} ${MAX_VUS:+-e MAX_VUS=${MAX_VUS}} \
    "${ROOT}/bench/k6/${script}" || echo "  (k6 reported threshold/check failures — see summary)"

  bash "${ROOT}/bench/snapshot_stats.sh" "${BASE}" "${dir}/server_after.json" >/dev/null
  stop_api
  echo "  saved → ${dir}"
}

# Scenario → (script, mode env). DATABASE_URL appended with a small pool for #7.
declare -A SCRIPTS=(
  [01]=01_no_redis.js [02]=02_cache_hit.js [03]=03_cache_miss.js [04]=04_auth_flow.js
  [05]=05_login_ratelimit.js [06]=06_token_revocation.js [07]=07_pool_exhaustion.js
)

run_one() {
  case "$1" in
    01) run_scenario 01 "${SCRIPTS[01]}" "${DATABASE_URL}" REDIS_ENABLED=false RATE_LIMIT_ENABLED=false ROUTE_CACHE_ENABLED=false ;;
    02) run_scenario 02 "${SCRIPTS[02]}" "${DATABASE_URL}" REDIS_ENABLED=true  RATE_LIMIT_ENABLED=false ROUTE_CACHE_ENABLED=true ;;
    03) run_scenario 03 "${SCRIPTS[03]}" "${DATABASE_URL}" REDIS_ENABLED=true  RATE_LIMIT_ENABLED=false ROUTE_CACHE_ENABLED=true ;;
    04) run_scenario 04 "${SCRIPTS[04]}" "${DATABASE_URL}" REDIS_ENABLED=true  RATE_LIMIT_ENABLED=false ROUTE_CACHE_ENABLED=true ;;
    05) run_scenario 05 "${SCRIPTS[05]}" "${DATABASE_URL}" REDIS_ENABLED=true  RATE_LIMIT_ENABLED=true  ROUTE_CACHE_ENABLED=false ;;
    06) run_scenario 06 "${SCRIPTS[06]}" "${DATABASE_URL}" REDIS_ENABLED=true  RATE_LIMIT_ENABLED=false ROUTE_CACHE_ENABLED=false ;;
    07)
      local sep='?'; case "${DATABASE_URL}" in *\?*) sep='&';; esac
      run_scenario 07 "${SCRIPTS[07]}" "${DATABASE_URL}${sep}pool_max_conns=5" REDIS_ENABLED=true RATE_LIMIT_ENABLED=false ROUTE_CACHE_ENABLED=false ;;
    *) echo "unknown scenario '$1' (use 01..07)";;
  esac
}

SCENARIOS=("$@")
[ ${#SCENARIOS[@]} -eq 0 ] && SCENARIOS=(01 02 03 04 05 06 07)
for s in "${SCENARIOS[@]}"; do run_one "${s}"; done

echo ""; echo "done. Compare with: jq '.metrics.http_req_duration.values' ${OUTDIR}/*/summary.json"
