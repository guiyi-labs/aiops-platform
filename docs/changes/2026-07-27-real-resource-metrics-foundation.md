# M15 真实资源指标基础

- 日期：2026-07-27
- 里程碑：M15
- 状态：Accepted
- 范围：固定 Node/Pod Metrics API、真实 CPU/内存绝对用量、可选能力降级、真实 kind 与响应式验收

## 目标与边界

M15 在现有固定 Kubernetes 只读网关上增加资源指标基础，不安装或管理 Metrics
Server，也不开放任意 API group、version、path 或写操作。Dashboard 只陈述目标
集群实际返回的 CPU/内存 quantity；没有 allocatable、request 或 limit 等真实分母
时不计算利用率百分比。Metrics API 缺失不能使 Node、Pod、Deployment、Service、
Event 和诊断汇总失效。

## 实现

1. 新增固定有界接口：
   - `GET /api/v1/clusters/{cluster_id}/metrics/nodes`
   - `GET /api/v1/clusters/{cluster_id}/metrics/pods`
   - Pod 支持固定 Namespace 路径；两者支持 name、分页、排序和 selector 约束。
2. 公共模型只保留 metadata、timestamp、window、CPU/memory usage 与 Pod container
   name。上游响应仍限制为 10 MiB，分页最大 100。
3. metrics 列表 404 后读取固定 discovery 根路径确认 capability；仅根路径也 404 时
   映射为 HTTP 424 `METRICS_API_UNAVAILABLE`。API 存在时的 Namespace/resource
   404、集群不存在、停用和其他上游故障保持原语义。
4. 目标观察角色增加 metrics.k8s.io Node/Pod 的 get/list，不增加 create、update、
   patch 或 delete。
5. 前端 quantity 纯函数覆盖 CPU `n/u/m/core`、内存二进制与十进制单位，并分别
   聚合 Node usage 和全部 Pod container usage。Dashboard CPU/内存卡片独立加载；
   API 缺失时显示 `--` 和明确不可用状态，核心健康卡片继续显示真实数据。

## 自动化验证

- 全量门禁：`.artifacts/verification/verify-20260727-135642.json`
- 结果：165.03 秒，119 个 Go `Test*` 入口、12 个 Vitest 文件/49 个用例、前端
  typecheck/build、Go build、Compose 三服务 healthy、Kustomize `16/5/10/3`、
  readiness 与 frontend proxy 全部通过。
- 后端测试覆盖固定响应裁剪、Namespace 路径、空 containers 规范化、404 capability
  错误、HTTP 424 与 Gin/OpenAPI 双向路由一致性。
- 前端测试覆盖固定请求序列化、CPU/内存 quantity、非法单位、多容器聚合和绝对单位格式。

## 真实 kind 与 API

- retained demo：`.artifacts/e2e-kind/e2e-kind-20260727-134216.json`
- demo ready：`.artifacts/demo/demo-ready-20260727-134216.json`
- 环境：Kubernetes v1.34.0，`demo-kind-20260727-134215`，平台 cluster ID 33，
  一小时观察凭据创建于约 13:42 +08:00。
- 当前 kind 没有 `v1beta1.metrics.k8s.io` APIService；核心 Node 接口返回 200、total=1，
  Node/Pod Metrics 均返回 `424 METRICS_API_UNAVAILABLE`。
- SubjectAccessReview：get Node metrics=true、list Pod metrics=true、create Pod
  metrics=false。三条演示诊断、受控修复和相同幂等键重放仍通过。

## 浏览器验收

- Desktop 1280x720：文档 `scrollWidth/clientWidth=1265/1265`，Dashboard 高度 976；
  Node 1/1、Pod 11/14、Warning 12、待处置 3，CPU/Memory 显示 `--` 和不可用原因。
- Mobile 390x844：文档 `375/375`，指标区宽 279px，两列卡片各 134.5px，所有卡片
  均在文档边界内。不可用提示、集群选择、操作按钮和健康面板完整。
- 两个视口均无浏览器 warning/error 日志。临时 viewport 已重置为 1280x720，应用
  标签页保留为交付页面。

## 后续范围

- 可增加带 Metrics Server 的一次性 kind fixture，验证真实 available 分支与 quantity
  样本，但不应把 Metrics Server 变成平台强依赖。
- 利用率百分比只有在明确选择并展示 Node allocatable、Pod requests 或 limits 分母后
  才能加入。
- 历史趋势需要先设计有界采样、保留期、缺口语义和多集群存储成本，本阶段不持久化指标。
