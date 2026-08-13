#!/usr/bin/env node
/**
 * ui-gate.mjs — Track A 前端全量门禁一键运行
 *
 * 按顺序执行：
 *   1. CSS token 审计 (--check: replaceable=0 才通过)
 *   2. 截图基线 --verify（需前端已部署到 BASE_URL）
 *   3. axe 无障碍审计（32 视图 × 2 视口，0 critical/serious/0 app errors）
 *   4. 前端 bundle gate（entry JS gzip 上限）
 *
 * 任一步骤非零退出 → 立即中止并报 FAIL。
 *
 * 用法：node scripts/ui-gate.mjs
 * 环境：AIOPS_BASE_URL 默认 http://127.0.0.1:18080（前端需已 docker cp 部署）
 */

import { execSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const __dirname = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(__dirname, '..')

const steps = [
  { name: 'CSS tokens', cmd: 'node scripts/audit-css-tokens.mjs --check', cwd: repoRoot },
  { name: 'Baselines',  cmd: 'node scripts/capture-ui-baselines.mjs --verify', cwd: repoRoot },
  { name: 'axe a11y',   cmd: 'node scripts/audit-a11y-axe.mjs', cwd: repoRoot },
  { name: 'Bundle',     cmd: 'node scripts/bundle-gate.mjs', cwd: path.join(repoRoot, 'frontend') },
]

let passed = 0
const total = steps.length

for (const { name, cmd, cwd } of steps) {
  console.log(`\n▸ [${passed + 1}/${total}] ${name} — ${cmd}`)
  try {
    execSync(cmd, { cwd, stdio: 'inherit', timeout: 300_000 })
    console.log(`✓ ${name} PASS`)
    passed++
  } catch {
    console.error(`✗ ${name} FAIL — gate aborted`)
    process.exit(1)
  }
}

console.log(`\n=== UI GATE PASS: ${passed}/${total} ===`)
