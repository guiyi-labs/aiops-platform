# M11 运维驾驶舱、资源拓扑与控制台视觉升级

- 日期：2026-07-27
- 里程碑：M11
- 状态：Accepted
- 范围：多集群资源态势、selector 拓扑、共享导航、响应式视觉、健康算法测试

## 参考与取舍

本轮参考工作区内多集群管理课程项目的功能组织，重点阅读其“Namespace
列表视角”“集群资源统计”和“Ingress/Service/Pod 拓扑”章节目录，以此确认
成熟 Kubernetes 控制台应优先回答资源规模、健康度和上下游关系。只借鉴产品
结构，不复制其源码或扩大本平台能力边界。

结合本平台 AIOps 定位，最终没有引入任意 YAML、通用资源代理或不受控写操作：
总览和拓扑继续复用服务端固定构造的 Node、Pod、Deployment、Service、Event
只读接口；关系只在同一 Namespace 内由 selector 与 labels 完全匹配时成立。

## 实现

1. Dashboard 升级为实时集群态势工作台：
   - 支持已启用集群切换和一键刷新；
   - 同时读取 Node、Pod、Deployment、Service 和 Event；
   - 展示 Node Ready、Pod Healthy、Deployment Available、Warning Event、
     待处置诊断和解决率；
   - 工作负载健康条、最近 Warning、诊断队列和平台依赖共用一个密集工具面板；
   - 提供资源拓扑、资源列表、事件中心和诊断队列的快捷入口。
2. 新增 `/topology` 资源拓扑：
   - 支持集群、Namespace 和名称筛选；
   - 以 Service -> Pod <- Deployment 三列展示真实资源；
   - 选择任一资源后高亮 selector 关联对象，并显示关联数量；
   - 无 selector、空 selector 或跨 Namespace 标签相同均不生成推测关系；
   - Pod 与 Deployment 使用统一健康算法显示 healthy/warning/critical。
3. 共享控制台壳层升级：
   - 导航分为“工作台”“资源观察”“分析与治理”；
   - 增加资源拓扑入口、角色信息、环境连接状态和紧凑顶栏；
   - 使用中性工作区、深色导航和绿/蓝/黄/红多状态色，移除现有渐变；
   - 桌面保持高信息密度，窄屏收敛为图标导航、双列指标和工具内部滚动。
4. 新增 `resource-health.ts` 与 5 个测试：
   - Pod phase、container waiting reason 和 readiness 分级；
   - Deployment 正常、部分就绪、全不可用和 scale-to-zero 语义；
   - selector 全量标签匹配与 Namespace 隔离。

## 真实数据与浏览器验收

保留的 `demo-kind-20260726-170601` 集群凭据按既有安全设计为短期 token。
验收前使用平台原子凭据轮换接口在内存中更新 1 小时只读 ServiceAccount token，
补齐 kind 所需 `tls-server-name` 后探测恢复为 Ready/v1.34.0。token、kubeconfig
和管理员密码均未写入临时文件或命令输出，集群 RBAC 与工作负载未改变。

真实总览读取到 1/1 Ready Node、11/14 Healthy Pod、4/6 Available
Deployment、4 个 Service、19 条 Warning Event 和 3 条待处置诊断。选择
`healthy-nginx` Service 后，拓扑只高亮同 Namespace 的 1 个 Service、1 个 Pod
和 1 个 Deployment，检查器显示关联数量 1/1/1。

| 页面与尺寸 | 结果 |
|---|---|
| 集群态势桌面 | 六项资源/诊断指标、三类健康条、Warning 和诊断队列真实数据显示正确 |
| 资源拓扑桌面 | 三列资源、健康色、选择高亮和关系检查器显示正确 |
| 集群态势 390x844 | 文档 `clientWidth=375`、`scrollWidth=375`，六项指标双列且无重叠 |
| 资源拓扑 390x844 | 页面无横向溢出；678px 关系画布只在 277px 内容区内部滚动 |

最终浏览器 warning/error 日志为空，临时 390x844 视口覆盖已恢复。

## 自动验证

完整 `scripts/verify.ps1` 于 2026-07-27 10:06:26 +08:00 通过，耗时
235.92 秒：Go 1.25 Docker 工具链全包 vet/test/build、前端 typecheck、
9 个 Vitest 文件/33 个用例、生产构建、Compose 三服务健康、Kustomize
16/5/7/3 与运行态 HTTP 检查全部通过。证据为
`.artifacts/verification/verify-20260727-100626.json`。

## 后续能力路线

- 将当前综合工作负载页拆成 Node、Deployment、Pod、Service 的可深链资源视图。
- 在明确只读合同后增加 ConfigMap、Ingress、PVC 和 StorageClass 观察页；Secret
  只显示元数据，不返回 data。
- 引入 Metrics API/Prometheus 后再展示 CPU、内存和趋势，当前不伪造利用率。
- 拓扑后续可增加 Ingress -> Service -> EndpointSlice -> Pod，但必须先补对应的
  固定只读 API、RBAC、响应上限和合同测试。
