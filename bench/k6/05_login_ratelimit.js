// Scenario 5 — login rate limiting. Run with rate limiting ON (REDIS_ENABLED=true,
// RATE_LIMIT_ENABLED=true). Burst POST /auth/login from one IP; the 5/min limit
// should yield 429 after the 5th. Asserts the limiter sheds load.
import http from 'k6/http';
import { check } from 'k6';
import { Counter } from 'k6/metrics';
import { BASE, EMAIL, PASSWORD } from './lib/config.js';

export { handleSummary } from './lib/summary.js';

const login429 = new Counter('login_429');

export const options = {
  vus: 1,
  iterations: 8, // 5/min limit → the 6th+ should 429
  thresholds: { login_429: ['count>=1'] },
};

export default function () {
  const res = http.post(`${BASE}/auth/login`, JSON.stringify({ email: EMAIL, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' } });
  if (res.status === 429) login429.add(1);
  check(res, { '200 or 429': (r) => r.status === 200 || r.status === 429 });
  console.log(`login status=${res.status}`);
}
