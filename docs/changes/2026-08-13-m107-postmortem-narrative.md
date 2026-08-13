# M107：复盘叙事视图（Postmortem Narrative）

- Date: 2026-08-13
- Status: Complete
- Scope: M107 事故协作闭环收官块——把 incident 详情抽屉的复盘区升级为只读叙事视图：
  复盘结论 + 结果指标（SLA/解决耗时/事件统计/证据来源），时间线支持「全部/备注/系统」
  过滤、证据时间线支持按来源过滤；Playwright Desktop/Mobile 双端覆盖新交互。

## Context

M107 目标是把 incident 升级为「10 秒可交接、可复盘」。前三块已交付证据时间线、SLA 提醒、
批量指派；本块落实 roadmap 中「复盘视图（只读）：事故解决后可查看证据、决策、动作、结果
完整叙事；时间线可按人工/系统/来源过滤；不修改历史记录」——数据全部复用现有
`getIncident` / `getIncidentEvidence` 只读接口，不扩展任何写入面。

## What Changed

### 前端
- `frontend/src/views/IncidentsView.vue`：
  - 时间线新增过滤 tabs「全部 (n) / 备注 (n) / 系统 (n)」，按 `event_type` 客户端过滤，
    与既有系统/人工事件分离约定一致。
  - 证据时间线在来源数 > 1 时显示来源过滤 tabs（诊断记录/人工上报/告警实例/巡检结果/
    信号实例 + 计数），按 `source_type` 过滤。
  - 已解决事故显示「复盘视图」区块：复盘结论 + 结果指标卡（SLA 达标/逾期、解决耗时、
    系统事件数、人工备注数、证据来源数）；编辑复盘仍限 `canManage`。
  - `openDetail` 时重置两个过滤器，避免跨事故残留。

### 测试
- `frontend/e2e/incidents.spec.ts`：新增 `/incidents/7/evidence` 双来源 mock（finding +
  alert）；workflow 用例扩展断言——复盘视图指标渲染、时间线「备注/系统」过滤计数与内容、
  证据「告警实例」过滤后卡片数与内容、切回全部后恢复。
- Playwright Desktop Chrome / Mobile Chrome 双项目 incidents spec 4/4 通过。

## Verification

- 前端：`pnpm typecheck`、`pnpm lint`、`pnpm test`（26 files / 141 tests）、`pnpm build`
  全绿。
- E2E：`npx playwright test incidents --project="Desktop Chrome"` 与
  `--project="Mobile Chrome"` 均 4/4 通过。
- 后端无改动：`go test ./... -short` 不回归（本次无 Go 变更）。
- 敏感扫描：`scripts/scan-sensitive-fields.sh` clean。

## Notes

- 本块为纯只读叙事增强：不修改历史记录、不新增 API、不改变权限面（复盘编辑仍仅
  operations_admin/system_admin）。
