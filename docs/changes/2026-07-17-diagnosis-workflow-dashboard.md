# 2026-07-17 Diagnosis Workflow and Dashboard

## Scope

- 新增 `000005_diagnosis_workflow`，保存负责人、诊断更新时间、状态活动和人工反馈。
- 实现显式状态机、事务行锁、首次操作自动认领与非法跳转冲突响应。
- 增加诊断状态更新、人工反馈、状态汇总和最近活动 API。
- 状态与反馈写接口限制为系统管理员和运维管理员。
- 智能诊断详情增加处置备注、确认/解决/驳回/重新打开、活动时间线和规则准确性反馈。
- Dashboard 接入真实集群数、诊断总数、待处置数、解决率和最近诊断活动。

## Verification

- `go test ./...` 与服务端构建通过；状态机测试覆盖合法流转、跨级非法流转和反馈枚举。
- `pnpm typecheck`、5 个 Vitest 文件共 11 项测试、`pnpm build` 通过。
- PostgreSQL 真实应用 `000005`，确认两张新表和迁移记录存在。
- API 验证 `open → confirmed → resolved`，负责人自动设置为当前管理员，详情返回 2 条活动和 1 条反馈；`resolved → confirmed` 返回 HTTP 409。
- 汇总 API 返回 total=1、resolved=1、recent=1；修复了聚合结果不能直接扫描到带切片响应结构的问题。
- 浏览器验证负责人、状态按钮、活动/反馈详情，并执行 `resolved → open`；Dashboard 随后显示 1 个集群、1 条诊断、1 条待处置记录和最近活动。
- 测试完成后删除专用集群，验证诊断、证据、活动和反馈均级联清理，并停止临时服务。

## Boundaries

- 人工处置不会修改规则结论和历史证据，也不会执行 Kubernetes 变更。
- 业务活动时间线不替代后续平台级安全审计。
- Dashboard 指标来自平台数据库，不跨集群实时聚合 Pod。
- AI Provider 仍未接入，人工反馈暂不自动影响规则。

## Deferred

- 诊断批量处置和通知；负责人转派与 SLA 已在后续变更 `2026-07-17-diagnosis-sla-assignment.md` 完成。
- 人工反馈评估；AI Provider 与引用式解释已在后续变更 `2026-07-17-cited-ai-explanations.md` 完成。
- kind 故障场景、EndpointSlice 和完整端到端自动化。

平台级写操作审计与审计中心已在后续变更 `2026-07-17-platform-audit-trail.md` 完成。
