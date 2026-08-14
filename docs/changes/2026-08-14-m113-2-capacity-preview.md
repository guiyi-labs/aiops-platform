# M113-2 容量感知预览：节点按剩余资源排序的只读适配评估

- Date: 2026-08-14
- Status: Complete
- Scope: M113 第二个切片——对候选资源（节点）按剩余 CPU、内存、GPU、存储估算排序，展示"为什么适配/为什么不适适配"与数据更新时间；只生成 remediation 预览，不创建 Deployment、不改 YAML、不开终端。

## Context

- 上位路线：`docs/development-roadmap-post-m110.md` Track D 第 156–157 行：容量感知预览，对候选资源按剩余 CPU、内存、GPU、存储和网络约束排序，展示"为什么适配/为什么不适配"和数据更新时间；只生成 remediation 预览（借鉴外部 scheduler 的容量排序结构，但去掉一键部署 / YAML 编辑）。
- 数据来源：`internal/capacity`（M60 容量趋势）回答"集群何时耗尽"，而本切片回答"候选工作负载当前能放哪些节点"——两者互补。本切片直接读取 `kubernetes` gateway 的 Node（`status.allocatable`）与 NodeMetrics（用量），纯只读实时查询，沿 M99-D 显式覆盖度 / 资源上下文契约（scope / observed_at / freshness / 空样本语义）。

## What Changed

### 新增 `backend/internal/capacitypreview/`（纯包，ADR 0004）
- `model.go`：`WorkloadRequest`（cpu nanocores / mem bytes / gpu 卡数 / storage bytes）、`NodeObservation`（allocatable map + usage 字符串 + 观测时间戳 + 可调度/Ready 状态）、`Constraint`（resource / status / remaining / required / missing_names / note）、`NodePreview`、`Preview`（cluster_id / evaluated_at / request / scope / observed_at / nodes_total / nodes_schedulable / fit_count / fail_closed / nodes）。
- `Evaluate(clusterID, request, bundle, evaluatedAt)`：确定性地逐节点评估 CPU / 内存 / GPU / 存储四个约束（GPU 与存储仅在请求 > 0 时评估），`remaining = allocatable - usage`；约束满足计 `satisfied`，不足计 `violated`，缺数据计 `unknown`（**fail-closed：带 unknown 的节点不计入 fit_count，空样本不视为健康**）；排序为：适配节点优先（按剩余 CPU 头寸降序）→ 未知数少优先 → 名称序；`Scope` 沿 M112 资源上下文契约。
- 数量解析复用平台既有单位约定：CPU nanocores / 内存 bytes（`finops.QuantityFromResourceMap` + `k8s.io/apimachinery/pkg/api/resource`），GPU 直接用 `resource.ParseQuantity` 计数。
- `service_test.go`：排名正确性（更大头寸节点排前）、未知约束 fail-closed、违约解释存在、GPU 约束满足、空 bundle 返回 `ErrEmpty`、CPU/内存单位解析。

### 新增 HTTP 层 `backend/internal/httpserver/capacity_preview.go`
- `capacityPreviewHandler`：验证请求（cluster_id 必填、资源请求非负且至少一个 > 0）、无 gateway 时 503；读取 `Nodes` + `NodeMetrics`（分页），组装 `NodeObservation`（`Schedulable = !Spec.Unschedulable`、`Ready` 由 Ready 条件推导、`AllocatableObservedAt = CreationTimestamp`、`UsageObservedAt = 指标时间戳`）；调用 `capacitypreview.Evaluate` 返回 JSON。
- 路由 `POST /api/v1/optimization/capacity/preview`（AuthRequired；读操作，无 RequiredRoles），AuditAction `optimization.capacity.preview` / AuditResource `Cluster`，仅在 `options.Optimization != nil && options.Kubernetes != nil` 时注册。
- `capacity_preview_test.go`：坏 body / 缺 cluster_id / 全零请求 / 负请求 / 无 gateway 五种校验路径。

### OpenAPI / 权限矩阵 / typegen
- `docs/api/openapi.yaml`：新增 `/api/v1/optimization/capacity/preview`（operationId `capacityPreview`）与 `CapacityPreviewRequest` / `CapacityPreview`（含节点 constraints 枚举 satisfied/violated/unknown）schema。
- `docs/security/permission-matrix.md`：重生成，登记 `optimization.capacity.preview`。
- `frontend/src/api/openapi.d.ts`：typegen 重生成。

### 前端
- `frontend/src/types/optimization.ts`：`CapacityPreviewRequest` / `CapacityConstraint` / `CapacityPreviewNode` / `CapacityPreview`。
- `frontend/src/api/optimization.ts` + `optimization.test.ts`：`getCapacityPreview(token, request)`（POST，nodes 空值归一为 []）+ 客户端测试。
- `frontend/src/views/OptimizationView.vue` 容量 tab：新增"容量感知预览"面板——表单（CPU 核 / 内存 GiB / GPU 卡 / 存储 GiB）+「评估适配节点」按钮；结果区展示适配节点数 / 数据观测时间（fail-closed 提示）、按排名列出节点（适配/不适配/数据不足徽标）+ 约束明细（每资源满足/不足/无法评估 + 余量或缺失字段名）+ 数据更新时间；底部注明"仅预览，不执行调度、不创建 Deployment、不修改 YAML"。
- `frontend/src/styles/base.css`：新增 `.phase-badge.pass` / `.phase-badge.warn`（沿用现有 token）。

## Verification

- `go test ./...`：69 包全绿（新增 capacitypreview 包 + httpserver 校验测试）。
- `pnpm typegen` / `pnpm typecheck` / `pnpm lint` / `pnpm test -- --run`（**153** 通过）/ `pnpm build`：全绿。
- OpenAPI 路由匹配 + 权限矩阵一致性：通过。
- 只读性：纯包无 cluster 访问、无状态变更、无写路径；handler 只走 Nodes/NodeMetrics 只读 gateway；前端无任何写按钮。
- 资源上下文契约：返回 `scope`（cluster:n:nodes:allocatable+usage）、`observed_at`、每节点 `freshness`；缺用量样本的节点约束为 unknown 且 fail-closed，不视为健康。

## Risks / Notes

- GPU 用量不由 metrics-server 暴露，GPU 头寸按 allocatable 计数（真实且保守）；存储同理（ephemeral-storage allocatable）。
- `FailClosed` 语义：存在任意 unknown 约束即置 true，前端据此提示"存在缺样本节点"；不会把缺样本节点误判为可适配。
- 网络约束：本切片按路线聚焦 CPU/内存/GPU/存储四类，网络约束留待 M114 事件/指标深化或后续扩展（当前 gateway 无网络带宽采集面）。
- 纯包层面与 `internal/capacity` 的 `Evaluate` 命名同构但职责不同（集群趋势 vs 节点适配），无冲突。
- M113 剩余：M113-1 已提交（finding→runbook 导航）；本切片为 M113-2；M113-3 巡检趋势与覆盖率度量待做。