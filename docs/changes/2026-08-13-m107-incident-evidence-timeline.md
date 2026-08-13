# M107：事故证据时间线（Incident Evidence Timeline）

- Date: 2026-08-13
- Status: Complete
- Scope: incident 详情新增「证据时间线」只读区块——五源（diagnosis/finding/alert/inspection/signal）证据的结构化聚合，附带前端深链；后端 evidence API + OpenAPI/typegen/permission 同步 + demo-drill 断言

## Context

M107 事故协作闭环的第一步：incident 已有 M98 的完整协作模型（状态机/CAS/时间线/关注者/SLA/复盘）与
M103–M105 的五源创建路由，但**缺失 incident 详情页的证据聚合视图**——创建者无法在 incident 首屏回答
「根因→影响→证据→下一步」。本里程碑为 incident 详情新增可靠、防泄漏、可回退的「证据时间线」：
把 incident 背后的源（诊断/人工上报/告警/巡检/信号）解析为带标签字段与前端深链的结构化证据块。

## What Changed

### 后端
- `backend/internal/incident/evidence.go`（新增）：`EvidenceItem` / `EvidenceField` 模型、
  `EvidenceResolver` 接口、`Service.Evidence(id)` 方法（快照回退 + resolver 富集）、
  `IncidentDeepLink(sourceType)` 前端深链映射。resolver 失败（源记录缺失）永不断详情：
  始终回退到 incident 快照字段。
- `backend/internal/incident/evidence_test.go`（新增）：快照回退、resolver 富集、
  resolver 错误回退、404 四类单测。
- `backend/internal/incident/service.go`：`Service` 增加 `evidenceResolver` 字段与
  `WithEvidenceResolver`。
- `backend/cmd/server/incident_resolver.go`：实现 `ResolveEvidence`（复用五源 resolve 逻辑，
  附加集群/来源标签字段 + 深链）。
- `backend/cmd/server/main.go`：`incidentSourceResolver` 同时作 source 与 evidence resolver。
- `backend/internal/httpserver/router.go`：注册 `GET /api/v1/incidents/{incident_id}/evidence`
  （审计 `incident.evidence.get`）。
- `backend/internal/httpserver/incidents.go`：`evidence` handler（200 items / 404）。
- `backend/internal/httpserver/incidents_test.go`：handler 测试（200 + 404）。

### OpenAPI / 类型
- `docs/api/openapi.yaml`：`/incidents/{incident_id}/evidence` 路由 +
  `IncidentEvidenceItem` / `IncidentEvidenceField` schema。
- `frontend/src/api/openapi.d.ts`：typegen 重新生成（CI sync gate 生效）。
- `docs/security/permission-matrix.md`：权限矩阵重新生成（新路由登记）。

### 前端
- `frontend/src/types/incident.ts`：`IncidentEvidenceItem` / `IncidentEvidenceField`。
- `frontend/src/api/incidents.ts`：`getIncidentEvidence`。
- `frontend/src/views/IncidentsView.vue`：详情抽屉新增「证据时间线」只读区块——证据卡含来源、
  严重度、深链、标题、摘要、字段表、资源、来源引用与时间；`openDetail` 时并发加载。

### 演示演练
- `scripts/demo-drill.sh`：incident-journey 新增 `incident-evidence` 断言（返回 diagnosis 源、
  deep_link == /diagnoses 且 title 非空）。

## Verification

- 后端：`go build ./cmd/server`、`go vet ./...`、`go test ./internal/incident/... ./internal/httpserver/...`
  全绿；`go test ./... -short` 无 FAIL。
- 契约：`TestRegisteredRoutesMatchOpenAPI`（OpenAPI ↔ Gin 路由一致）、
  `TestPermissionMatrixMatchesCommittedDocument` 通过。
- 前端：`pnpm typecheck`、`pnpm lint`、`pnpm test`（26 files / 141 tests）、`pnpm build` 全绿。
- 敏感扫描：`scripts/scan-sensitive-fields.sh` clean（1235 tracked files）。
- 路由 E2E（新后端镜像 v0.3.0-m107，`/incidents/99999/evidence`）：HTTP 404
  `INCIDENT_NOT_FOUND`（证明路由已注册并走 handler）。
- 浏览器 /incidents 页渲染：3/3 运行无 pageerror，页面正常加载。

## Risks / Notes

- 本里程碑单独交付「证据时间线」；SLA 仪表、复盘叙事深化、五源统一时间轴合并在后续 milestone。
- 前端并发轨（另一 Agent）正在改 `console-theme.css` / `LoginView.vue`，与本文档改动
  （`IncidentsView.vue` 等）为不同文件，不冲突；其未提交改动不纳入本提交。
- `IncidentDeepLink` 返回源模块列表路由；列表页当前不支持 query 深链聚焦，后续若加
  `?focus=source_ref` 可平滑升级。
