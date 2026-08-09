# M93-B2：登录页性能预算与证据闭环

- Date: 2026-08-10
- Status: Complete
- Scope: 建立登录页专属体积统计、三模式性能采样、版本化基线 JSON 与报告模式 CI 探针。

## Context

M93-C 已把登录页扩展为全屏 Canvas2D 粒子 + 拓扑视觉，但缺少独立的登录页性能预算和可复核证据。
M92 与 M93-B1.1 期间声明的"低端设备不卡顿"、reduced-motion 静态帧等行为没有结构化采样基线；
全局 bundle 门禁不能代表登录页专属体积。

M93-B2 的目标不是新增动画，而是把登录页的视觉成本数字化：
- 登录页专属 JS/CSS/Canvas 体积；
- Desktop / Mobile / reduced-motion 三种模式的真实运行指标；
- 可版本化的性能基线和跨环境复现脚本。

## What Changed

### 新脚本：登录专属 bundle 体积分析

- `frontend/scripts/login-bundle-analyze.mjs`：读取 `dist/assets`，计算
  `LoginView-*.js`（包含 `ParticleNetwork.vue` 与 Canvas 逻辑）原始/gzip 体积；
  同时报告 entry JS/CSS 以及登录 CSS 规则在全局 stylesheet 中的字节占比。
- 输出 `.artifacts/login-perf/login-bundle.json`。

### 新脚本：Playwright 登录性能采样

- `frontend/scripts/login-perf-sample.mjs`：spawn 生产 preview 服务器，headless Chromium 采样三模式
  （desktop-normal 1440x900/no-pref、mobile-degraded 390x844/DSF3/no-pref、reduced-motion
  1440x900/reduce），每种模式 3 次（可用 `LOGIN_PERF_REPEATS` 调整）。
- 每次遍历记录：navigation timing、FCP/LCP、long-task count/duration/max、rAF 帧率/最大间隔、
  交互延迟（focus→两帧）、Canvas DPR/粒子数/拓扑元素数、JS heap、hidden+restore 行为、console error。
- 支持 `LOGIN_PERF_STRICT=1` 在失败时非零退出；默认报告模式（有失败也不阻塞 CI）。
- 输出 `.artifacts/login-perf/login-samples.json`。

### 新脚本：基线报告生成

- `frontend/scripts/login-perf-report.mjs`：读取 samples + bundle 数据，计算 min/max/mean/median/p90，
  按 `max(median×1.4, max×1.15)` 生成报告模式预算阈值，写出
  `.artifacts/login-perf/login-baseline-v1.json`（schema 1）与
  `.artifacts/login-perf/login-perf-report.md`。

### package.json

- 新增 `perf:bundle`、`perf:sample`、`perf:report`、`perf:login` 脚本。

### CI 集成（报告模式）

- `.github/workflows/ci.yml` frontend job 在 build 后执行：
  - `npx playwright install chromium`（安装 headless browser）
  - `pnpm perf:login`（采样 + 体积 + 报告）
  - upload `frontend/.artifacts/login-perf/` 为 CI artifact（保留 7 天，失败不阻塞）。

## 实测基线（v1，schema 1）

> 环境：Windows 11 + Node v24.18.0 + headless Chromium（Playwright chromium-1234）。
> 采样 9 visits（3 模式 × 3 次），0 失败。产物为 `.artifacts/login-perf/login-samples.json`。

### 体积

| 项 | 原始 | Gzip |
|---|---|---|
| LoginView JS chunk | 14.75 kB | 5.62 kB |
| Entry JS | 109.04 kB | 42.13 kB |
| Entry CSS（全局） | 146.6 kB | 28.06 kB |
| 登录 CSS 规则字节占比 | 4.1%（约 6.05 kB 原始） | — |

### 性能按路径（median）

| 指标 | desktop-normal | mobile-degraded | reduced-motion |
|---|---|---|---|
| DCL | ~47 ms | ~40 ms | ~39 ms |
| FCP/LCP | ~336 ms | ~188 ms | ~308 ms |
| Long-task | 0 | 0 | 0 |
| avgFrameMs | ~68 ms（~15fps） | 16.6 ms（~60fps） | 17.1 ms |
| maxFrameGapMs | ~69–137 ms | ~18 ms | ~39–41 ms |
| interactionLatency | ~124 ms | ~24 ms | ~31 ms |
| Canvas particles | 90 | 21 | 90 |
| Canvas DPR | 1 | 1.5 | 1 |
| memoryHeap | 10 MB | 10 MB | 10 MB |

### 不变量（3 模式全部 ✅）

- Canvas 存在且粒子数 > 0；DPR ≤ 2。
- 非 reduced-motion 模式 `data-reduced-motion=false` 且动画运行中。
- reduced-motion 模式 `data-reduced-motion=true` 且 `data-running=false`（静态帧）。
- 页面隐藏时动画暂停，恢复后按预期继续（reduced-motion 除外，保持停止）。
- 全路径 console error = 0。

## Verification

- `pnpm typecheck`：通过。
- `pnpm lint`：通过。
- `pnpm build`：通过（Vite 1791 modules / 7.35s；LoginView JS 15.10 kB raw）。
- `pnpm bundle:gate`：通过（entry JS gzip 42.13 kB，总 JS gzip 247 kB，总 CSS gzip 53 kB）。
- `pnpm perf:login`：成功（9 visits，0 failures）。
- 基线 JSON：`.artifacts/login-perf/login-baseline-v1.json`。
- 报告 Markdown：`.artifacts/login-perf/login-perf-report.md`。
- `git diff --check`：通过。

## Risks / Notes

- 桌面 normal 路径 avgFrameMs ≈ 68ms（headless software rendering 下的 rAF 频率），不代表真实
  GPU 环境同等数值；报告与基线 JSON 已记录环境和采样条件，可在 CI ubuntu runner 上对比。
- 预算为**报告模式**，不阻塞 CI；连续两个稳定 CI 周期后可升级为 fail-closed 门禁。
- 登录 CSS 仍存在全局 `index-*.css` 中，登录样式与其余 34 个视图混合。当前报告给出
  "entry CSS（全部）+ 登录规则字节占比" 两层统计，诚实反映未拆分 CSS 的结构事实。
- 体积与运行时基线的绝对阈值由本次实测得出，未凭主观猜测设定。