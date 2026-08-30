# p2d-fleet-diagnoses-aggregation: 多集群 Federation 深化 — 跨集群诊断聚合端点

- Date: 2026-08-30
- Status: Complete
- Scope: P2d — federation 层新增跨集群只读诊断聚合端点（list + stats），补完旗舰路线图最大缺口。

## Context

P2d 是 P2 旗舰深化四方向的最后一大块：在 M48 联邦模型（host/member 拓扑、心跳、ResourceSummary fan-out、GlobalSearch）之上，
补「跨集群诊断聚合」能力——单一视图查看全舰队诊断记录。

实现策略采用路线图 §3 P2d 标注的**平台侧聚合**（降级方案）：diagnosis_records 表已集中存储（每行含 cluster_id），
不需实时 fan-out 到各集群；通过 SQL `WHERE cluster_id IN (visible)` 实现 2D 授权作用域过滤，
保持 ADR 0004 有界只读网关的约束。

## What Changed

### 后端 — 诊断仓储（diagnosis.Repository）
- `internal/diagnosis/repository.go`：`Repository` 接口新增 `ListByClusters` 与 `StatsByClusters` 方法，
  `GormRepository` 实现 SQL `WHERE cluster_id IN ?` 聚合查询（列表按 observed_at DESC、统计按 status/severity/cluster_id 分组）。
- 新增共享类型：`FederationDiagnosisRow`（列表投影）、`FederationDiagnosisStats` / `FederationClusterCount`（统计聚合）。

### 后端 — 联邦诊断聚合（federation 包）
- `internal/federation/diagnoses.go`（新建）：
  - `FederationDiagnosisRepository` 接口（诊断仓储面向联邦的读表面）。
  - `Service.ListDiagnoses` — 校验输入 → 调用仓储 → 附加集群名（经 federation 自身集群注册表查找）。
  - `Service.DiagnosesStats` — 校验 → 调用仓储 → 转换为联邦统计投影 → 附加集群名。
  - 输入校验：status / severity 枚举白名单、limit 上限 200（默认 50）、空集群集 fail-closed。
- `internal/federation/service.go`：`Service` 新增 `diagnosisRepo` 字段与 `WithDiagnosisRepository` 流式 setter。

### 后端 — handler 与路由
- `internal/httpserver/federation.go`：新增 `listDiagnoses`（GET /federation/diagnoses）与 `diagnosisStats`（GET /federation/diagnoses/stats），
  复用 `authorizedClusterFilter` 实现 2D 集群授权。
- `internal/httpserver/router.go`：注册两条新路由（仅当 `FederationService != nil` 时），
  AuditAction 分别为 `federation.diagnoses.list` / `federation.diagnoses.stats.read`。

### 后端 — 接线（main.go）
- `cmd/server/main.go`：`federationService` 构造后附加 `.WithDiagnosisRepository(diagnosis.NewGormRepository(database.GORM()))`。

### 文档 — OpenAPI + 权限矩阵
- `docs/api/openapi.yaml`：新增 `/api/v1/federation/diagnoses` 与 `/api/v1/federation/diagnoses/stats` 两条路径，
  新增 `FederationDiagnosisRow` / `FederationDiagnosisList` / `FederationDiagnosisStats` / `FederationDiagnosisClusterCount` schema。
- `docs/security/permission-matrix.md`：重新生成（`-update`），路由总数由 309 升至 311，两条新路由均标记为 `scope=none`、角色 `any`（认证即读）。

### 测试
- `internal/federation/service_test.go`：新增 `TestListDiagnoses_NilRepo`、`TestListDiagnoses_EmptyClusters`、
  `TestListDiagnoses_InvalidStatus`、`TestListDiagnoses_InvalidSeverity`、`TestDiagnosesStats_NilRepo` 共 5 条。
- `internal/httpserver/federation_test.go`：新增 `TestFederationHandler_ListDiagnosesReturns200`（含集群名富化断言）、
  `TestFederationHandler_ListDiagnosesStatusFilter`、`TestFederationHandler_ListDiagnosesInvalidStatusReturns400`、
  `TestFederationHandler_ListDiagnosesInvalidSeverityReturns400`、`TestFederationHandler_DiagnosisStatsReturns200`、
  `TestFederationHandler_DiagnosisStatsNilRepoReturnsZeros`、`TestFederationHandler_ListDiagnosesNilRepoReturnsEmpty` 共 7 条。
- Mock 桩：`internal/httpserver/federation_test.go` 新增 `handlerFedDiagRepo`；
  `internal/httpserver/diagnosis_handler_test.go` 与 `diagnosis_node_metrics_test.go` 的 `diagTestRepo` / `diagnosisRepositoryStub` 补齐接口方法。
- `internal/diagnosis/service_test.go`：`memRepo` 补齐 `ListByClusters` / `StatsByClusters`（含 `sort` import）。

## Verification

- `go vet ./...`：0 issue（全仓）。
- `go build ./...`：通过。
- `go test ./internal/federation/... -v`：全部通过（含 5 条新 tests）。
- `go test ./internal/diagnosis/... -v`：全部通过。
- `go test ./internal/httpserver/... -v`：全部通过（含 7 条新 P2d handler tests）。
- `TestRegisteredRoutesMatchOpenAPI`：PASS（新增 2 条路由已写入 OpenAPI）。
- `TestPermissionMatrixMatchesCommittedDocument`：PASS（矩阵已重新生成并包含 311 条路由）。

## Risks / Notes

- 实现为平台侧聚合（纯 SQL），非实时 fan-out——符合路线图降级方案，且与 M48 mock 网关测试策略一致。
- 空集群集 fail-closed：authorizedClusterFilter 返回 []int64{} 时直接返回零结果/零统计。
- diagnosisRepo 为 nil 时端点降级为零结果/空统计（不报 503），保持联邦服务整体可发现。
- 后续若需实时 fan-out（P2d-2），可替换 ListByClusters 实现为有界并发扇出（与 ResourceSummary 同模式）。
