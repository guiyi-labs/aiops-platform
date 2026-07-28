# M14 Ingress 到后端完整资源拓扑

- 日期：2026-07-27
- 里程碑：M14
- 状态：Accepted
- 范围：固定 EndpointSlice 列表 API、Ingress/Service/EndpointSlice/Pod/Deployment 拓扑、真实 kind 与响应式验收

## 边界与取舍

当前环境未部署 Metrics API 或 Prometheus，因此本阶段不伪造 CPU、内存或趋势。
M14 继续遵守 ADR 0004 的固定只读边界，只补充
`GET /api/v1/clusters/{cluster_id}/endpointslices`。浏览器不能提交 Kubernetes API
path、GVK、原始 YAML 或写操作；列表沿用最大 100 条分页和 10 MiB 上游响应上限。

公共 EndpointSlice 模型只保留 metadata、addressType、端口 name/protocol/port、
endpoint addresses、ready/serving/terminating、nodeName、targetRef，以及从
`kubernetes.io/service-name` 标签派生的 Service 身份。未知字段不会序列化输出，
null ports/endpoints 会规范化为空数组。

## 实现

1. 后端新增 EndpointSlice 固定列表服务、Gin 路由、OpenAPI 合同和序列化测试；
   目标集群 observer RBAC 已具备 discovery.k8s.io EndpointSlice get/list，无需扩大权限。
2. 前端新增 EndpointSlice 类型与 API 客户端，并将关系推断抽成纯函数：
   - Ingress backend name 只连接同 Namespace Service；
   - Service 只按同 Namespace 和精确 `serviceName` 连接 EndpointSlice；
   - EndpointSlice 只连接 kind=Pod 且名称/Namespace 精确匹配的 targetRef；
   - Service 仅在没有匹配 EndpointSlice 时使用非空完整 selector 回退；
   - Deployment 到 Pod 继续使用同 Namespace 完整 selector 匹配。
3. `/topology` 升级为 Ingress、Network（Service + EndpointSlice）、Runtime 和
   Workloads 四条扫描通道。选择任意节点会沿真实关系高亮连通链路，检查器统计
   五类资源；EndpointSlice 可检查但不伪造工作台详情。
4. 资源摘要扩展为 Ingress、Service、EndpointSlice、Pod、Deployment 和 Warning
   Event。常见桌面宽度将检查器放到画布下方，超宽屏并排；移动端只允许拓扑画布
   内部横向滚动。

## 回归修复

首次真实浏览器验收发现，无后端 Service 的 EndpointSlice 可能返回
`endpoints: null`，页面读取 `.length` 后白屏。后端现将空集合规范化为 `[]`，前端
关系和展示函数同时使用空值防御，并新增 Go 回归测试。修复后
`service-without-endpoints` 显示 `0/0 Ready` 和异常状态，不再中断整个页面。

## 测试与真实数据

新增纯关系测试覆盖命名/数字 Ingress 端口、同 Namespace 限制、标准 Service 标签、
精确 Pod targetRef、not-ready endpoint、selector 回退、空 selector 和跨 Namespace。
当前自动化基线为 114 个 Go `Test*` 入口、11 个 Vitest 文件/44 个用例。

`scripts/demo-up.ps1` 在 kind v1.34.0 通过并保留平台集群 ID 32：
`demo-kind-20260727-130453`。固定 API 返回业务 Namespace 的 1 个 Ingress 和 2 个
EndpointSlice；`healthy-nginx` 保留 `http:8080/TCP`、Ready=true 与精确 Pod
targetRef。三条诊断、rollout restart 幂等和 RBAC `yes/yes/no/no` 同时通过。证据为
`.artifacts/e2e-kind/e2e-kind-20260727-130455.json` 与
`.artifacts/demo/demo-ready-20260727-130455.json`。

## 浏览器验收

1280x720 下，文档 `clientWidth=scrollWidth=1265`，拓扑画布
`clientWidth=scrollWidth=943`。选择 `healthy-nginx` Ingress 后，四通道高亮数量为
1/2/1/1，检查器中的 Ingress、Service、EndpointSlice、Pod、Deployment 均为 1。

390x844 下，文档 `clientWidth=scrollWidth=375`；六项摘要保持两列，无重叠。拓扑
画布 `clientWidth=277`、`scrollWidth=928`，实测可滚动到 `scrollLeft=520`，页面本身
仍无横向溢出。移动端选择同一 Ingress 后 1 个 selected、5 个 related，五类统计
仍各为 1。修复后的当前前端资源包 warning/error 日志为空，临时视口已恢复。

## 最终门禁

最终 `scripts/verify.ps1` 于 2026-07-27 13:23:55 +08:00 通过，耗时
154.69 秒：Go 1.25 Docker 工具链全包 vet/test/build、前端 typecheck、11 个
Vitest 文件/44 个用例、生产构建、Compose 三服务 healthy、Kustomize
16/5/10/3 与运行态 HTTP 检查全部通过。证据为
`.artifacts/verification/verify-20260727-132355.json`。

敏感材料扫描未匹配私钥、Bearer/JWT 或 kubeconfig client certificate/key data。

## 后续能力路线

- 接入 Metrics API 或 Prometheus 后增加真实 CPU、内存与趋势。
- 在独立威胁分析后评估 Secret 仅元数据/键名合同，禁止返回 data。
- 人工确认作者身份和范围后创建初始 Git 基线，再重采集绑定 revision 的答辩截图。
