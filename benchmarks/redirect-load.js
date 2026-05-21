// k6 script that drives the redirect path under sustained load.
//
// It first creates a small pool of short links, then runs a constant-
// arrival-rate executor against `GET /{code}` for the configured
// duration. The constant-arrival-rate executor is important: it keeps
// throughput steady regardless of latency, so the resulting p99
// numbers reflect server performance, not "k6 backed off".
//
// Run:
//   k6 run benchmarks/redirect-load.js
//   k6 run --env BASE=http://localhost:8080 --env RATE=8000 benchmarks/redirect-load.js
//
// Outputs match the table in README — copy the p95/p99/RPS line into
// the appropriate stage row.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const BASE = __ENV.BASE || 'http://localhost:8080';
const RATE = Number(__ENV.RATE || 5000);          // requests per second
const DURATION = __ENV.DURATION || '60s';
const SEED_LINKS = Number(__ENV.SEED || 200);

const redirectLatency = new Trend('redirect_latency_ms', true);
const seededCounter = new Counter('seeded_links');

export const options = {
  scenarios: {
    constant_load: {
      executor: 'constant-arrival-rate',
      rate: RATE,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: Math.max(50, Math.floor(RATE / 10)),
      maxVUs: Math.max(200, Math.floor(RATE / 2)),
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    redirect_latency_ms: ['p(99)<150', 'p(95)<60'],
  },
};

// setup() runs once before the test starts. We seed SEED_LINKS short
// codes and return them so VUs pick a random one per iteration.
export function setup() {
  const codes = [];
  for (let i = 0; i < SEED_LINKS; i++) {
    const res = http.post(
      `${BASE}/links`,
      JSON.stringify({ url: `https://example.com/origin/${i}` }),
      { headers: { 'Content-Type': 'application/json' } },
    );
    check(res, { 'seed created': r => r.status === 201 });
    if (res.status === 201) {
      const body = res.json();
      codes.push(body.code);
      seededCounter.add(1);
    }
  }
  if (codes.length === 0) {
    throw new Error('failed to seed any links — is the server up?');
  }
  return { codes };
}

export default function (data) {
  const code = data.codes[Math.floor(Math.random() * data.codes.length)];
  const res = http.get(`${BASE}/${code}`, { redirects: 0 });
  check(res, {
    'status is 302': r => r.status === 302,
    'location header is set': r => !!r.headers['Location'],
  });
  redirectLatency.add(res.timings.duration);
}
