# M95：统一 Finding 与证据模型（FindingDetail v2）

- Date: 2026-08-10
- Status: Complete（M95 后端统一证据模型；前端同一证据组件渲染为 M95 后续增量）
- Scope: 在 `internal/finding` 定义 `FindingDetail v2` 统一证据模型，提供 v1→v2 兼容层、
  共享 severity 映射、按资源合并去重、golden dataset 版本升级与迁移提示。

## Context

M94 已完成诊断叙事、类型化行动区与深链。M95 目标是让 18 个分析器、诊断、巡检和治理态势
共享“规则 → 证据 → 建议”的稳定模型：规则身份、资源引用、严重度、证据引用、建议、可执行
能力与版本信息。本步交付后端统一模型与契约锁，并为前端同一证据组件渲染预留稳定 v2 结构。

## What Changed

### 后端：统一证据模型（新增）

- `backend/internal/finding/detail.go`（新增）：
  - `FindingDetail` v2：内嵌并保留全部 v1 `Finding` 字段（code/severity/summary/resource/
    details/observed_at），新增 `SchemaVersion`、`Rule`（rule_id/framework/source/version）、
    `Evidence`（稳定证据引用）、`Recommendations`（类型化建议）、`OriginIDs`（合并来源）。
  - `FromV1(Finding) FindingDetail` / `FindingDetail.ToV1() Finding`：v1↔v2 兼容层，
    v2 → v1 扁平化与原始 v1 JSON 字节一致，旧消费者继续工作。
  - 建议类型 `advisory` / `controlled_action_available` / `manual_only`，平台默认不自动执行
    （受控动作仍需 dry-run + 确认，与 M94 行动区语义一致）。
  - `EvidenceKind`：`resource_state / event / log / alert / change / automation`，
    与 M94 时间线分类同一证据语法。
  - `MergeDistinct([]FindingDetail) []FindingDetail`：同一资源重复 finding 合并展示，
    保留每条规则来源与原始 ID（写入 `OriginIDs`）。
- `backend/internal/finding/severity.go`（新增）：`SeverityRank` / `NormalizeSeverity` /
  `MaxSeverity`，统一 posture / optimization / diagnosis / inspection 的严重度映射
  （high→critical、medium→warning、low→info；未知归入 info）。

### 后端：分析器契约共享严重度映射

- `backend/internal/posture/posture.go`：`severityRank` 委托给 `finding.SeverityRank`，
  聚合治理态势与所有分析器共享同一排序（critical → warning → info）。

### 后端：golden dataset 版本升级与迁移提示

- `backend/internal/golden/model.go`：`DatasetVersion` 1.1 → 1.2（证据模型升级按 M45 质量
  报告契约记录；本次为金标数据版本变更，旧快照保持可读）。
- `backend/internal/golden/quality.go`：新增 `DatasetMigrationHint(version)`，对 1.1/1.0/
  缺失版本返回迁移提示；当前版本返回空。
- `backend/internal/golden/model_test.go`：`TestDatasetVersion` 期望更新为 1.2；新增
  `TestDatasetMigrationHint` 证明旧快照版本仍能读取并得到迁移提示。

### 测试

- `backend/internal/finding/detail_test.go`（新增）：v1→v2→v1 无损往返、v1 JSON 字节级
  parity 锁、v2 序列化结构快照（schema_version/rule/evidence/recommendations/origin_ids）、
  `MergeDistinct` 按资源合并并保留规则来源、建议类型不自动执行、统一严重度映射锁。
- `backend/internal/finding/detail_parity_test.go`（新增）：11 个 posture 分析器的
  `Status.Findings` 元素类型反射断言为规范 `finding.Finding` + finops `ToFindings()`
  返回类型断言（capacity/cis/deprecatedapi/gitopsdrift/hpa/imagepolicy/ingressposture/
  netpolicy/pdb/policy + finops），证明分析器未漂移到私有 finding 结构；另加
  `TestV1ToV2ParityLock` 锁定 v1 wire contract。

## Verification

- `go test ./...`（backend 全量）：green。
- `go vet ./...`：green。
- `go test ./internal/finding/... ./internal/golden/... ./internal/posture/...`：green。
- OpenAPI 未变更（`FindingDetail v2` 为后端内部统一模型），`pnpm typegen` 无 diff，旧 API
  消费者兼容窗口保持不变。
- git diff --check：通过。

## Risks / Notes

- M95 后端统一模型完成；前端 PostureView/OptimizationView/DiagnosesView/InspectionView 统一
  证据组件渲染为 M95 后续增量（新 change-record 归档），v2 结构已按本轮定义供其消费。
- 诊断记录保留自身 high/medium/low 词汇保证 API 兼容；统一映射经 `NormalizeSeverity` 在
  消费侧转换，不篡改历史记录。
- 旧快照（v1.0/v1.1）读取路径未变，`DatasetMigrationHint` 仅在需要升级提示时调用。