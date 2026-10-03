// Three tenant groups on one timeline. The quiet tenants send for the whole
// run, so their delivery lag shows what the other two groups do to them.
//
//   0:00 ─ quiet ─────────────────────────────────────────────────── 13:00
//          1:00 bulk (20 s)
//                                            10:00 slow ──── 12:00
//
//   k6 run -e QUIET_KEYS=... -e BULK_KEY=... -e SLOW_KEY=... load.js
//
// Every rate, start, and duration can be overridden with -e, for example
// -e BULK_RATE=200 -e BULK_DURATION=50s.
import http from 'k6/http';
import { check } from 'k6';

const api = __ENV.API || 'http://localhost:8080';
const quietKeys = (__ENV.QUIET_KEYS || '').split(',').filter(Boolean);
const bulkKey = __ENV.BULK_KEY;
const slowKey = __ENV.SLOW_KEY;
if (quietKeys.length === 0 || !bulkKey || !slowKey) {
  throw new Error('QUIET_KEYS, BULK_KEY and SLOW_KEY are required: run setup.sh, then source .lab.env');
}

const env = (name, fallback) => __ENV[name] || fallback;

// Open model, as in the earlier labs: a slow API shows up as latency and
// dropped_iterations instead of silently lowering the rate.
function arrival(exec, rate, startTime, duration, vus) {
  return {
    executor: 'constant-arrival-rate',
    exec,
    rate: Number(rate),
    timeUnit: '1s',
    startTime,
    duration,
    preAllocatedVUs: Number(vus),
    maxVUs: Number(vus) * 2,
  };
}

export const options = {
  scenarios: {
    quiet: arrival('quiet', env('QUIET_RATE', 1000), env('QUIET_START', '0s'), env('QUIET_DURATION', '13m'), 300),
    bulk: arrival('bulk', env('BULK_RATE', 500), env('BULK_START', '1m'), env('BULK_DURATION', '20s'), 400),
    slow: arrival('slow', env('SLOW_RATE', 10), env('SLOW_START', '10m'), env('SLOW_DURATION', '2m'), 10),
  },
  thresholds: {
    dropped_iterations: ['count==0'],
    http_req_failed: ['rate==0'],
  },
  summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'p(99)', 'max'],
};

function send(key, group) {
  const res = http.post(
    `${api}/messages`,
    JSON.stringify({ event_type: 'booking.created', payload: { group, vu: __VU, iteration: __ITER } }),
    { headers: { Authorization: `Bearer ${key}`, 'Content-Type': 'application/json' }, tags: { group } },
  );
  check(res, { 'status is 202': (r) => r.status === 202 });
}

export function quiet() {
  send(quietKeys[Math.floor(Math.random() * quietKeys.length)], 'quiet');
}

export function bulk() {
  send(bulkKey, 'bulk');
}

export function slow() {
  send(slowKey, 'slow');
}
