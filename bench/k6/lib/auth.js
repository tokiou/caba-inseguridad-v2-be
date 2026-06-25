import http from 'k6/http';
import { BASE, EMAIL, PASSWORD } from './config.js';

// login returns an access token; throws if the bench user is not registered
// (run.sh registers it idempotently before each run).
export function login(email = EMAIL, password = PASSWORD) {
  const res = http.post(`${BASE}/auth/login`, JSON.stringify({ email, password }), {
    headers: { 'Content-Type': 'application/json' },
  });
  if (res.status !== 200) {
    throw new Error(`login failed (${res.status}); register ${email} first (run.sh does this)`);
  }
  return res.json('access_token');
}

export function authHeaders(token) {
  return { Authorization: `Bearer ${token}` };
}
