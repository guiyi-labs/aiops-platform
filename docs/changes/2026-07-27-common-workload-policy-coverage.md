# M17 常用工作负载与策略资源覆盖

- 日期：2026-07-27
- 里程碑：M17
- 状态：Accepted
- 范围：九类固定只读资源合同、资源工作台、Secret 安全边界、真实 kind 与响应式验收

## 目标与边界

M17 扩展既有固定 Kubernetes gateway，不开放任意 GVK、API path、YAML、manifest
或写操作。所有资源继续绑定明确 `cluster_id`，namespaced 资源要求精确 Namespace，
列表沿用最大 100 条的有界分页，详情和关联 Event 独立加载。

| 公开资源 | 上游 API | 固定路径族 |
|---|---|---|
| StatefulSet / DaemonSet / ReplicaSet | `apps/v1` | `statefulsets` / `daemonsets` / `replicasets` |
| Job / CronJob | `batch/v1` | `jobs` / `cronjobs` |
| HPA | `autoscaling/v2` | `horizontalpodautoscalers` |
| ResourceQuota / LimitRange / Secret | `core/v1` | `resourcequotas` / `limitranges` / `secrets` |

## 后端与安全合同

1. 九类资源均提供固定 list/detail 服务、HTTP 路由、OpenAPI 3.0.3 schema 和双向
   route drift 覆盖；上游错误继续经过统一 Kubernetes 错误映射。
2. StatefulSet、DaemonSet、ReplicaSet、Job 和 CronJob 的 Pod template 只公开
   container name/image，不公开 template labels、annotations 或未声明字段。
3. HPA 保留目标、replica 范围、声明指标、当前/期望副本和 Conditions，不公开
   metric selector、behavior 或当前 metric 内部结构；空集合统一为数组。
4. Secret 经 `rawSecret -> Secret` 独立转换，只公开 name、Namespace、UID、
   时间、resourceVersion、type、immutable 和排序后的 `dataKeys`。公开 metadata
   主动移除 labels/annotations，值和诱饵内容不可能经 JSON 模型序列化。
5. managed-cluster observer 对新增资源保持 `get/list`；没有增加 create/update/
   patch/delete。真实 SubjectAccessReview 为 Secret list=yes、create=no、HPA list=yes。

Secret 裁剪发生在平台公开模型边界。Kubernetes RBAC 不能授予“只读键名、不读值”的
字段级权限，因此 observer ServiceAccount 仍能从 API Server 读取原始 Secret 对象。
生产启用此能力必须明确接受该风险，限制并保护平台身份和运行时；不得把公开响应脱敏
表述为目标集群凭据没有读取 Secret 值的能力。

## 资源工作台

- 工作负载分类覆盖 Pod、Deployment、StatefulSet、DaemonSet、ReplicaSet、Job、
  CronJob 和 Node；弹性与配置覆盖 HPA、ResourceQuota、LimitRange、ConfigMap 和 Secret。
- 九类资源提供统一库存行、类型化状态/核心配置/观测摘要、精确 URL 深链和详情抽屉。
- Job/HPA 显示 Conditions；Secret 详情只显示 key 名且不渲染 Labels 区域。
- Event 使用精确 involvedObject kind/Namespace/name 查询，详情失败与 Event 失败互不遮蔽。
- 页面一次刷新并发加载 Namespace 和 17 类资源，共 18 个有界请求。当前单集群工作台
  可接受；M20 多集群效率阶段需评估按分类延迟加载、缓存和取消未使用请求。

## 演示 Fixture 与真实 kind

`deploy/demo-scenarios/m17-resources.yaml` 新增九个代表对象：scale-to-zero StatefulSet、
DaemonSet、scale-to-zero ReplicaSet、暂停 Job、暂停 CronJob、HPA、ResourceQuota、
PVC-only LimitRange 和 immutable Secret。它们不额外创建运行中的 Pod。

- E2E：`.artifacts/e2e-kind/e2e-kind-20260727-152830.json`
- Metrics：`.artifacts/metrics-e2e/metrics-e2e-20260727-152830.json`
- 独立公开 API 复核：`.artifacts/api-m17/api-m17-20260727-154748.json`
- 环境：kind Kubernetes v1.34.0、Metrics Server v0.8.0、
  `demo-kind-20260727-152828`、平台 cluster ID 35
- M17 E2E 计数：StatefulSet 1、DaemonSet 1、ReplicaSet 11、Job 1、CronJob 1、
  HPA 1、ResourceQuota 1、LimitRange 1、Secret 1
- Secret 验证：`dataKeys=["example-key"]`；公开原始 JSON 不包含 `data`、labels、
  annotations、`must-not-enter-public-secret-model` 或 `public-demo`
- 回归：1 Node/12 Pod metrics、三条演示诊断、处置执行和同键幂等重放均通过

## 自动化验证

- 最终全量门禁：`.artifacts/verification/verify-20260727-155239.json`
- 结果：148.86 秒；121 个 Go `Test*` 入口；12 个 Vitest 文件/54 个用例；
  frontend typecheck/build、Go vet/test/build、Compose build/health、运行态 HTTP 均通过。
- Kustomize：platform/managed-cluster/demo/diagnosis 分别为 `16/5/19/3`。
- 后端重点覆盖 18 个 M17 list/detail 路径用例、Secret 非序列化、HPA selector
  非序列化/空集合以及 observer RBAC；前端覆盖九类 API URL 与 Event kind 映射。

## 浏览器验收

证据目录：`.artifacts/browser-m17/`。

- Desktop 1280x720：四分类和八个工作负载标签完整；StatefulSet、Job、CronJob、
  HPA、ResourceQuota、LimitRange、Secret 的列表/详情/深链通过；文档无横向溢出。
- Mobile 390x844：实际文档宽度 375/375；分类和资源类型按两列换行；930px 宽表只在
  277px 资源面板内部滚动；详情抽屉为 375px；Secret key 名无泄漏或重叠。
- 浏览器验收发现 M17 Condition 模板没有复用既有状态点和内容容器，HPA reason/message
  被挤入 9px 网格列。修复模板后，三个卡片宽 326px、正文宽 281px、正常自动换行，
  抽屉高度内滚动且页面无横向溢出。
- 最终浏览器 warning/error 日志为空；临时 viewport override 已重置，资源工作台标签保留。

## 后续路线

M18 进入 evidence-based diagnosis expansion：优先评估 Node pressure、PVC Pending、
HPA saturation、Ingress backend failure 和 sustained restart。每条规则必须具备可观察
证据、版本化正反例和可重放 fixture；确定性规则继续是权威结论，AI 只解释已持久化证据。

仓库仍没有初始 Git commit。正式发布前必须人工确认作者身份与提交范围，创建基线
revision 后重跑全量门禁、重采 revision-bound 截图并打 tag；现有 artifacts 不声明 Git revision。
