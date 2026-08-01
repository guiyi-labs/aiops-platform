# M58: DevOps Read-Only + Cross-Cluster Copy + Backup/Restore GUI

- Date: 2026-08-01
- Status: **开发完成 (开发完成)**
- Milestone: M58
- ADR: [0070](https://docs/adr/0070-devops-readonly-copyops-and-backup-gui.md)

## 概述

M58 在 v2 路线上完成 Phase 3 的最后一个后端增量里程碑。交付三项 DevOps 能力的后端支持：

1. **GitOps 只读接入（ArgoCD Application）**：通过集群已注册的 ADR 0004 受限 Kubernetes 网关直接读取 `argoproj.io/v1alpha1 Application` CR，输出统一的 sync/health + repo/destination 投影 + 原始 manifest，同时暴露 `/gitops/capability` 能力探测（CRD/API 不存在时返回 `available=false`，不报错）。
2. **交互式跨集群复制 copyops**：复用 M19 受控操作契约（Preview → 签发一次性确认令牌 → Execute 校验令牌 + 幂等键 + CAS 源命名空间身份 → 单集群内 dry-run 再执行）。新增一张 `copy_plans` 表（全部 JSONB，单写原子持久化），Bundle 上限 20 条、资源种类走 Operator 白名单。
3. **备份/还原 GUI 列表&详情**：在原有的 M22/M23 备份/还原 *Plan* CRUD 之上，扩展出 Velero Backup/Restore CR 的浏览接口（按命名空间、分页、行投影 + 原始 manifest），使 GUI 在引导创建备份向导前可先呈现已有备份。

## 交付清单

### 数据库迁移

- `backend/migrations/000039_copyops_and_gitops.up.sql`
  - 创建 `copy_plans` 表：`id CHAR(36) CHECK(char_length(id)=36)`，状态枚举 `CHECK(status IN ('awaiting_confirmation','executing','succeeded','failed','expired'))`，JSONB 列 `resource_items`、`copy_summary`、`diff`，确认令牌哈希、幂等键、租约 `locked_at`、TTL `expires_at`。
  - 索引：`idx_copy_plans_source_cluster`、`idx_copy_plans_target_cluster`、`idx_copy_plans_requested_by`、唯一键 `UNIQUE(requested_by_user_id, idempotency_key)` 禁止不同用户复用同一幂等键。
- `backend/migrations/000039_copyops_and_gitops.down.sql`
  - 删除 `copy_plans` 表及索引。

### 代码变更

**GitOps 包（只读）**

- `backend/internal/gitops/service.go`
  - `Capability(ctx, clusterID) → GitOpsCapability{available, mode, application_count, ...}`：先探测 APIResource `argoproj.io/v1alpha1 applications` 是否存在，再 LIST 一次拿到总数；CRD 不存在时返回 `mode=none` 不报错。
  - `List(ctx, clusterID, query) → {items,total,remaining}`：动态列出 Application，按 `name` substring 过滤，统一提取 `spec.project / source / destination / status.sync / status.health`，投影到 GUI 友好结构并附加 `raw_manifest`。
  - `Get(ctx, clusterID, name) → GitOpsApplication`：单个 CR，投影同上。
- `backend/internal/gitops/service_test.go`
  - 动态客户端假实现 + fakeKubernetes（ListRaw/GetRaw）。覆盖：capability=none（缺 CRD）、capability=direct_application_cr（有 CRD）、List 分页/过滤、Get 命中/缺失。
  - 所有 stub 无 CGO 依赖，零 go-sqlite3。

**跨集群复制 copyops 包**

- `backend/internal/copyops/model.go`
  - 核心模型：`Plan`、`ResourceItem`、`PreviewRequest`、`ExecuteRequest`、`ActorRef`、`CopySummaryItem`、`PlanDiff`、`ItemDiff`。
  - 常量：`MaxBundle=20`，状态常量（`StatusAwaitingConfirmation`…），模式常量（`ModeCreate`…），错误变量（`ErrKindDisallowed` / `ErrBundleTooLarge` / `ErrNamespaceMissing` / `ErrInvalidIdempotency` / `ErrConfirmationInvalid` …）。
  - JSON 辅助：`MarshalResourceItems/UnmarshalResourceItems`、`MarshalPlanDiff/UnmarshalPlanDiff`、新增 `MarshalDiff/UnmarshalDiff`（单条资源 diff）。
  - 修复：`ResourceItem` 新增 `Diff JSON` 持久化字段，用于 GUI diff 查看器。
- `backend/internal/copyops/repository.go`
  - `Repository` 接口：`Create/GetByID/ListByUser/ListByCluster/ClaimAndLoad/UpdateExecution/UpdateStatus`。
  - `GormRepository` 实现：`ClaimAndLoad` 支持终端状态的**幂等重放**（先比对幂等键再走已执行短路 — 这个是 M58 过程中由测试反推出来的重写，避免 GUI 重复点 Execute 时被 `ErrAlreadyExecuted` 挡掉）；15s 租约 + CAS 循环；`StatusAwaitingConfirmation → StatusExecuting` 原子切换。
  - 常量比较用 `constantTimeEqual`（来自原 promotion 风格）。
- `backend/internal/copyops/service.go`
  - `Preview(ctx, req, actor) → Plan`：
    - 入口先 cap bundle size（**在 normalize/dedup 之前**，防止客户端塞 1e6 重复项打内存）。
    - 预检 0：SourceNamespaceIdentity 捕获 CAS 基准。
    - 预检 1：TargetNamespace 必须存在（不自动创建，避免惊扰集群管理员）。
    - 预检 2：每资源 GVR → 拉原始 manifest → scrub 固定前缀标签/注解 + nodeName + 状态 → 重写 metadata.namespace/name → "already exists on dest" 则 skip → 服务端 dry-run create：成功=Pending，冲突=Skipped，其他 admission 错误=Failed+DryRunError。
    - 生成 `ResourceItem.Diff`（ModeCreate + scrubbed manifest 作为 After + changed_fields=".*"）。
    - `newIdentity()`：`cp-` + 16B hex + 补位 `c`，确保 `len=36`（CHECK 约束），确认令牌 32B → base64url，sha256 哈希落库；TTL 默认 10 分钟。
    - 单事务性写 `repository.Create(plan)`，返回 plan 带 transient `ConfirmationToken`（仅 Preview 响应头带 `Cache-Control: no-store`，其他接口不泄露）。
  - `Execute(ctx, req, actor) → Plan`：`ClaimAndLoad` → 若终端状态且幂等匹配则直接返回（重放）；否则：
    - CAS 门：再次 `SourceNamespaceIdentity`，UID 必须与 Preview 一致。
    - Destination namespace 再次检查。
    - 对每个 Pending 项 dry-run=false `CreateResource`，解析响应拿到 applied UID/RV；Skipped 项跳过计数，Failed 项累计。
    - 终态：全失败→`StatusFailed`，部分失败→`StatusFailed`+聚合错误，其余→`StatusSucceeded`。
    - `failPlan` helper 将剩余 Pending 项统一标 Skipped 并写 `preflight_skip = plan precheck failed: …`，保证 plan JSONB 行内状态一致。
  - `Get / ListByUser / ListByCluster`：基础访问 + 参数非法走 `ErrInvalidRequest`。
  - internals：`normalizeRequest`（bundle 项去重、label/annotation 前缀去重）、`validatePreviewRequest / validateExecuteRequest`、`kubeNamePattern`、`stripManifest`（标签/注解前缀、Secret 数据打码、`spec.nodeName` 等集群特定字段清洗）、`sha256Sum`。
  - 修复：plan id 长度 bug（原 `cp`+32 hex+`1`=35，不满足 CHECK → 改为 `cp-`+32 hex+`c`=36）。
- `backend/internal/copyops/service_test.go`
  - **移除了 CGO 依赖 go-sqlite3 的 Gorm SQLite 测试持久化**，改以 `inmemRepo` 线程安全假实现覆盖所有 Repository 接口（包括幂等重放分支和租约冲突分支）。
  - 假 Kubernetes：`fakeKubernetes { rawResource, nsExists, nsIdentity, resExists, createRes }`，可细粒度模拟 "资源已存在 / dry-run 失败 / CAS drift / 成功响应回填 applied UID"。
  - 覆盖场景：
    - `TestValidatePreviewRequest_InvalidCluster / EmptyBundle / BundleTooLarge / DisallowedKind` 各走错误分支。
    - `TestPreview_Success`：21 条相同项 BundleTooLarge（cap before dedup 验证）；单 Deployment 正常预览：ArgoCD 标签 / last-applied 注解被 scrub、`nodeName` 被 scrub、Status 字段不在 manifest 中出现，Diff.Mode=create，已落库可 `GetByID` 再读回一致，RequestedByName 对齐 actor。
    - `TestPreview_DestinationNamespaceMissing`：目标命名空间不存在即短路。
    - `TestExecute_IdempotencyReplay`：首次 execute 成功 → 同幂等键重放返回同一 plan + succeeded → 不同幂等键被拒 `ErrInvalidIdempotency`。
    - `TestExecute_CASDrift`：Preview→Execute 之间源命名空间 UID 变了 → 整体 failPlan，LastError 含 "drift" 提示，pending 项标成 skipped。
  - 测试全部 CGO_ENABLED=0 友好，Windows 正常通过。

**备份/还原 GUI 浏览（在现有 handler 包扩展）**

- `backend/internal/httpserver/backup.go`
  - 新增 `listBackups(c)`：集群+分页+可选 `namespace`（默认 `velero`），走 `k8sgateway.ListRaw(velero.io/v1, backups, ns, query)`，投影行（phase/errors/warnings/started/completed/schedule/included namespaces）。
  - 新增 `getBackup(c)`：`{namespace}/{name}` 直接取 manifest。
  - 已有备份 Plan 相关的 Preview/Execute 保留未变。
- `backend/internal/httpserver/restore.go`
  - 新增 `listRestores(c)` / `getRestore(c)`，同上但对应 `velero.io/v1 restores`，每行带 `backup_name`。
- `backend/internal/kubernetes/service.go`
  - 已在此前阶段补齐了 ADR 0004 网关侧的 `customResourceWhitelist`（含 `argoproj.io/v1alpha1 Application`）、`KindToGVR`、`GetRawResource`、`NamespaceExists`、`SourceNamespaceIdentity`、`NamespacedResourceExists`，不再重复。

**HTTP 处理器 + 路由 + 服务初始化**

- `backend/internal/httpserver/gitops.go`：`gitopsHandler {capability, list, get}`。
- `backend/internal/httpserver/copyops.go`：`copyopsHandler {preview, listByCluster, get, execute, listMine}`，含 `previewCopyRequest`（body struct + binding validations）、`executeCopyRequest`、clusterID 双来源（path source_cluster_id / body override）校验，审计目标写入 `CopyPlan`。
- `backend/internal/httpserver/router.go`：在 `clusterScopedRoutes` 内注册：
  - `GET /api/v1/clusters/{cluster_id}/gitops/capability`
  - `GET  /api/v1/clusters/{cluster_id}/gitops/applications`、`GET …/applications/{name}`
  - `GET /api/v1/clusters/{cluster_id}/velero/backups`、`GET …/backups/{namespace}/{name}`
  - `GET /api/v1/clusters/{cluster_id}/velero/restores`、`GET …/restores/{namespace}/{name}`
  - `POST /api/v1/clusters/{cluster_id}/copy-plans/preview`、`GET …/copy-plans`
  - 顶层路由：`GET /api/v1/copy-plans`、`GET /api/v1/copy-plans/{plan_id}`、`POST /api/v1/copy-plans/{plan_id}/execute`。
- `backend/cmd/server/main.go`：
  - 新建 `gitops.NewService(k8sSvc)`。
  - 新建 `copyops.NewService(k8sSvc, copyopsRepo)`（Gorm 连接来自现有 DB pool，clock=time.Now，randFn=crypto rand）。
  - Options 注入对应字段并被 `router.go` 对应注册函数消费。
- `backend/internal/httpserver/openapi_route_test.go`：已在 ADR 0069 阶段扩展，不额外引用新包（测试按路由+OpenAPI 反射比对）。

**OpenAPI 契约**

- `docs/api/openapi.yaml`
  - 新增 11 条 Path：GitOps(3)+Velero(4)+CopyOps(4)。
  - 新增 Schemas：`GitOpsCapability / GitOpsApplication / GitOpsApplicationList / VeleroBackupList / VeleroRestoreList / CopyBundleItem / CopyPlanPreviewRequest / CopyPlanExecuteRequest / CopyItemDiff / CopyPlanResourceItem / CopyPlanDiffSummary / CopyPlan / CopyPlanList` 共 13 个。
  - 全部 Path 带 `bearerAuth`、对应 tags、对应 4xx/5xx 响应。
  - `TestRegisteredRoutesMatchOpenAPI` 在全量 go test 里通过（见门禁验证）。

## 单元测试

- GitOps：`go test ./internal/gitops/…` 绿（6 条以上用例），全 CGO=0。
- CopyOps：`go test ./internal/copyops/…` 绿，覆盖 Preview 全校验分支 + Preview Success 写库 + Destination 缺失 + Execute 幂等重放 + CAS Drift。使用 `inmemRepo` 代替 Gorm SQLite。
- HTTPServer：`go test ./internal/httpserver/…` 全绿，含 `TestRegisteredRoutesMatchOpenAPI`（新增 11 条路由全部命中 OpenAPI 契约）。

## 门禁验证

```
PS C:\BS\aiops-platform> .\scripts\verify-fast.ps1 -Scope Backend
[fast] gofmt check
[fast] go vet ./...
[fast] go test ./...
Fast verification passed in 57.83 seconds (backend=True frontend=False manifests=False).
```

全量后端包（47 个测试套件）全部通过，CGO_ENABLED=0 兼容。

## 授权 / 审计

- 所有 M58 路由挂 `bearerAuth` + 已有 authz 中间件，workspace scoped routes 复用 cluster_id 参数的 ownership 校验。
- copyops Preview 执行前调用 `setAuditClusterID(sourceClusterID)` / `setAuditTarget("CopyPlan", plan.SourceNamespace, plan.ID)`；GUI 用户点击确认的 execute 由 copyops 状态机审计，并由 `LastError`/`Status` 列保留轨迹（配合现有的统一 audit trail 中间件写 `audit_logs`）。
- Confirmation token：仅在 Preview 201 响应里通过 transient 字段返回，Handler 附加 `Cache-Control: no-store`；库表只存 sha256 哈希（与 M22/M23 备份/还原强度一致）。

## 开放项 / 后续

- GitOps 发布事件到 M40 ChangeEvent、M42 ChangeCandidate 关联：已在 ADR 0070 中标记为 post-M58 增强，留给 M59/Phase 4。
- copyops 当前只支持 Create 模式（资源不存在则创建）。Update/Delete 模式需要引入三向合并和逐资源的 CAS（源 live UID == 预览时 source UID），留作后续里程碑。
- Velero Backup/Restore 详情的 `errors / warnings` 明细当前只放列表行计数；如果 GUI 需要 "逐错误行" 展示，另开 `GET …/backups/{name}/errors` 端点更合适。

## 风险与缓解

- **Bundle 大爆炸攻击**：入口先 cap MaxBundle=20（normalize 之前），拒绝超大请求早于 manifest 解析。
- ** torn read（源命名空间被删建）**：CAS SourceNamespaceIdentity Preview→Execute 比对，漂移即 failPlan。
- **已有目标资源被覆盖**：Preflight "already exists" + dry-run 双重 skip；Execute 阶段只对 Pending 项调用 Create，不可能走 update/patch。
- **ArgoCD / Velero 在集群不存在**：Cap 返回 `available=false`，Backups/Restores 返回空列表（零错误 panic）。
