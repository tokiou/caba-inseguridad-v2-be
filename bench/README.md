# Benchmark suite

Measures latency, throughput, errors, **pgxpool pressure**, and **cache effectiveness** across the
runtime modes and the key flows. Two layers:

- **Client (k6)** — `http_req_duration` (p50/p90/p95/p99), `http_reqs` (RPS), `http_req_failed`, 429s,
  and the `X-Cache: hit|miss` header on `/routes/safe`.
- **Server (`GET /api/v1/debug/stats`)** — pgxpool (`acquired/idle/total/max`, `acquire_count`,
  `acquire_duration_ms`, `empty_acquire_count`, ...) and cache (`hits/misses/errors/sets/hit_rate`),
  snapshotted before/after each run. Requires `METRICS_ENABLED=true` and is loopback-only.

`run.sh` boots the API in the right mode per scenario, warms up, snapshots, runs the k6 script,
snapshots again, tears down, and writes everything under `results/<timestamp>/<scenario>/`.

## Prereqs

```bash
go, k6 (https://k6.io), curl, jq    # tools
docker compose up -d postgres redis # Postgres (DATABASE_URL) + Redis up
```

## Run

```bash
bench/run.sh                # all 7 scenarios
bench/run.sh 02 03          # only cache hit + miss (the with/without-cache comparison)
VUS=20 DURATION=1m bench/run.sh 02
MAX_VUS=80 bench/run.sh 07  # pool exhaustion ramp target
```

Env: `DATABASE_URL`, `REDIS_ADDR` (default `localhost:6379`), `REDIS_DB`, `PORT` (default `8090`),
`VUS`, `DURATION`, `MAX_VUS`, `K6_EMAIL`, `K6_PASSWORD`. The bench user is registered idempotently.

## Scenarios

| # | What | Mode (REDIS / RATE / CACHE) | Proves |
|---|---|---|---|
| 01 | Sin Redis (baseline) | false / false / false | raw DB cost (control) |
| 02 | Cache **hit** | true / false / true | cache-served path (fixed coords) |
| 03 | Cache **miss** | true / false / true | miss overhead (unique coords) |
| 04 | Full auth flow | true / false / true | login→me→safe→refresh→logout latency + rotation |
| 05 | Login rate limit | true / **true** / false | 429 after 5/min |
| 06 | Token revocation | true / false / false | refresh reuse + post-logout refresh → 401 |
| 07 | Pool exhaustion | true / false / false, `pool_max_conns=5` | bounded degradation under saturation |

## Reading results

```bash
# with vs without cache — compare p95/p99 latency
jq '.metrics.http_req_duration.values | {p95:."p(95)", p99:."p(99)"}' results/*/0{1,2,3}/summary.json

# cache effectiveness + pool pressure for a run
jq '.cache, .pgxpool' results/<ts>/02/server_after.json
```

Expected shape: **02 (hit)** p95 ≪ **01 (baseline)** with `empty_acquire_count`/`acquired_conns`
barely moving (DB untouched); **03 (miss)** ≈ baseline + a `SET`; **07** shows `empty_acquire_count`
and `acquire_duration_ms` climbing.

## Notes

- **Scenario 6 (by design):** access tokens are stateless — logout revokes the *refresh session*, not
  the access token (valid until its 15-min `exp`). The test asserts refresh behavior, not instant
  access-token death.
- **Rigor:** discard the warmup, run ≥3 times, keep coords/dataset fixed, report percentiles. `run.sh`
  records the git SHA in the results dir.
- Results are committed so runs are diffable across commits (regression tracking).
