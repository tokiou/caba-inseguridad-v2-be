// Scenario 4 — full auth flow per iteration: login → me → safe → refresh →
// logout. Run with rate limiting OFF (else the shared-IP login burst hits the
// 5/min limit). Measures end-to-end chain latency and that refresh rotates.
import http from 'k6/http';
import { check, group } from 'k6';
import { BASE, EMAIL, PASSWORD } from './lib/config.js';
import { fixedCoords, safeURL } from './lib/config.js';

export { handleSummary } from './lib/summary.js';

export const options = {
  scenarios: {
    flow: { executor: 'constant-vus', vus: Number(__ENV.VUS || 5), duration: __ENV.DURATION || '30s' },
  },
};

export function setup() {
  // Idempotent register (409 if already there — fine).
  http.post(`${BASE}/auth/register`, JSON.stringify({ email: EMAIL, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' } });
}

export default function () {
  let token;
  group('login', () => {
    const res = http.post(`${BASE}/auth/login`, JSON.stringify({ email: EMAIL, password: PASSWORD }),
      { headers: { 'Content-Type': 'application/json' } });
    check(res, { 'login 200': (r) => r.status === 200 });
    token = res.json('access_token');
  });
  group('me', () => {
    const res = http.get(`${BASE}/auth/me`, { headers: { Authorization: `Bearer ${token}` } });
    check(res, { 'me 200': (r) => r.status === 200 });
  });
  group('safe', () => {
    const res = http.get(safeURL(fixedCoords()), { headers: { Authorization: `Bearer ${token}` } });
    check(res, { 'safe 200/429': (r) => r.status === 200 || r.status === 429 });
  });
  group('refresh', () => {
    const res = http.post(`${BASE}/auth/refresh`, null);
    check(res, { 'refresh 200': (r) => r.status === 200 });
  });
  group('logout', () => {
    const res = http.post(`${BASE}/auth/logout`, null);
    check(res, { 'logout 200': (r) => r.status === 200 });
  });
}
