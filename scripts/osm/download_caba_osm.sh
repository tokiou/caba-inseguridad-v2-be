#!/usr/bin/env bash
#
# Download the CABA OpenStreetMap extract used to build the walkable road graph.
# Offline build input — the Go API never reads this file directly.
#
# Usage:
#   scripts/osm/download_caba_osm.sh [--force]
#
# Idempotent: skips the download if data/osm/caba.osm.pbf already exists, unless
# --force is passed.
set -euo pipefail

OSM_URL="https://download.openstreetmap.fr/extracts/south-america/argentina/buenos_aires_city-latest.osm.pbf"
OUT_DIR="data/osm"
OUT_FILE="${OUT_DIR}/caba.osm.pbf"

FORCE=0
if [[ "${1:-}" == "--force" ]]; then
  FORCE=1
fi

mkdir -p "${OUT_DIR}"

if [[ -f "${OUT_FILE}" && "${FORCE}" -eq 0 ]]; then
  echo "==> ${OUT_FILE} already exists ($(du -h "${OUT_FILE}" | cut -f1)); skipping (use --force to re-download)."
  exit 0
fi

echo "==> Downloading CABA OSM extract -> ${OUT_FILE}"
wget -O "${OUT_FILE}" "${OSM_URL}"
echo "==> Done: $(du -h "${OUT_FILE}" | cut -f1)"
