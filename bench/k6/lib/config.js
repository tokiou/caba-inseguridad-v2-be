// Shared config + helpers for the benchmark scenarios.
export const BASE = __ENV.BASE_URL || 'http://localhost:8080/api/v1';
export const EMAIL = __ENV.K6_EMAIL || 'k6@example.com';
export const PASSWORD = __ENV.K6_PASSWORD || 'password123';

// fixedCoords: identical every call → cache hits (after warmup).
export function fixedCoords() {
  return { origin_lat: -34.60, origin_lng: -58.38, dest_lat: -34.61, dest_lng: -58.37 };
}

// uniqueCoords: jitter the destination by a few 5th-decimals keyed by `seed`, so
// the cache key never repeats (forces misses / DB work) while staying on a small
// (~220 m) on-graph patch of central CABA. ~40k distinct keys.
export function uniqueCoords(seed) {
  const a = (seed % 200) * 0.00001;
  const b = (Math.floor(seed / 200) % 200) * 0.00001;
  return { origin_lat: -34.60, origin_lng: -58.38, dest_lat: -34.61 - a, dest_lng: -58.37 - b };
}

export function safeURL(c) {
  return `${BASE}/routes/safe?origin_lat=${c.origin_lat}&origin_lng=${c.origin_lng}` +
    `&dest_lat=${c.dest_lat}&dest_lng=${c.dest_lng}`;
}
