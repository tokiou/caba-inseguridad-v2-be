// Scenario 7 — pgxpool exhaustion. Start the API with a small pool
// (DATABASE_URL=...?pool_max_conns=5) and cache OFF, then ramp VUs well above the
// pool size against an uncached, DB-bound /routes/safe. Watch EmptyAcquireCount
// and AcquireDuration climb in /debug/stats (snapshotted by run.sh). The pass
// criterion is bounded degradation (no crash, no 5xx storm), not low latency.
import http from 'k6/http';
import { check } from 'k6';
import exec from 'k6/execution';
import { login, authHeaders } from './lib/auth.js';
import { uniqueCoords, safeURL } from './lib/config.js';

export { handleSummary } from './lib/summary.js';

export const options = {
  scenarios: {
    ramp: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '10s', target: Number(__ENV.MAX_VUS || 50) },
        { duration: '20s', target: Number(__ENV.MAX_VUS || 50) },
        { duration: '5s', target: 0 },
      ],
    },
  },
};

export function setup() {
  return { token: login() };
}

export default function (data) {
  const res = http.get(safeURL(uniqueCoords(exec.scenario.iterationInTest)),
    { headers: authHeaders(data.token), timeout: '30s' });
  check(res, { 'not 5xx': (r) => r.status < 500 });
}
