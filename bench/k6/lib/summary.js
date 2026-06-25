// Shared end-of-test summary: write the full k6 metrics JSON to SUMMARY_OUT
// (set by run.sh) plus a compact console line. Scenarios re-export this as
// `export { handleSummary } from './lib/summary.js';`.
export function handleSummary(data) {
  const out = __ENV.SUMMARY_OUT || 'summary.json';
  const m = data.metrics || {};
  const dur = m.http_req_duration ? m.http_req_duration.values : {};
  const reqs = m.http_reqs ? m.http_reqs.values : {};
  const failed = m.http_req_failed ? m.http_req_failed.values : {};

  const line =
    `\n  reqs=${reqs.count || 0} rps=${(reqs.rate || 0).toFixed(1)} ` +
    `p95=${(dur['p(95)'] || 0).toFixed(1)}ms p99=${(dur['p(99)'] || 0).toFixed(1)}ms ` +
    `fail=${((failed.rate || 0) * 100).toFixed(2)}%\n`;

  const result = { stdout: line };
  result[out] = JSON.stringify(data, null, 2);
  return result;
}
