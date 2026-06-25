// Scenario 3 — Con Redis, cache MISS. Same cache mode as #2 but UNIQUE coords
// per iteration → the key never repeats → every request misses and recomputes
// (plus a SET). Measures the overhead of the miss path vs baseline (#1).
import http from 'k6/http';
import { check } from 'k6';
import exec from 'k6/execution';
import { login, authHeaders } from './lib/auth.js';
import { uniqueCoords, safeURL } from './lib/config.js';

export { handleSummary } from './lib/summary.js';

export const options = {
  scenarios: {
    load: { executor: 'constant-vus', vus: Number(__ENV.VUS || 10), duration: __ENV.DURATION || '30s' },
  },
};

export function setup() {
  return { token: login() };
}

export default function (data) {
  const res = http.get(safeURL(uniqueCoords(exec.scenario.iterationInTest)), { headers: authHeaders(data.token) });
  check(res, {
    'status 200': (r) => r.status === 200,
    'X-Cache miss': (r) => r.headers['X-Cache'] === 'miss',
  });
}
