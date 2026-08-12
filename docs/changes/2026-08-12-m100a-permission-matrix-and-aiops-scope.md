# M100-A：路由权限矩阵生成与差异门禁 + AIOps 查询维度集群授权强制

- Date: 2026-08-12
- Status: Complete
- Scope: M100 第一步——按路线“全路由角色/Workspace/Cluster/Namespace 权限矩阵生成与差异检查”落地矩阵生成器与差异门禁，并修复矩阵暴露出的首个真实授权缺口：`/aiops` 读路由可按 `cluster_id` 越权读取任意集群数据。

## Context

M100 要求“未授权读写路径的契约测试覆盖率达到 100%，前后端导航限制不作为唯一安全措施”。现有 RouteDescriptor 注册表（routeTable）记录每条路由的方法/角色/审计元数据，但没有面向文档的权限矩阵，也没有对 `/aiops`（signals/slos/correlation/topology）读路由的集群授权校验——这些路由挂在 `/api/v1/aiops` 组下，组上只有 `withAuthentication`，处理器直接解析 `?cluster_id=` 并返回数据，任何已认证用户都可以探测任意集群的 SLO/信号/关联案例（违反 M35 反泄漏标准）。

## What Changed

### 权限矩阵生成器与差异门禁

- `backend/internal/httpserver/permission_matrix.go`：`BuildPermissionMatrix()` 从 routeTable 派生完整矩阵（方法/路径/角色/scope/审计动作），scope 按路径模板分类（`workspace`/`cluster`/`namespace`/`none`，namespace 优先于 cluster，因组嵌套）；`RenderMarkdown()` 输出确定性文档（汇总 + 按路径排序明细），不含时间戳。
- `backend/internal/httpserver/permission_matrix_test.go`：
  - `TestPermissionMatrixMatchesCommittedDocument` 差异门禁——已提交的 `docs/security/permission-matrix.md` 必须与实时 routeTable 生成结果一致，`-update` 标志重新生成。
  - 结构不变量：namespace 路由必含 `:cluster_id`、workspace 路由不得携带 cluster/namespace 键、角色封闭集（四角色）、条目排序与去重。
- `backend/internal/httpserver/router_harness_test.go`：提取完整生产 Options 的 `buildFullEngine(t)`（此前内联在 OpenAPI 契约测试中），OpenAPI 契约测试与权限矩阵测试共享同一 route 注册面。
- 生成 `docs/security/permission-matrix.md`：279 条路由、83 条角色受限、157 条已审计、workspace 13 / cluster 82 / namespace 32 / none 152。

### AIOps 查询维度集群授权强制（矩阵暴露的缺口修复）

- `backend/internal/httpserver/authz_middleware.go`：新增 `requireClusterQueryAccess(service)` 中间件——`?cluster_id=` 存在时校验 `CanAccessCluster`，同时存在 `?namespace=` 时再校验 `CanAccessNamespace`；拒绝一律返回 404（M35 反泄漏），shape 校验留给处理器（400）；service 为 nil（开发模式）放行。
- `backend/internal/httpserver/router.go`：`/aiops` 组挂上该中间件，signals/overview/slos/correlation/topology 等所有 aiops 查询路由的集群维度授权生效。
- `backend/internal/httpserver/authz_query_scope_test.go`：8 个用例覆盖放行/拒绝/admin 绕过/shape 透传/namespace 粒度/无 authz 透传。

## Verification

- Backend：`go test ./...`、`go vet ./...`、`golangci-lint run ./...` 全绿（新增 12 个测试用例；OpenAPI 契约测试复用共享 harness 后仍全绿）。
- 运行时冒烟（重建 backend 镜像，容器 healthy）：
  - 无授权的 viewer：`GET /aiops/signals?cluster_id=1`、`slos?cluster_id=1`、`correlation/cases?cluster_id=1` 均 404（修复前返回数据）。
  - 授权后（cluster grant）：同路径均 200；system_admin 天然放行。
  - 未带 `cluster_id` 的平台级读（signals/slos）保持 200，`correlation/cases` 保持 400（handler 要求 cluster_id）。
  - 测试 viewer 已禁用清理。
- 差异门禁：修改路由/角色后未同步文档会失败（`go test ./internal/httpserver -run TestPermissionMatrixMatchesCommittedDocument`）。

## Risks / Notes

- 未带 `cluster_id` 的无 scope aiops 读（如 `GET /aiops/signals` 不带 cluster_id）仍返回跨集群数据；授权范围内的数据过滤需服务层注入可见集群列表（M100 后续步骤，已在中间件注释与矩阵文档说明）。
- SLO 按 id 读（`/slos/:id`、`/slos/:id/evaluations`）的集群归属校验需要服务层带出定义归属后比对授权，属后续步骤。
- `/aiops/topology/*` 生产 main.go 未接线（ROUTE_NOT_FOUND），为既有状态；接入时同受本中间件保护。
- 下一步：M100-B（会话/密码/auth_version 失效旅程复验）、M100-C（敏感字段静态扫描与日志脱敏门禁）。
