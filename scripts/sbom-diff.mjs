#!/usr/bin/env node
// M100-D: deterministic SBOM diff gate.
//
// Compares two syft SPDX JSON documents (baseline, current) and reports added,
// removed and version-changed packages. Exit codes:
//   0 - no material change (or only removals/version changes unless --strict)
//   1 - packages were ADDED (strict/fail-closed mode, the default for CI)
//
// Usage: node sbom-diff.mjs <baseline-spdx.json> <current-spdx.json> [--added-threshold N]
//   --added-threshold 0 (default): fail on any addition.
//   --added-threshold -1: report only, never fail.
// Package identity is "name@version" (SPDX name + versionInfo).
import { readFile } from 'node:fs/promises';

async function loadPackages(path) {
  const doc = JSON.parse(await readFile(path, 'utf8'));
  if (!Array.isArray(doc.packages)) {
    throw new Error(`invalid SPDX document ${path}: missing packages array`);
  }
  return doc.packages
    .filter((p) => p && p.name)
    .map((p) => ({ name: p.name, version: p.versionInfo || '' }))
    .sort((a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : a.version < b.version ? -1 : 1));
}

function key(pkg) {
  return `${pkg.name}@${pkg.version}`;
}

const [baselinePath, currentPath] = process.argv.slice(2);
const thresholdArg = process.argv.indexOf('--added-threshold');
const threshold = thresholdArg === -1 ? 0 : Number(process.argv[thresholdArg + 1] ?? 0);

if (!baselinePath || !currentPath) {
  console.error('usage: sbom-diff.mjs <baseline-spdx.json> <current-spdx.json> [--added-threshold N]');
  process.exit(2);
}

const baseline = await loadPackages(baselinePath);
const current = await loadPackages(currentPath);

const baselineKeys = new Set(baseline.map(key));
const currentKeys = new Set(current.map(key));
const currentByName = new Map();
for (const pkg of current) {
  if (!currentByName.has(pkg.name)) currentByName.set(pkg.name, []);
  currentByName.get(pkg.name).push(pkg);
}

const added = [];
const removed = [];
const changed = [];
for (const pkg of current) {
  if (!baselineKeys.has(key(pkg))) {
    const sameName = baseline.filter((p) => p.name === pkg.name);
    if (sameName.length === 0) {
      added.push(pkg);
    } else if (!sameName.some((p) => p.version === pkg.version)) {
      changed.push({ name: pkg.name, from: sameName.map((p) => p.version).join(','), to: pkg.version });
    }
  }
}
for (const pkg of baseline) {
  if (!currentByName.has(pkg.name)) {
    removed.push(pkg);
  }
}

console.log(`SBOM diff: baseline ${baseline.length} packages, current ${current.length} packages`);
console.log(`  added:   ${added.length}`);
for (const pkg of added) console.log(`    + ${key(pkg)}`);
console.log(`  removed: ${removed.length}`);
for (const pkg of removed) console.log(`    - ${key(pkg)}`);
console.log(`  changed: ${changed.length}`);
for (const pkg of changed) console.log(`    ~ ${pkg.name} ${pkg.from} -> ${pkg.to}`);

if (threshold >= 0 && added.length > threshold) {
  console.error(`SBOM diff gate: ${added.length} package(s) added (threshold ${threshold}) — fail-closed`);
  process.exit(1);
}
console.log(`SBOM diff gate: ok (${added.length} additions <= threshold ${threshold})`);
