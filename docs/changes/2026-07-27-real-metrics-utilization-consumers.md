# M16 Metrics available path 与资源消费排行

- 日期：2026-07-27
- 里程碑：M16
- 状态：Accepted
- 范围：可选 Metrics Server kind fixture、真实 Node 利用率、Pod CPU/内存消费排行、真实 kind 与响应式验收

## 目标与边界

M16 完成 M15 已定义 Metrics 合同的真实 available path。Metrics Server 仍不是
平台依赖，也不加入默认平台或目标集群 Kustomize。利用率只由平台已经读取的真实
quantity 与同名 Node allocatable 计算；缺少有效分母时不显示百分比。Pod 排行只
展示有界实时样本，不持久化时序指标，也不把未加载或缺失样本解释为零。

## 实现

1. Node Metrics 与核心 Node 按名称匹配，CPU 和内存分别使用对应
   `status.allocatable`；缺失、非法和零值均返回无百分比状态。
2. Dashboard 新增 CPU/Memory 分段排行，按全部已加载 Pod container usage 聚合
   排序并显示前五名，同时陈述 `loaded / total Pods` 覆盖率。
3. 每个排行项深链到 `/workloads`，显式保留 cluster、kind=Pod、Namespace 和 name，
   到达后自动打开精确 Pod 详情。
4. 新增 `scripts/e2e-metrics-kind.ps1`。脚本校验 vendored Metrics Server v0.8.0
   清单的 SHA-256，等待 Node/Pod 样本，再复用 `e2e-kind.ps1 -RequireMetrics`
   验证平台合同、诊断、处置和 RBAC。
5. fixture 仅为本地 kind 增加 `--kubelet-insecure-tls`，并使用可达的固定镜像
   `registry.cn-hangzhou.aliyuncs.com/google_containers/metrics-server:v0.8.0`。
   上游清单保持 byte-equivalent，默认 Kustomize 不引用此目录。

## 自动化验证

- 全量门禁：`.artifacts/verification/verify-20260727-143242.json`
- 结果：242.56 秒，119 个 Go `Test*` 入口、12 个 Vitest 文件/51 个用例、前端
  typecheck/build、Go build、Compose 三服务 healthy、Kustomize `16/5/10/3`、
  readiness 与 frontend proxy 全部通过。
- 前端资源指标单元测试新增 name-matched allocatable、非法/缺失分母、利用率格式、
  Pod CPU/内存排序和有界采样覆盖。
- 部署合同测试确认 fixture、patch、README 与 E2E 入口存在，且 fixture 不进入默认
  平台、managed-cluster、demo 或 diagnosis Kustomize。
- 最终运行态复核确认三项 Compose 服务 healthy，后端近 30 分钟无 warning/error；
  排除生成目录后扫描 315 个仓库文件，未发现私钥、JWT、CA/key payload 或常见云密钥。

## 真实 kind 与 Metrics Server

- Metrics 证据：`.artifacts/metrics-e2e/metrics-e2e-20260727-142714.json`
- 下游 E2E：`.artifacts/e2e-kind/e2e-kind-20260727-142714.json`
- 环境：Kubernetes v1.34.0，Metrics Server v0.8.0，
  `demo-kind-20260727-142712`，平台 cluster ID 34，一小时观察凭据创建于约
  14:27 +08:00。
- vendored 上游清单 SHA-256：
  `ff64d1a13b9ac3b0635f0dd985815fb44c23eed4706c04e5db1daadf6bc0a83b`。
- kind 直连与平台合同均返回 1 个 Node、12 个 Pod；样例 Node quantity 为
  CPU `343990071n`、内存 `886676Ki`。
- 三条演示诊断、Deployment rollout restart 和相同幂等键重放继续通过；保留环境
  供 available-path 浏览器验收使用。

## 浏览器验收

- Desktop 1280x720：文档 `scrollWidth/clientWidth=1265/1265`，无指标不可用提示；
  CPU `479m / 3.0% allocatable`，Memory `911.4 MiB / 11.7% allocatable`，
  排行覆盖 `12 / 12 Pods`。
- CPU 切换到 Memory 后前五名按真实内存量重排；首项显示 `260.8 MiB`。点击首项
  到达
  `/workloads?cluster=34&kind=Pod&namespace=kube-system&name=kube-apiserver-aiops-test-control-plane`，
  并自动打开精确 Pod 详情。
- Mobile 390x844：文档 `375/375`，无越界元素；排行区域 279px，内部单列 277px，
  CPU/Memory 控件各 117.5px，五行均保持固定 72px 高度且无文本/控件重叠。
- 两个视口均无浏览器 warning/error 日志；临时 viewport 最终恢复为 1280x720，
  应用标签页保留为交付页面。

## 后续范围

- M17 按 `docs/roadmap.md` 增加常用工作负载、弹性和配额的固定只读合同。
- 历史趋势仍需先定义采样间隔、保留期、缺口语义和多集群成本，本阶段不持久化。
- Prometheus 接入属于后续架构选型，不替代当前 Metrics API 的显式可用/不可用语义。
