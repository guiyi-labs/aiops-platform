# M12 可深链资源工作台与统一详情视图

- 日期：2026-07-27
- 里程碑：M12
- 状态：Accepted
- 范围：Node、Deployment、Pod、Service 分类视图，固定详情 API，URL 状态，响应式详情抽屉

## 设计边界

M12 延续 ADR 0004 的受限只读网关：浏览器只能访问服务端固定注册的资源
列表、单资源详情和 Pod 日志路由。页面不接受任意 Kubernetes API path、YAML
或动态资源类型，也没有新增写操作。Node 使用集群作用域名称；Deployment、Pod
和 Service 始终显式携带 Namespace 与名称。

## 实现

1. 新增三个 HTTP 详情合同，与现有 Pod 详情组成完整的四类资源读取面：
   - `GET /api/v1/clusters/{cluster_id}/nodes/{name}`；
   - `GET /api/v1/clusters/{cluster_id}/deployments/{namespace}/{name}`；
   - `GET /api/v1/clusters/{cluster_id}/services/{namespace}/{name}`；
   - Gin 注册与 OpenAPI 3.0.3 同步，404 继续映射为统一的
     `RESOURCE_NOT_FOUND`。
2. `/workloads` 从综合长页面收敛为四个互斥资源标签：
   - 共用集群、Namespace 和名称筛选上下文；Node 自动禁用 Namespace；
   - Pod 表展示 phase/reason、Ready、重启、节点和日志；
   - Deployment 表展示副本、Available/Updated 和镜像；
   - Service 表展示类型、ClusterIP、端口和 selector；
   - Node 表展示 Ready、调度、InternalIP、kubelet 和容器运行时。
3. 点击资源行打开统一只读详情抽屉：
   - Pod 包含地址、容器、镜像、Conditions、Labels、当前/previous 日志和诊断；
   - Deployment 包含六项副本计数、镜像、selector、Labels 和诊断；
   - Service 包含地址类型、端口映射、selector、Labels 和后端诊断；
   - Node 包含调度状态、系统版本、地址、capacity/allocatable、Conditions、Labels
     和节点诊断。
4. 详情选择由 URL 恢复，例如
   `/workloads?cluster=23&kind=Service&namespace=aiops-demo&name=healthy-nginx`。
   刷新可恢复同一抽屉；关闭后删除 `kind/namespace/name`，只保留集群上下文。
5. Dashboard 打开资源工作台时保留当前集群；Topology 关系检查器增加真实资源详情
   入口。前端 API 测试覆盖四类固定详情 path 编码和所有资源列表的名称筛选。

## 真实数据与浏览器验收

保留的 kind v1.34.0 演示集群返回 14 Pod、6 Deployment、4 Service 和
1 Node。桌面 1280x720 逐一打开四类资源，详情 API 均返回真实对象：
`coredns` Pod/Deployment、`healthy-nginx` Service 和
`aiops-test-control-plane` Node。Pod Conditions、Service 端口以及 Node
capacity/allocatable 均正确呈现。

直接刷新 Node 深链接后抽屉恢复成功；从 Topology 选择 `healthy-nginx` 并进入
详情后，URL 和 Service 抽屉一致。关闭详情后的 URL 为
`/workloads?cluster=23`。

390x844 检查中，文档 `scrollWidth=clientWidth=375`；930px 资源表只在
277px 工具区内部滚动。详情抽屉 `x=0`、宽 375px，操作栏宽 360px，未产生
页面级横向溢出、控件重叠或文字遮挡。最终页面 warning/error 日志为空，临时
视口覆盖已恢复。

## 自动验证

完整 `scripts/verify.ps1` 于 2026-07-27 10:38:59 +08:00 通过，耗时
186.24 秒：Go 1.25 Docker 工具链全包 vet/test/build、前端 typecheck、
9 个 Vitest 文件/35 个用例、生产构建、Compose 三服务健康、Kustomize
16/5/7/3 与运行态 HTTP 检查全部通过。证据为
`.artifacts/verification/verify-20260727-103859.json`。

## 后续能力路线

- 为 Ingress、PVC、StorageClass 和 ConfigMap 元数据建立独立的固定只读合同；
  Secret 只允许元数据和键名，不返回 data。
- 引入 Metrics API 或 Prometheus 后增加真实 CPU、内存和趋势，不根据 requests
  或 limits 伪造利用率。
- 在拓扑中增加 Ingress -> Service -> EndpointSlice -> Pod，并保持 Namespace、
  selector、响应大小和 RBAC 边界。
