# 2026-07-17 Core Resource Read Path

## Scope

- 扩展集群注册表，增加固定路径、只读、带响应上限的 Kubernetes GET Gateway。
- 增加 Namespace、Pod 列表、Pod 详情、Event 和当前/previous 日志 API。
- 所有请求显式携带 `cluster_id`，停用集群不可查询。
- 列表统一支持 selector、名称筛选、排序、页码和 `items/total/remaining` 响应。
- 前端增加工作负载页面、集群与 Namespace 切换、Pod 状态、Warning Event 和日志抽屉。

## Verification

- `go test ./...` 通过；`go build ./cmd/server` 通过。
- 后端测试覆盖集群禁用、Namespace 路径、selector 转发、名称过滤、排序分页和日志参数。
- `pnpm typecheck` 通过；Vitest 4 个测试文件、8 个测试通过；`pnpm build` 通过。
- 本地 HTTPS 假 Kubernetes API 端到端验证通过：探测状态 `ready`、版本 `v1.33.2`、2 个 Namespace、Pod `api-1`、BackOff Event 和容器日志均经平台 API 返回。
- 浏览器验证工作负载列表、Warning Event 与日志抽屉；390px 视口无页面横向溢出，日志抽屉宽度为 390px，控制台无错误或警告。
- 验证后删除临时集群记录并停止本地 API、平台和前端进程。

## Security Boundaries

- 不接受前端传入上游路径或 HTTP 方法。
- 资源响应最大 10 MiB，日志最大 1 MiB，日志行数限制 1–2000。
- kubeconfig 仅在后端内存中解密，不进入资源响应或前端存储。

## Deferred

- 引入 client-go 及 Kubernetes fake client。
- Node、Deployment、Service 与 YAML 摘要。
- Deployment 扩缩容、滚动重启、Pod 删除和操作审计。
- Event 聚合、异常候选识别与首条 ImagePullBackOff 规则。
