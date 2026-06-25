// Scenario 2 — Con Redis, cache HIT. Run the API in cache mode
// (REDIS_ENABLED=true, ROUTE_CACHE_ENABLED=true, RATE_LIMIT_ENABLED=false).
// Fixed coords → every request after the warmup is a hit. Expect low p95 and
// near-flat pgxpool pressure in /debug/stats.
import http from 'k6/http';
import { check } from 'k6';
import { login, authHeaders } from './lib/auth.js';
import { fixedCoords, safeURL } from './lib/config.js';

export { handleSummary } from './lib/summary.js';

export const options = {
  scenarios: {
    load: { executor: 'constant-vus', vus: Number(__ENV.VUS || 10), duration: __ENV.DURATION || '30s' },
  },
};

export function setup() {
  const token = login();
  // Warm the cache so the measured run is all hits.
  http.get(safeURL(fixedCoords()), { headers: authHeaders(token) });
  return { token };
}

export default function (data) {
  const res = http.get(safeURL(fixedCoords()), { headers: authHeaders(data.token) });
  check(res, {
    'status 200': (r) => r.status === 200,
    'X-Cache hit': (r) => r.headers['X-Cache'] === 'hit',
  });
}
