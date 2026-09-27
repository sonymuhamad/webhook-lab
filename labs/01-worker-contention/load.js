// Sends POST /messages at a fixed rate (an open model), so a slow API shows up
// as latency and dropped_iterations instead of silently lowering the rate.
//
//   k6 run -e API_KEY=whl_... load.js
//   k6 run -e API_KEY=whl_... -e RATE=400 -e DURATION=30s load.js
import http from 'k6/http';
import { check } from 'k6';

const api = __ENV.API || 'http://localhost:8080';
const apiKey = __ENV.API_KEY;
if (!apiKey) {
  throw new Error('API_KEY is required: run setup.sh, then source .lab.env');
}

export const options = {
  scenarios: {
    messages: {
      executor: 'constant-arrival-rate',
      rate: Number(__ENV.RATE || 200),
      timeUnit: '1s',
      duration: __ENV.DURATION || '60s',
      // Allocated up front: VUs added mid-run take time to start, and the
      // iterations due in that gap are dropped.
      preAllocatedVUs: Number(__ENV.VUS || 300),
      maxVUs: 1000,
    },
  },
  // A run with dropped iterations did not deliver the requested load, so its
  // numbers do not count.
  thresholds: {
    dropped_iterations: ['count==0'],
    http_req_failed: ['rate==0'],
  },
  summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'p(99)', 'max'],
};

const params = {
  headers: { Authorization: `Bearer ${apiKey}`, 'Content-Type': 'application/json' },
};

export default function () {
  const body = JSON.stringify({
    event_type: 'booking.created',
    payload: { vu: __VU, iteration: __ITER },
  });
  const res = http.post(`${api}/messages`, body, params);
  check(res, { 'status is 202': (r) => r.status === 202 });
}
