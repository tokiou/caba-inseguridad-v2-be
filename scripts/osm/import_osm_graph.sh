#!/usr/bin/env bash
#
# Import the CABA walkable graph from OSM into raw osm2pgrouting tables.
#
#   data/osm/caba.osm.pbf  --osmium-->  caba.osm  --osm2pgrouting-->  osm_ways / osm_ways_vertices_pgr
#
# osm2pgrouting 2.3.8 reads OSM XML (not .pbf), so the .pbf is first converted
# with osmium. Tables are created with the `osm_` prefix so they never collide
# with the internal road_nodes / road_edges (populated later by
# normalize_osm_graph.sql). This is an offline build step.
#
# Credentials are read from the environment (never hardcoded). Required:
#   POSTGRES_HOST POSTGRES_PORT POSTGRES_DB POSTGRES_USER POSTGRES_PASSWORD
# These are loaded from .env if present.
#
# By default the OSM tools run inside a throwaway Ubuntu container on the host
# network (no host install needed). Set OSM_IMPORT_NATIVE=1 to use osm2pgrouting
# and osmium already installed on the host instead.
#
# Usage:
#   scripts/osm/import_osm_graph.sh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${REPO_ROOT}"

PBF_FILE="data/osm/caba.osm.pbf"
OSM_FILE="data/osm/caba.osm"
CONF_FILE="scripts/osm/mapconfig_foot.xml"
TOOLS_IMAGE="${OSM_TOOLS_IMAGE:-ubuntu:22.04}"

# --- Load .env (POSTGRES_* + credentials) without clobbering existing env -----
if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

: "${POSTGRES_HOST:?POSTGRES_HOST is required}"
: "${POSTGRES_PORT:?POSTGRES_PORT is required}"
: "${POSTGRES_DB:?POSTGRES_DB is required}"
: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}"

if [[ ! -f "${PBF_FILE}" ]]; then
  echo "ERROR: ${PBF_FILE} not found. Run scripts/osm/download_caba_osm.sh first." >&2
  exit 1
fi
if [[ ! -f "${CONF_FILE}" ]]; then
  echo "ERROR: ${CONF_FILE} not found." >&2
  exit 1
fi

echo "==> Target DB: ${POSTGRES_USER}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}"

run_import() {
  # Convert .pbf -> .osm XML (osm2pgrouting cannot read .pbf), then import.
  echo "==> Converting ${PBF_FILE} -> ${OSM_FILE}"
  osmium cat "${PBF_FILE}" -o "${OSM_FILE}" -f osm --overwrite

  echo "==> Running osm2pgrouting (foot profile) into osm_ways / osm_ways_vertices_pgr"
  osm2pgrouting \
    -f "${OSM_FILE}" \
    -c "${CONF_FILE}" \
    --prefix osm_ \
    -d "${POSTGRES_DB}" \
    -U "${POSTGRES_USER}" \
    -W "${POSTGRES_PASSWORD}" \
    -h "${POSTGRES_HOST}" \
    -p "${POSTGRES_PORT}" \
    --clean
}

if [[ "${OSM_IMPORT_NATIVE:-0}" == "1" ]]; then
  echo "==> Using native osm2pgrouting / osmium"
  run_import
else
  echo "==> Using throwaway ${TOOLS_IMAGE} container (set OSM_IMPORT_NATIVE=1 to use host tools)"
  docker run --rm --network host \
    -v "${REPO_ROOT}/data/osm:/work/data/osm" \
    -v "${REPO_ROOT}/scripts/osm:/work/scripts/osm:ro" \
    -e POSTGRES_HOST -e POSTGRES_PORT -e POSTGRES_DB -e POSTGRES_USER -e POSTGRES_PASSWORD \
    -w /work \
    "${TOOLS_IMAGE}" bash -c '
      set -euo pipefail
      export DEBIAN_FRONTEND=noninteractive
      apt-get update -qq >/dev/null
      apt-get install -y -qq osm2pgrouting osmium-tool >/dev/null
      echo "==> Converting data/osm/caba.osm.pbf -> data/osm/caba.osm"
      osmium cat data/osm/caba.osm.pbf -o data/osm/caba.osm -f osm --overwrite
      echo "==> Running osm2pgrouting (foot profile)"
      osm2pgrouting \
        -f data/osm/caba.osm \
        -c scripts/osm/mapconfig_foot.xml \
        --prefix osm_ \
        -d "${POSTGRES_DB}" -U "${POSTGRES_USER}" -W "${POSTGRES_PASSWORD}" \
        -h "${POSTGRES_HOST}" -p "${POSTGRES_PORT}" \
        --clean
    '
fi

echo "==> Import done. Next: psql \"\$DATABASE_URL\" -f scripts/osm/normalize_osm_graph.sql"
