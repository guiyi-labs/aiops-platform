# Frontend CI 失败深查结论（M115 收口 · 非 flaky 稳定失败）

- Status: Analysis complete（落盘，未推送）
- Scope: CI `Frontend Browser regression (M96 shell)` 4 个失败测试
- Commit 基线: af670de（operator 覆盖率修复后）

## 1. 根因结论

**Operator 增强零间接影响。这是前端测试的既存数据缺口问题**，根因是
**M112/M113 前端加了新接口的驾驶舱渲染，但 e2e mock 从未同步补充对应路由**，
请求落到 api-fixtures 的兜底空响应 → 缺结构字段 → Vue render function 抛
TypeError → 整个组件 DOM 更新中断 → 断言目标停留在旧状态。

4 个失败 = 2 个 spec × Desktop/Mobile 双浏览器：

| 测试 | 根因 | 崩溃点 |
|---|---|---|
| `incidents.spec.ts:154` workflow | M112-1 (d96db33) 引入 cockpit 渲染，e2e 未补 `/incidents/:id/context` mock | `cockpit.resource_context.scope.cluster_id` → `undefined.scope` TypeError |
| `finding-evidence.spec.ts:131` inspection | M113-3 (dbee872) 引入 coverage 渲染，e2e 未补 inspection `coverage` 接口 mock | `Object.keys(coverage.by_severity)` → `Object.keys(undefined)` TypeError |

## 2. 证据（本地完整复现）

- `npx playwright test` 本地 Desktop + Mobile 稳定复现同一批失败（非 flaky）。
- 重跑 CI（gh run rerun，same commit af670de）→ 同样 4 个失败。
- Playwright pageerror 抓取到精确堆栈：
  - incidents: `TypeError: Cannot read properties of undefined (reading 'scope')`
    ① PATCH `/api/v1/incidents/7` → 200，响应体 `status:"confirmed"`（trace.zip 验证）
    ② 前端 `transitionIncident` 正常执行，打点确认 `detail.status after= confirmed`
    ③ 但 `cockpit.resource_context.scope` → undefined（mock 兜底 `emptyAPIResponse`
       无 resource_context）→ 渲染崩溃 → workflow-status 停留旧 DOM「待确认」
  - finding-evidence: `TypeError: Cannot convert undefined or null to object`
    at `Object.keys` (InspectionView.vue coverage 区块)：`coverage.fail_closed` undefined
    取反为 true 进入渲染分支 → `Object.keys(coverage.by_severity=undefined)` 崩溃。

- **接口 / 后端无参与**：前端 E2E 使用 `page.route('**/api/v1/**')` 全量 mock，
  所有请求被拦截，**不触达真实后端** → operator 增强（backend 变更）不可能影响。

## 3. 修复方案（待前端配套执行，不属当前任务范围）

- 在 `frontend/e2e/incidents.spec.ts` beforeEach 补 `**/api/v1/incidents/7/context`
  路由，返回合法 `IncidentContextCockpit`（含 resource_context.scope / freshness /
  health / sla / evidence_sources / recent_events / recommended_actions）。
- 在 `frontend/e2e/finding-evidence.spec.ts` beforeEach 补 inspection 覆盖率接口
  mock，返回合法 `InspectionCoverageSummary`（含 by_severity / fail_closed /
  trend / rule_coverage 等必填字段）。
- 建议 commit：`test(frontend): add missing cockpit/coverage mocks in e2e`。
- 长期：api-fixtures 兜底改 fail-closed（返回 501/400 而非结构不符的 200 空对象），
  或对未知路由 fail 测试，避免这类静默缺字段崩溃。

## 4. 建议下一步

1. **修复方向明确**：补 2 个 spec 的 mock 路由（纯测试数据修正，不动业务代码），
   预计 1 个 commit；修复后 4 个测试应转绿（其余 28 个测试不受影响）。
2. 需前端职责 Agent / 持库人执行（本任务约束：只判因，不改前端业务代码）。
3. 若采纳「兜底 fail-closed」长期项，可与修复一并做，或后续单独记录。

## 附：与增强无关联的证据链

- `git diff b8d2374..af670de -- frontend/` 为空（operator 提交零前端改动）。
- b8d2374 的 Frontend job 为 **skipped**（change-scope 纯文档），并非真正通过；
  最近一次 Frontend 真跑且通过是 4b1a862（M111，8/14 05:03）。
- 首个前端失败 commit：3d79961（8/15 10:20，gate 上调，早于 operator 全部提交）。
