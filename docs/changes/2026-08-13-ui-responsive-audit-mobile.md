# UI 响应式审计（Track A · ≤720px 可用性 · 首批 4 页 + 登录页）

- Date: 2026-08-13
- Status: Complete
- Scope: 前端关键页面在 375px 移动视口的可用性审计（证据型，无代码改动）

## Context

Track A「响应式审计」要求 35 个视图在 ≤720px 的可用性（表格横向滚动、工具栏折叠、
抽屉全屏化）。本改动对已纳入截图基线的 5 个视图（登录页 + `/`、`/clusters`、
`/workloads`、`/diagnoses`）在 375×812 移动视口做 CDP 实测审计，产出证据并归档。

## Audit Method

- headless Chrome CDP：`Emulation.setDeviceMetricsOverride` 375×812 mobile:true，
  以 admin/admin123 登录后逐页导航。
- 断言维度：
  1. `documentElement.scrollWidth > clientWidth` → 页面级横向溢出；
  2. 任意内部元素 `scrollWidth > clientWidth`（投影到屏外）→ 溢出容器；
  3. 所有 `button/a/[role=tab]/select/input/textarea` 的 `getBoundingClientRect()`
     出屏（right > innerWidth 或 left < 0）→ 交互可点性裁切。

## Findings

| 页面 | 页级横向溢出 | 出屏交互元素 | 备注 |
|---|---|---|---|
| `/`（工作台） | 无 | 无 | 2 个内部装饰圆点（environment-dot/status-dot 投影被 clientWidth 截断）为误报 |
| `/clusters` | 无 | 无 | – |
| `/workloads` | 无 | 无 | 4 个空态装饰 span 投影截断，非交互 |
| `/diagnoses` | 无 | 无 | – |
| `/login` | 无 | 无 | 既有 14/15 轮移动端修复回归通过 |

- 结论：登录页 + 4 个控制台关键页面在 375px 移动视口**无横向滚动、无可点击元素出屏**，
  与截图基线中的 `*-mobile-375x812.png` 吻合。前端响应式基线整体成立，无需代码改动。

## Verification

- 全部实测命令与上述表格证据一致；页面截图基线（`docs/ui-baselines/`）在 mobile
  视口 sha256 一致。
- 本改动为审计证据归档，无源码变更、无需重新 build。

## Follow-up

- 其余 ~30 个视图（事件中心、拓扑、优化中心、事故工作空间、通知投递、审计日志、
  用户管理、Helm/GitOps 等）按同一审计脚本补充；发现问题时按既有样式修复并纳入基线。
- 可将审计脚本固化到 `scripts/` 并扩为所有 `views`，配合 `--verify` 形成响应式门禁。
