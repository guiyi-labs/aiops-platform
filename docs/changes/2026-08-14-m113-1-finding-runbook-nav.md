# M113-1 Finding → Runbook 预览导航：优化中心闭环第一步

- Date: 2026-08-14
- Status: Complete
- Scope: M113 第一个切片——把 posture/optimization finding 一键跳转到对应的 insight runbook（deterministic diagnosis → inspection rule → AI explanation deep-link → dry-run operation preview），全程只读。

## Context

- 上位路线：`docs/development-roadmap-post-m110.md` Track D 第 154–155 行：finding → runbook 预览导航，posture/optimization finding 一键跳转对应 `insight` runbook（dry-run 预览 + 可执行 runbook 入口），全程只读导航，不新增写路径。
- 现状发现：M81 已有 `GET /api/v1/aiops/insight` 端点，`PostureView.vue` 已实现闭环导航（`toggleInsight` + InsightRunbook 渲染）。但 `OptimizationView.vue` 的所有 11 个分析器 tab（FinOps/CIS/废弃API/网络/镜像/GitOps漂移/容量/策略/HPA/PDB/Ingress）中，finding 行只有 `FindingEvidencePanel`（M95 证据链）而**没有** insight runbook 导航按钮，违反路线"优化中心 finding 一键跳转"的验收要求。
- 设计决策：不新建重复的后端端点（`/optimization/runbook-preview` 已被发现与 `/api/v1/aiops/insight` 完全重复，已在代码写入后立即回滚）。改为提取可复用的 `FindingRunbookPanel.vue` 组件，嵌入 OptimizationView 的 11 个分析器 finding 行，复用现有 M81 端点。

## What Changed

### 新增
- `frontend/src/components/FindingRunbookPanel.vue`（新增）：可复用只读闭环导航组件，接受 `domain/code/kind/namespace/name/clusterId` props；点击"查看闭环"按钮 → 调用 `getInsightRunbook`（M81 GET /aiops/insight）→ 渲染确定性诊断路由、巡检佐证、AI 引用解释 deep-link、受控操作预览（dry-run）四步；支持展开/收起；加载失败显示错误信息；零写操作。
- `frontend/src/views/OptimizationView.vue`（修改）：在全部 11 个分析器（FinOps/CIS/废弃API/网络/镜像/GitOps漂移/容量/策略/HPA/PDB/Ingress）的 finding 行中，在 `FindingEvidencePanel` 之后增加 `<FindingRunbookPanel>` 组件实例；导入 `FindingRunbookPanel`。每个实例传入对应的 domain、item.code（或 rec.code）、resource kind/namespace/name 和 selectedClusterID。

### 后端（无新增）
- 未新增后端端点（`GET /api/v1/aiops/insight` M81 端点已覆盖所有 domain + kind 的 runbook 解析，OptimizationView 各分析器的 finding domain 与资源 kind 均已在 `insight.Resolve` 的 `diagnosisByKind` / `inspectionByDomain` / `operationByKind` 映射表中有对应条目）。
- 在代码写入过程中曾短暂创建 `backend/internal/httpserver/finding_runbook.go` + test，经发现与现有 M81 端点完全重复后已完全删除，工作树恢复干净（无后端文件变更残留）。

## Verification

- `pnpm typecheck`：vue-tsc 全绿，无 TS 错误。
- `pnpm lint`：ESLint 全绿。
- `pnpm test -- --run`：152 用例全部通过（与 M112-4 完成时一致）。
- `pnpm build`：dist 产物生成成功，无错误。
- 后端无变更（M81 端点未改动），无新路由，无 OpenAPI / 权限矩阵变化。
- 行为验证：每个 finding 行的"查看闭环"按钮调用 `getInsightRunbook` → 返回的 `InsightRunbook` 包含 `diagnoses`/`inspection`/`ai_explanation`/`operations`（dry-run 预览）；点击"收起闭环"折叠面板；未选集群时按钮保持 disabled，不发请求。

## Risks / Notes

- FinOps findings（`FinOpsRecommendation`）的 `domain` 为 `"finops"`，与 capacity/network 等分析器的 finding 域一致，`insight.Resolve` 能正确匹配 `operationByKind`（Deployment 级操作）和 `diagnosisByKind`（Deployment 诊断路由）。若 finops 的 workload kind 为裸 Pod（非 Deployment），`operations` 数组为空，只渲染诊断路由，符合预期。
- `FindingRunbookPanel` 是一个新组件，目前仅在 OptimizationView 使用。PostureView 的 M81 闭环导航仍然是内联逻辑（不使用此组件），若后续需统一，可将 PostureView 重构为使用同一组件，但不在本切片范围内（避免 diff 过大）。
- M81 端点 `/api/v1/aiops/insight` 要求 `domain` 非空；OptimizationView 各分析器的 `domain` 字符串（finops/cis/deprecated_api/network/image/gitops/capacity/policy/hpa/pdb/ingress）均为非空，满足约束。
- M113 剩余两个切片：M113-2 容量感知预览（按资源约束排序 + 适配解释 + 数据更新时间）、M113-3 巡检趋势与覆盖率度量（plan→findings 时间序列 + 规则命中覆盖率 + fail-closed）。
