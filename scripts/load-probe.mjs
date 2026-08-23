#!/usr/bin/env node
// load-probe.mjs — functional-level HTTP latency probe for the local AIOps
// compose stack. NOT a production benchmark: sequential-per-worker requests,
// loopback network, dev-size dataset. Output is a lower bound reference for
// the thesis experiment chapter (same honesty framing as experiment-summary).
//
// Usage:
//   node scripts/load-probe.mjs \
//     --base http://127.0.0.1:8080 \
//     --user admin --password "$AIOPS_ADMIN_PASSWORD" \
//     --path "/api/v1/clusters?limit=10" --path "/api/v1/diagnoses?limit=10" \
//     --levels 1,4,16,64 --total 240 [--json out.json]
import { parseArgs } from "node:util";
import { writeFileSync, mkdirSync } from "node:fs";
import { dirname } from "node:path";

const args = parseArgs({
  args: process.argv.slice(2),
  options: {
    base: { type: "string", default: "http://127.0.0.1:8080" },
    user: { type: "string", default: "admin" },
    password: { type: "string", default: process.env.AIOPS_ADMIN_PASSWORD ?? "" },
    path: { type: "string", multiple: true, default: [] },
    levels: { type: "string", default: "1,4,16,64" },
    total: { type: "string", default: "240" },
    json: { type: "string", default: "" },
  },
});

const base = args.values.base.replace(/\/$/, "");
const paths = args.values.path.length > 0 ? args.values.path : ["/api/v1/clusters?limit=10"];
const levels = args.values.levels.split(",").map((n) => parseInt(n, 10)).filter((n) => n > 0);
const total = parseInt(args.values.total, 10);

if (!args.values.password) {
  console.error("load-probe: missing --password / AIOPS_ADMIN_PASSWORD");
  process.exit(2);
}

async function login() {
  const res = await fetch(`${base}/api/v1/auth/login`, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ username: args.values.user, password: args.values.password }),
  });
  if (!res.ok) throw new Error(`login failed: ${res.status}`);
  const body = await res.json();
  if (!body.access_token) throw new Error("login returned no access_token");
  return body.access_token;
}

function percentile(sorted, p) {
  if (sorted.length === 0) return NaN;
  const idx = Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1);
  return sorted[Math.max(0, idx)];
}

async function probePath(token, path, concurrency, totalRequests) {
  // warmup: 20 sequential requests to populate caches before measurement
  for (let i = 0; i < 20; i++) {
    await fetch(`${base}${path}`, { headers: { authorization: `Bearer ${token}` } }).catch(() => {});
  }
  const latencies = [];
  let errors = 0;
  const perWorker = Math.max(1, Math.floor(totalRequests / concurrency));
  async function worker() {
    for (let i = 0; i < perWorker; i++) {
      const t0 = performance.now();
      try {
        const res = await fetch(`${base}${path}`, {
          headers: { authorization: `Bearer ${token}` },
        });
        if (!res.ok) errors++;
        await res.arrayBuffer();
      } catch {
        errors++;
      }
      latencies.push(performance.now() - t0);
    }
  }
  const t0 = performance.now();
  await Promise.all(Array.from({ length: concurrency }, worker));
  const wallMs = performance.now() - t0;
  latencies.sort((a, b) => a - b);
  const done = latencies.length;
  return {
    path,
    concurrency,
    requests: done,
    errors,
    wall_ms: Math.round(wallMs),
    rps: Math.round((done / wallMs) * 1000),
    p50_ms: +percentile(latencies, 50).toFixed(2),
    p95_ms: +percentile(latencies, 95).toFixed(2),
    p99_ms: +percentile(latencies, 99).toFixed(2),
    max_ms: +latencies[latencies.length - 1].toFixed(2),
  };
}

const token = await login();
console.log(`aiops load-probe — ${base} · paths=${paths.length} · levels=[${levels}] · total≈${total}/level\n`);
const report = { tool: "scripts/load-probe.mjs", generated_at: new Date().toISOString(), base, note: "local compose stack, loopback, functional-level reference — not a production benchmark", results: [] };

for (const path of paths) {
  console.log(`▶ ${path}`);
  for (const c of levels) {
    const r = await probePath(token, path, c, total);
    report.results.push(r);
    console.log(
      `   c=${String(c).padStart(3)}  p50=${String(r.p50_ms).padStart(7)}ms  p95=${String(r.p95_ms).padStart(8)}ms  p99=${String(r.p99_ms).padStart(8)}ms  max=${String(r.max_ms).padStart(8)}ms  rps=${String(r.rps).padStart(5)}  err=${r.errors}`,
    );
  }
  console.log("");
}

if (args.values.json) {
  mkdirSync(dirname(args.values.json), { recursive: true });
  writeFileSync(args.values.json, JSON.stringify(report, null, 2) + "\n");
  console.log(`report written to ${args.values.json}`);
}
