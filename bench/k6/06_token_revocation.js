// Scenario 6 — token revocation + refresh rotation (functional). Run with rate
// limiting OFF. Verifies: a reused (rotated) refresh token is rejected, and
// refresh after logout is rejected.
//
// NOTE (by design): access tokens are stateless — logout revokes the *refresh
// session*, not the access token (it stays valid until its 15-min exp). This
// scenario asserts refresh behavior, not instant access-token death.
import http from 'k6/http';
import { check } from 'k6';
import { BASE, EMAIL, PASSWORD } from './lib/config.js';

export { handleSummary } from './lib/summary.js';

export const options = { vus: 1, iterations: 1 };

export function setup() {
  http.post(`${BASE}/auth/register`, JSON.stringify({ email: EMAIL, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' } });
}

export default function () {
  let res = http.post(`${BASE}/auth/login`, JSON.stringify({ email: EMAIL, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' } });
  check(res, { 'login 200': (r) => r.status === 200 });

  // Capture the current refresh cookie before rotating.
  const jar = http.cookieJar();
  const cookies = jar.cookiesForURL(`${BASE}/auth/refresh`);
  const oldRefresh = cookies.refresh_token ? cookies.refresh_token[0] : null;

  // Rotate: refresh succeeds and issues a new cookie (jar updates).
  res = http.post(`${BASE}/auth/refresh`, null);
  check(res, { 'refresh 200': (r) => r.status === 200 });

  // Reuse the OLD (now rotated/revoked) refresh token → must be 401.
  if (oldRefresh) {
    res = http.post(`${BASE}/auth/refresh`, null, { cookies: { refresh_token: oldRefresh } });
    check(res, { 'reused old refresh 401': (r) => r.status === 401 });
  }

  // Logout revokes the current session; refresh afterwards → 401.
  res = http.post(`${BASE}/auth/logout`, null);
  check(res, { 'logout 200': (r) => r.status === 200 });
  res = http.post(`${BASE}/auth/refresh`, null);
  check(res, { 'refresh after logout 401': (r) => r.status === 401 });
}
