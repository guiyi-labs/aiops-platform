# M93-A：登录页数据真实性与确定性浏览器回归

- Date: 2026-08-09
- Status: Complete
- Scope: 移除登录前未经证实的 12 / 186 / 99 指标，改为能力卡；修复 Playwright 认证夹具与 Dashboard 对比度回归。

## Context

M92 登录页展示了“12 套集群 / 186 个节点 / 99% 准确率”，但这些数字由前端硬编码，
既不来自后端实时数据，也没有版本化证据。登录前新增资源统计接口还会扩大未认证信息面，
因此 M93-A 采用安全优先方案：不公开资源数量，只展示可由产品能力直接证明的事实。

复跑 Playwright 时还发现现有测试注释声称会建立会话，实际却直接访问真实后端，导致
所有受保护路由被 401 重定向；修复夹具后又暴露 Dashboard 三处 WCAG AA 对比度不足。

## What Changed

### 登录页数据真实性

- `frontend/src/views/LoginView.vue`：删除 `useCountUp` 与 12 / 186 / 99 三组硬编码值；
  “平台实时概况”改为“平台核心能力”。
- 三张卡改为可验证的产品能力：多集群治理/统一视图、诊断链路/证据优先、
  变更控制/审计闭环。
- `frontend/src/styles/console-theme.css`：将 `login-stat` / `stat-pip` 语义重命名为
  `login-capability` / `capability-pip`，删除数字单位样式。

### 确定性 Playwright 夹具

- 新增 `frontend/e2e/api-fixtures.ts`：受保护页面使用固定管理员会话与空数据集；
  登录页单独覆盖为空会话，不依赖本机账号、Cookie 或真实后端数据。
- `frontend/e2e/smoke.spec.ts`：登录测试固定验证三项能力，并断言不存在
  12 / 186 / 99 / “实时概况”。
- `frontend/e2e/a11y.spec.ts`：使用同一确定性认证夹具。

### WCAG AA 回归修复

- `frontend/src/styles/base.css`：加深 fleet 汇总与空状态文字颜色。
- `frontend/src/styles/console-theme.css`：加深 ready 状态文字，消除三处低于 4.5:1 的对比度。

## Verification

- `pnpm lint`：通过。
- `pnpm typecheck`：通过。
- `pnpm test`：23 files / 130 tests 全绿。
- `pnpm build`：通过（Vite 7.3.6，1791 modules，8.60s）。
- `pnpm bundle:gate`：通过；entry JS gzip 44.4 kB，largest chunk 21.6 kB，
  total JS gzip 242.0 kB，total CSS gzip 49.5 kB。
- `pnpm test:e2e`：Desktop Chrome + Mobile Chrome 共 28/28 全绿；axe critical/serious = 0；console error = 0。

## Risks / Notes

- M93-A 只关闭“数据真实性 + 浏览器回归确定性”；ParticleNetwork 的 ResizeObserver、
  Page Visibility 暂停、动态 reduced-motion 与低端设备帧耗仍属于 M93-B。
- 若未来确需在登录页显示实时统计，必须先完成未认证信息泄露审查，并为统计口径、
  缓存、失败降级与数据时效提供明确契约。
