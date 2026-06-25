# Benchmark results — 2026-06-23 (commit 37c712a)

Setup: real PostgreSQL+PostGIS+pgRouting (:5434), isolated bench-redis (:6380),
k6 v2.0.0, API on :8090. Load scenarios: 10 VUs × 15s. Single dev machine.

| # | Scenario | reqs | RPS | p95 | p99 | fail% | server-side |
|---|---|---:|---:|---:|---:|---:|---|
| 01 | Sin Redis (baseline) | 97 | 6.0 | 1880 ms | 1939 ms | 0% | ~12 pool acq/req, empty_acquire 10 |
| 02 | Cache **hit** | 76 829 | 5056 | 2.4 ms | 2.8 ms | 0% | hit_rate 1.0, ~2 acq/req (auth only) |
| 03 | Cache **miss** | 110 | 6.8 | 1756 ms | 1836 ms | 0% | hit_rate ~0, recompute every req |
| 04 | Auth flow (5 steps) | 3621 | 236 | 204 ms | 209 ms | 0.03% | bcrypt-bound; safe=cache hit |
| 05 | Login rate limit | 8 | — | 190 ms | 191 ms | 37.5%* | 5×200 then 3×429 |
| 06 | Token revocation | 6 | — | 189 ms | 190 ms | 50%* | all 5 assertions pass |
| 07 | Pool exhaustion | 192 | 5.0 | 7959 ms | 8089 ms | 0% | empty_acquire 2230/2298, ~1035 s cum wait, 0 × 5xx |

\* 429 (scenario 5) and 401 (scenario 6) are the EXPECTED responses; k6 counts
them as http failures but every functional check passed.

## Findings

- **Cache is decisive for /routes/safe.** Hit vs baseline: p95 1880 ms → 2.4 ms
  (~780× faster), 6 → 5056 RPS (~840×). Uncached, each request does ~12 pool
  acquisitions (auth + pgRouting dijkstra×3 + ksp + risk joins); cached, only ~2
  (the auth user lookup). The route computation is genuinely expensive (~1.6–1.9 s),
  which is exactly why the cache matters.
- **Cache miss ≈ baseline** (1756 vs 1880 ms) — the miss path just recomputes; the
  cache layer adds negligible overhead.
- **Pool exhaustion degrades gracefully.** Capping at 5 conns under 40 VUs: 97% of
  acquisitions waited (empty_acquire 2230/2298, ~1035 s cumulative wait), p95
  ballooned to ~8 s — but 0 errors / 0 × 5xx. It queues, it doesn't crash. To bound
  latency (vs bounding errors) add an acquire/query timeout (documented follow-up).
- **Rate limiting & token revocation verified** end-to-end: 5 logins then 429;
  reused (rotated) refresh → 401, post-logout refresh → 401.
