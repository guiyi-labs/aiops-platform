# M13 扩展只读资源工作台与关联事件

- 日期：2026-07-27
- 里程碑：M13
- 状态：Accepted
- 范围：Ingress、PersistentVolumeClaim、StorageClass、ConfigMap 键名，八类资源关联事件，分组资源工作台

## 安全边界

M13 继续遵守 ADR 0004 的固定路由边界。浏览器不能提交 Kubernetes API path、
资源类型、原始 YAML 或写操作；服务端只新增四类列表和详情 GET 合同。ConfigMap
先解码到内部结构，再映射为 metadata、immutable、排序后的 `dataKeys` 与
`binaryDataKeys`。StorageClass 公共模型不包含 `parameters`。Secret、ConfigMap
值、StorageClass 参数以及任意动态 API 代理均不在本阶段范围内。

单元测试用显式哨兵值验证 ConfigMap data/binaryData 和 StorageClass parameters
不会进入序列化响应。敏感材料扫描未发现私钥、JWT、长 Token、CA payload 或
Bearer 凭据。

## 实现

1. 后端新增 Ingress、PersistentVolumeClaim、StorageClass 和 ConfigMap 的固定
   列表/详情服务、Gin 路由与 OpenAPI 3.0.3 合同；404 延续统一
   `RESOURCE_NOT_FOUND` 映射。
2. `/workloads` 保持兼容，并升级为两级“资源工作台”：
   - 工作负载：Pod、Deployment、Node；
   - 网络：Service、Ingress；
   - 存储：PVC、StorageClass；
   - 配置：ConfigMap。
3. Node 与 StorageClass 按集群作用域深链；其余资源必须显式携带 Namespace。
   新详情覆盖 Ingress host/path/backend/TLS/load balancer、PVC 申请/容量/访问模式、
   StorageClass 供应器/回收/绑定/扩容，以及 ConfigMap 键名。
4. 八类详情统一并发读取 Kubernetes Events，按 involvedObject 的精确名称、kind
   和适用的 Namespace 过滤，再按 series.lastObservedTime、eventTime、
   lastTimestamp、firstTimestamp、creationTimestamp 倒序展示。事件读取失败不会
   阻断资源详情。
5. 目标集群 observer RBAC 增加四类资源的 `get/list`，仍不包含变更动词；演示
   清单增加 `healthy-nginx` Ingress、32Mi `demo-cache` PVC 和
   `demo-runtime-profile` ConfigMap。

## 诊断回归修复

真实演示准备暴露了一个既有规则优先级回归：通用 Pending 规则先于
ImagePullBackOff，导致处于 Pending phase 的镜像拉取故障被错误分类。M13 将
OOMKilled、ImagePullBackOff、CrashLoopBackOff 放在通用 Pending 前，并修正
PodScheduled=False 条件过滤逻辑；新增测试锁定具体容器故障优先级。

`scripts/demo-up.ps1` 随后在 kind v1.34.0 通过：三条规则分别命中
`pod.image_pull_backoff.v1`、`pod.crash_loop_backoff.v1` 和
`service.no_ready_endpoints.v1`，rollout restart 成功且同幂等键未重复执行，
RBAC 为 `yes/yes/no/no`。证据为
`.artifacts/e2e-kind/e2e-kind-20260727-114800.json` 和
`.artifacts/demo/demo-ready-20260727-114800.json`。

## 浏览器验收

真实 kind 返回 14 Pod、6 Deployment、1 Node、4 Service、1 Ingress、1 PVC、
1 StorageClass 和 15 ConfigMap。桌面端逐一验证四类新增详情；ConfigMap DOM 只
出现 `APP_MODE`、`LOG_LEVEL`、`REGION` 键名，不出现值；PVC 显示
WaitForFirstConsumer 事件；StorageClass 不显示 parameters；Ingress 路由正确
指向 `healthy-nginx:http`。原有 ImagePullBackOff Pod 抽屉显示 4 条精确关联
事件并按最新观察时间排序。

390x844 验收中，文档 `scrollWidth=clientWidth=375`，分类导航宽 279px；930px
资源表只在 277px 容器内滚动。详情抽屉 `x=0`、宽 375px，内部
`scrollWidth=clientWidth=360`。页面无控件重叠、文字遮挡和 warning/error 日志，
临时视口已恢复。

## 自动验证

最终 `scripts/verify.ps1` 于 2026-07-27 11:51:31 +08:00 通过，耗时
154.08 秒：Go 1.25 Docker 工具链全包 vet/test/build、111 个 Go `Test*`
入口、前端 typecheck、10 个 Vitest 文件/38 个用例、生产构建、Compose 三服务
healthy、Kustomize 16/5/10/3 与运行态 HTTP 检查全部通过。证据为
`.artifacts/verification/verify-20260727-115131.json`。

## 后续能力路线

- 接入 Metrics API 或 Prometheus 后提供真实 CPU、内存与历史趋势。
- 评估 Secret 仅元数据/键名合同；在完成威胁分析前不进入实现。
- 拓扑增加 Ingress -> Service -> EndpointSlice -> Pod 关系。
- 初始 Git 基线确认后重采集绑定 revision 的演示截图与发布标签。
