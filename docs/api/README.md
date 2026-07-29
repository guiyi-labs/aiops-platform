# API Documentation

所有接口使用 `/api/v1` 前缀。Kubernetes 资源接口必须显式包含 `cluster_id`。

当前基础接口：

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/health/live` | 进程存活检查 |
| GET | `/api/v1/health/ready` | 数据库等依赖就绪检查 |
| POST | `/api/v1/auth/login` | 用户名密码登录并创建刷新会话 |
| POST | `/api/v1/auth/refresh` | 轮换刷新令牌并签发新访问令牌 |
| POST | `/api/v1/auth/logout` | 撤销当前刷新会话 |
| GET | `/api/v1/auth/me` | 获取当前访问令牌中的用户信息 |
| POST | `/api/v1/auth/password-change` | 校验当前密码并主动修改密码；所有登录用户 |
| GET | `/api/v1/auth/sessions` | 当前用户的有效刷新会话元数据 |
| DELETE | `/api/v1/auth/sessions/{session_id}` | 撤销一个其他会话；当前用户 |
| POST | `/api/v1/auth/sessions/revoke-others` | 保留当前会话并撤销全部其他会话 |
| GET | `/api/v1/users` | 用户列表；系统管理员 |
| POST | `/api/v1/users` | 创建用户并分配角色；系统管理员 |
| PATCH | `/api/v1/users/{user_id}` | 更新显示名、状态或角色；系统管理员 |
| POST | `/api/v1/users/{user_id}/password-reset` | 重置其他用户密码并使全部旧会话失效；系统管理员 |
| GET | `/api/v1/users/assignable` | 可接收诊断的活跃系统/运维管理员 |
| GET | `/api/v1/clusters` | 集群列表；所有已登录角色 |
| POST | `/api/v1/clusters` | 导入 kubeconfig；系统管理员 |
| GET | `/api/v1/clusters/{cluster_id}` | 集群详情；所有已登录角色 |
| PATCH | `/api/v1/clusters/{cluster_id}` | 启用或停用；系统管理员 |
| PUT | `/api/v1/clusters/{cluster_id}/credentials` | 原子替换加密 kubeconfig；系统管理员 |
| POST | `/api/v1/clusters/{cluster_id}/probe` | 连接探测；系统管理员或运维管理员 |
| DELETE | `/api/v1/clusters/{cluster_id}` | 删除集群和凭据；系统管理员 |
| GET | `/api/v1/fleet/health` | 有界比较已启用集群的 Node/Pod/Deployment/Event 健康采样；所有已登录角色 |
| GET | `/api/v1/fleet/resources/search` | 按名称和可选 Namespace 有界搜索 Pod/Deployment/Service/Ingress；所有已登录角色 |
| GET | `/api/v1/fleet/resources/search/filters` | 列出当前用户的私有搜索筛选器及 20 条上限；所有已登录角色 |
| POST | `/api/v1/fleet/resources/search/filters` | 保存当前固定搜索条件；所有已登录角色，写入审计 |
| PATCH | `/api/v1/fleet/resources/search/filters/{filter_id}` | 重命名或用完整当前查询覆盖本人筛选器；所有已登录角色，写入审计 |
| DELETE | `/api/v1/fleet/resources/search/filters/{filter_id}` | 删除本人筛选器；所有已登录角色，写入审计 |
| GET | `/api/v1/clusters/{cluster_id}/namespaces` | Namespace 列表 |
| GET | `/api/v1/clusters/{cluster_id}/nodes` | Node 列表 |
| GET | `/api/v1/clusters/{cluster_id}/metrics/nodes` | Node CPU/内存绝对用量；Metrics API 可选 |
| GET | `/api/v1/clusters/{cluster_id}/metrics/pods` | Pod 容器 CPU/内存绝对用量；可用 `namespace` 筛选 |
| GET | `/api/v1/clusters/{cluster_id}/metrics/history` | 鉴权的精确 Node/Pod CPU/内存稀疏历史序列 |
| GET | `/api/v1/clusters/{cluster_id}/pods` | Pod 列表；可用 `namespace` 筛选 |
| GET | `/api/v1/clusters/{cluster_id}/pods/{namespace}/{name}` | Pod 详情 |
| GET | `/api/v1/clusters/{cluster_id}/pods/{namespace}/{name}/logs` | 当前或 previous 容器日志 |
| GET | `/api/v1/clusters/{cluster_id}/events` | Event 列表；可用 `namespace` 筛选 |
| GET | `/api/v1/clusters/{cluster_id}/deployments` | Deployment 列表；可用 `namespace` 筛选 |
| GET | `/api/v1/clusters/{cluster_id}/services` | Service 列表；可用 `namespace` 筛选 |
| POST | `/api/v1/clusters/{cluster_id}/diagnoses` | 对指定 Pod、Service、Node、Deployment、Ingress、PVC 或 HPA 执行已注册规则诊断 |
| GET | `/api/v1/diagnoses` | 诊断历史；可用 `cluster_id`、`status`、`overdue` 筛选 |
| GET | `/api/v1/diagnoses/summary` | 诊断状态、逾期数与最近 5 条活动 |
| GET | `/api/v1/diagnoses/{diagnosis_id}` | 诊断详情，包含持久化证据 |
| PATCH | `/api/v1/diagnoses/{diagnosis_id}` | 状态流转并自动认领；系统或运维管理员 |
| PATCH | `/api/v1/diagnoses/{diagnosis_id}/assignment` | 转派负责人并追加历史；系统或运维管理员 |
| POST | `/api/v1/diagnoses/{diagnosis_id}/feedback` | 提交规则准确性反馈；系统或运维管理员 |
| GET | `/api/v1/diagnoses/{diagnosis_id}/explanations` | AI 解释历史；所有已登录角色 |
| POST | `/api/v1/diagnoses/{diagnosis_id}/explanations` | 基于持久化证据生成引用式解释；系统或运维管理员 |
| GET | `/api/v1/ai/status` | AI Provider、并发、当日用量与预算状态；所有已登录角色 |
| GET | `/api/v1/ai/quality` | AI 解释反馈总量、有帮助率、贡献者与按模型汇总；所有已登录角色 |
| POST | `/api/v1/ai/explanations/{explanation_id}/feedback` | 对单条解释提交一次质量评价；所有已登录角色 |
| GET | `/api/v1/audit-logs` | 审计日志列表；系统管理员或安全审计员 |
| GET | `/api/v1/audit-logs/export` | 按相同条件导出有界 UTF-8 CSV；系统管理员或安全审计员 |
| GET | `/api/v1/notification-deliveries` | Webhook 投递元数据；系统管理员或安全审计员 |
| POST | `/api/v1/notification-deliveries/{delivery_id}/retry` | 重新排队 dead 投递；系统管理员 |
| GET | `/api/v1/diagnoses/{diagnosis_id}/remediations` | 查看受控修复计划元数据；所有已登录角色 |
| POST | `/api/v1/diagnoses/{diagnosis_id}/remediations/preview` | 对匹配诊断 Pod 的 Deployment rollout restart 做 server-side dry-run；系统管理员或运维管理员 |
| GET | `/api/v1/clusters/{cluster_id}/operations` | 查看有界资源操作历史；所有已登录角色，可按 Namespace、目标类型和名称筛选 |
| POST | `/api/v1/clusters/{cluster_id}/operations/preview` | 对固定 Deployment/CronJob 资源操作做 server-side dry-run；系统管理员或运维管理员 |
| POST | `/api/v1/remediations/{remediation_id}/execute` | 使用一次性确认 token 和 `Idempotency-Key` 执行固定计划；系统管理员或运维管理员 |

登录成功响应：

```json
{
  "access_token": "<jwt>",
  "token_type": "Bearer",
  "expires_in": 900,
  "user": {
    "id": 1,
    "username": "admin",
    "display_name": "System Administrator",
    "roles": ["system_admin"]
  }
}
```

访问令牌通过 `Authorization: Bearer <jwt>` 传递。刷新令牌不进入 JSON，由 `/api/v1/auth` 路径下的 HttpOnly Cookie 传递并在每次刷新时轮换。平台角色为 `system_admin`、`operations_admin`、`security_auditor`、`viewer`。

Metrics 端点只访问服务端固定构造的 `metrics.k8s.io/v1beta1` Node/Pod 列表路径，分页上限 100、上游响应上限 10 MiB。公共响应只保留 metadata、timestamp、window、usage 和 Pod container name，不开放任意 group/version/path。目标集群未安装或未提供 Metrics API 时返回 HTTP 424 和 `METRICS_API_UNAVAILABLE`；集群不存在、停用、权限拒绝与其他上游错误继续使用各自原有语义。CPU/内存值保留 Kubernetes quantity，Dashboard 只展示解析后的绝对用量，不在缺少真实分母时伪造利用率百分比。

历史端点 `GET /api/v1/clusters/{cluster_id}/metrics/history` 只接受一个完整
精确序列：`resource_kind=Node|Pod`、资源名、`cpu|memory`，Pod 还必须提供
Namespace 和 container。`from`/`to` 为必填 RFC3339 时间戳，窗口最多 24 小时，
`limit` 为 1..1440。响应按采集时间稳定返回真实稀疏点，并显式提供成功、部分、
不可用、超时、失败、缺样本和截断信息；缺样本不会补零。所有平台身份均须先鉴权，
接口不接受任意标签、聚合、selector、GVK 或 PromQL。详见 ADR 0036。

主动改密请求为 `{"current_password":"...","new_password":"..."}`。新密码要求 12–128 字符且不能与当前密码相同。服务端先校验当前 bcrypt 摘要，再以旧摘要作为 compare-and-swap 条件提交更新，避免并发管理员重置后被旧操作覆盖。成功响应为 204，同时递增 `auth_version`、撤销全部刷新会话并清除当前 Cookie；当前访问令牌在下一请求立即失效，前端返回登录页。错误码包括 `CURRENT_PASSWORD_INVALID`、`PASSWORD_UNCHANGED` 与 `INVALID_NEW_PASSWORD`。审计操作为 `auth.password.change`，不记录请求体。

会话列表只返回数据库 ID、User-Agent、来源地址、创建/到期时间和 `current` 标记，不返回刷新令牌或 SHA-256 摘要。当前会话由 HttpOnly Cookie 的摘要与本人有效会话匹配确定。按 ID 撤销只允许本人其他会话，当前会话返回 409 `CURRENT_SESSION_PROTECTED`；撤销全部其他会话要求当前 Cookie 对应的会话仍有效，否则返回 `CURRENT_SESSION_REQUIRED`。单个撤销和批量撤销分别审计为 `auth.session.revoke`、`auth.sessions.revoke_others`。

## User Management

系统管理员可以分页读取用户、创建用户，以及更新显示名、`active`/`disabled` 状态和角色集合。创建请求示例：

```json
{
  "username": "ops.user",
  "display_name": "Operations User",
  "password": "initial-password-2026",
  "roles": ["operations_admin", "viewer"]
}
```

用户名必须是 3–64 位小写字母、数字、点、下划线或连字符，且首字符为字母或数字；初始密码为 12–128 字符；显示名为 1–128 字符；角色至少一个且只能使用平台预置代码。用户名创建后不可修改，密码摘要使用 bcrypt 存储，列表和响应均不返回摘要或明文。

更新请求只需提交需要修改的字段，例如 `{"status":"disabled"}` 或 `{"roles":["security_auditor","viewer"]}`。角色变更或账号停用会撤销该用户所有未过期刷新会话；每个受保护请求还会按令牌中的用户 ID 读取当前数据库状态与角色，因此已签发访问令牌不会保留被撤销的权限。

当前用户不能停用自己或修改自己的角色。仓储使用事务级 advisory lock 串行化系统管理员资格变更，并拒绝停用或移除最后一个活跃系统管理员。相关错误码为 `INVALID_USER`、`USERNAME_EXISTS`、`USER_NOT_FOUND`、`SELF_PROTECTION` 和 `LAST_SYSTEM_ADMIN`。创建与更新分别记录 `user.create`、`user.update` 审计。

管理员密码重置请求为 `{"password":"replacement-password-2026"}`。新密码必须为 12–128 字符。服务端在同一事务中保存 bcrypt 摘要、递增用户 `auth_version` 并撤销全部刷新令牌；访问令牌携带签发时的版本，每次认证与数据库当前版本比较，因此旧访问令牌也立即失效。系统管理员不能通过该管理接口重置自己的密码，错误码为 `SELF_PASSWORD_RESET`。审计操作代码为 `user.password.reset`，不会记录密码或请求体。

## Cluster Contract

创建请求只在请求体中接收 kubeconfig：

```json
{
  "name": "kind-dev",
  "kubeconfig": "apiVersion: v1\nkind: Config\n..."
}
```

kubeconfig、token、证书和密钥永不进入响应。集群响应只包含名称、API Server、启用状态、版本、探测时间和 Conditions。`status` 为 `disabled`、`unknown`、`ready` 或 `unreachable`；Condition 类型为 `Ready`、`CredentialValid`、`Reachable`。

凭据轮换请求只包含新的 `kubeconfig`。服务端先完成同创建接口一致的安全解析和 AES-256-GCM 加密，再在单个事务中替换密文与 API Server 元数据，清除旧版本/探测时间，并把三个 Condition 设为 `Unknown / CredentialsUpdated`。事务成功后立即使该集群的缓存客户端失效；不会自动探测，调用方必须显式执行 `/probe`。非法 kubeconfig 不改变原凭据，成功响应也不回显任何凭据材料。操作审计代码为 `cluster.credentials.rotate`。

当前接入器仅接受 kubeconfig 内嵌 token 或内嵌客户端证书，并要求绝对 HTTPS API 地址。不读取本地证书路径，不执行 `exec` 插件，也不支持外部 auth-provider，避免服务端导入时产生文件读取或命令执行风险。

## Kubernetes Resource Queries

所有资源接口显式包含 `cluster_id`，并拒绝查询已停用集群。列表支持 `page`、`limit`、`sort_by=name`、`ascending`、`name`、`label_selector`、`field_selector`；Namespace 使用 `namespace` 查询参数。

Pod 日志参数：

| Parameter | Default | Constraint |
|---|---:|---|
| `container` | 空，由 API Server 决定 | 容器名 |
| `previous` | `false` | `true` 或 `false` |
| `tail_lines` | `200` | 1–2000 |

日志响应最大 1 MiB，资源 JSON 响应最大 10 MiB。平台只返回结构化资源摘要和日志文本，不返回目标集群凭据。目标 API 的 404 映射为 `RESOURCE_NOT_FOUND`，停用集群映射为 `CLUSTER_DISABLED`，其余上游错误映射为 `KUBERNETES_API_ERROR`。

`GET /api/v1/clusters/{cluster_id}/endpointslices` 是固定只读列表，支持 `namespace`、
`name` 和通用有界分页/名称排序参数。响应只包含 metadata、addressType、端口、
endpoint addresses/conditions/nodeName/targetRef，以及从
`kubernetes.io/service-name` 标签派生的 `serviceName`；不返回任意原始对象或开放
动态 discovery 代理。`ports` 和 `endpoints` 空值规范化为空数组。

## Rule Diagnosis

诊断请求：

```json
{
  "resource_kind": "Pod",
  "namespace": "demo",
  "name": "broken-api"
}
```

`resource_kind` 当前支持 `Pod`、`Service`、`Node`、`Deployment`、`Ingress`、`PersistentVolumeClaim` 与 `HorizontalPodAutoscaler`；HTTP 兼容输入别名 `PVC` 与 `HPA`，响应资源引用始终使用完整 Kind。Node 为集群级资源，`namespace` 可为空；其余资源必须提供 Namespace。Service 和 Ingress 后端证据优先查询 `discovery.k8s.io/v1` EndpointSlice，并使用 `kubernetes.io/service-name` 标签限定目标 Service；只有 discovery API 返回 404 时才回退同名 core/v1 Endpoints。403、超时或解码错误会原样失败，不以旧 API 掩盖权限和兼容问题。EndpointSlice 的 `ready` 为空时按 Kubernetes 兼容语义视为 Ready。Node 诊断先检查 `Ready`，只对 Ready Node 继续检查压力 Condition；Deployment 诊断比较期望副本与 Ready/Available 副本。已启用规则：

| Rule ID | Match condition | Evidence |
|---|---|---|
| `pod.image_pull_backoff.v1` | 容器 Waiting reason 为 `ImagePullBackOff` 或 `ErrImagePull` | 容器状态、镜像和相关 Warning Event |
| `pod.crash_loop_backoff.v1` | 容器 Waiting reason 为 `CrashLoopBackOff` | 当前等待状态、重启次数、上一次终止状态和 BackOff Event |
| `pod.pending.v1` | Pod phase 为 `Pending` | Pod 状态、PodScheduled 条件和 FailedScheduling Event |
| `pod.oom_killed.v1` | 当前或上一次容器终止 reason 为 `OOMKilled` | 容器终止状态、退出码、重启次数和内存相关 Event |
| `service.no_ready_endpoints.v1` | Service 有 selector、不是 ExternalName，且 EndpointSlice/Endpoints 没有 Ready address | Service selector/端口与后端地址计数、来源 API |
| `node.not_ready.v1` | Node 的 Ready Condition 缺失或状态不是 `True` | 全部 Node Conditions 的状态、Reason、Message 和 LastTransitionTime |
| `deployment.replicas_unavailable.v1` | Ready 或 Available 副本数低于期望副本数 | desired、replicas、ready、available、updated 和 unavailable 计数 |
| `node.pressure.v1` | Node Ready=True，且 MemoryPressure、DiskPressure 或 PIDPressure 至少一项为 True | 命中的压力 Condition、Reason、Message 和 LastTransitionTime |
| `persistentvolumeclaim.pending.v1` | PVC 为 Pending，且存在按 PVC UID 精确关联的 Warning Event | PVC phase、StorageClass、访问模式、申请容量和 Warning Events |
| `horizontalpodautoscaler.saturated.v1` | current/desired 达到 maxReplicas，且 ScalingLimited=True/TooManyReplicas | 扩缩目标、min/max/current/desired、指标摘要和精确 Condition |
| `ingress.backend_unavailable.v1` | 至少一个非 ExternalName Service 后端没有 Ready address | host/path/backend/port、Service 类型与 selector、地址计数和来源 API |

PVC 仅处于 Pending 并不足以判定故障，`WaitForFirstConsumer` 且没有 Warning Event 时不会命中。HPA 的 `TooFewReplicas` 表示下限约束，不属于扩容饱和。Ingress 会先去重后端 Service 再收集证据，任何 Service 或 Endpoint 读取失败都会使本次诊断失败，不输出不完整结论。

当前 Pod 模型只有单次快照和累计重启计数，没有时间窗口采样或前后快照比较，因此 M18 明确不提供“持续重启”规则。累计次数不能证明问题仍在持续；该能力需在后续引入有界时序采样和缺失样本语义后再评估。

响应包含严重级别、资源引用、规则结论、可能根因、处理建议、SLA 截止时间及证据。列表接口返回结论摘要、`sla_due_at`、`resolved_at` 与实时计算的 `overdue`，但不携带证据；详情接口按需读取 `diagnosis_evidence`，避免历史列表响应随证据体积线性膨胀。

未命中规则返回 HTTP 422 与 `NO_RULE_MATCH`；证据采集失败返回 `DIAGNOSIS_FAILED`。规则结果会先独立持久化，AI 不参与该创建接口；用户可在诊断详情中另行请求引用式解释。

## Diagnosis Workflow

状态流转请求：

```json
{
  "status": "confirmed",
  "comment": "已复现故障，开始处理"
}
```

允许的流转为：`open → confirmed/dismissed`、`confirmed → resolved/dismissed`、`resolved/dismissed → open`。首次状态变化会把负责人设置为当前操作者，并同步追加首次认领历史；每次流转都追加活动记录，不覆盖规则结论或证据。非法跳转返回 HTTP 409 与 `INVALID_STATUS_TRANSITION`。`comment` 可选，最多 2000 字符。

SLA 在诊断创建时按观察时间固定计算：`critical` 1 小时、`high` 4 小时、`warning` 24 小时、`info` 72 小时。只有 `open`、`confirmed` 且超过截止时间的记录计为逾期；解决或驳回后停止计入，重新打开后沿用原截止时间重新判断。该版本不因转派或状态确认重置时钟。

负责人转派请求：

```json
{
  "assignee_user_id": 2,
  "comment": "转交当班运维处理"
}
```

目标用户必须是活跃的 `system_admin` 或 `operations_admin`。每次转派追加操作者、前后负责人名称快照和备注；重复转给当前负责人返回 HTTP 409 与 `ALREADY_ASSIGNED`，非法目标返回 HTTP 400 与 `ASSIGNEE_NOT_ALLOWED`。

人工反馈请求：

```json
{
  "verdict": "accurate",
  "comment": "规则与实际故障一致"
}
```

`verdict` 仅接受 `accurate`、`inaccurate`、`uncertain`。反馈只追加保存，可用于后续规则质量评估和 AI 提示优化。查看诊断、汇总与活动对所有登录角色开放；状态修改和反馈要求 `system_admin` 或 `operations_admin`。

## Cited AI Explanations

AI 解释是显式按需操作，不进入规则诊断创建事务。服务端从已持久化的诊断详情构建上下文，将证据编号为 `E1`、`E2` 等；已知敏感键会脱敏，单项长文本和总上下文均有上限。Provider 使用 Responses API 风格的 `POST /responses`，发送 `store=false` 和严格 JSON Schema。

成功响应包含摘要、分析、带优先级的建议操作、证据引用、模型和 token 用量。每个 `evidence_id` 必须存在于本次输入，未知引用、非结构化响应或缺少引用都会被拒绝且不入库。历史只追加，重新生成不会覆盖旧解释。

AI 未启用返回 HTTP 503 `AI_DISABLED`；进程内并发槽位已满返回 HTTP 429 `AI_BUSY`；剩余每日 token 预算不足以覆盖本次预留返回 HTTP 429 `AI_BUDGET_EXCEEDED`；上游网络或限流失败返回 HTTP 502 `AI_PROVIDER_ERROR`；引用校验失败返回 HTTP 502 `AI_INVALID_OUTPUT`。这些错误不会改变规则结论、证据、负责人或状态，前端继续展示确定性结果。

`GET /api/v1/ai/status` 返回 `enabled`、`available`、`provider`、`model`、`max_output_tokens`、`max_concurrent_requests`、`active_requests`、`daily_token_budget`、`used_tokens_today`、`reserved_tokens`、`remaining_tokens`、`requests_today` 与 `last_success_at`。当预算配置为 `0` 时，`remaining_tokens` 为 `null`。状态是运行观测快照；在并发请求之间可能立即变化，生成接口仍会重新执行原子预算检查。

解释列表中的每一项同时返回 `feedback_summary` 和当前用户自己的 `my_feedback`；不会返回其他用户的评价备注。评价请求为：

```json
{
  "verdict": "helpful",
  "comment": "引用证据清晰，建议可直接执行"
}
```

`verdict` 只接受 `helpful`、`partially_helpful`、`not_helpful`，备注最多 1000 字符。每位用户对同一解释只能提交一次，重复提交返回 HTTP 409 `AI_FEEDBACK_EXISTS`；不存在的解释返回 HTTP 404 `AI_EXPLANATION_NOT_FOUND`，非法评价返回 HTTP 400 `INVALID_AI_FEEDBACK`。评价只追加，不改写模型输出、诊断结论或证据。

`GET /api/v1/ai/quality` 返回全量评价计数、有帮助率、已评价解释数、贡献者数和 `by_model` 汇总。有帮助率定义为 `helpful / total_feedback`；“部分有帮助”单独计数，不折算权重，避免在没有明确评估协议时制造虚假精度。

## Audit Logs

审计列表支持以下查询参数：

| Parameter | Meaning |
|---|---|
| `cluster_id` | 按仍存在的集群外键筛选 |
| `action` | 精确匹配操作代码 |
| `result` | `success`、`failure` 或 `denied` |
| `limit` | 1–100，默认 50 |

CSV 导出复用 `cluster_id`、`action`、`result`，并接受 1–5000 的 `limit`，默认 5000。响应使用 `text/csv; charset=utf-8`、UTC 时间戳文件名和 UTF-8 BOM；`X-Audit-Export-Rows`、`X-Audit-Export-Total`、`X-Audit-Export-Truncated` 分别表示本次行数、匹配总数和是否被上限截断。CSV 固定包含 15 列：ID、时间、操作者、集群、操作、资源、结果、请求 ID、HTTP 状态、来源、User-Agent 和 JSON 详情。

所有文本单元格在写出前检查首个非空白字符；以 `=`、`+`、`-`、`@` 开头时添加单引号，避免电子表格软件把不可信审计文本解释为公式。导出只包含已经过审计白名单处理的数据，不包含请求体、凭据或 Cookie。导出行为自身记录为 `audit.export`，但不会包含在正在生成的同一文件中。

当前操作代码包括认证会话与主动改密、用户创建/更新/密码重置、集群创建/启停/探测/删除/凭据轮换、规则诊断、诊断状态更新、负责人转派、规则反馈、AI 解释生成和 AI 解释质量反馈。响应包含操作者快照、资源引用、结果、请求 ID、HTTP 状态、来源地址、User-Agent、非敏感详情及时间。审计不保存访问令牌、刷新令牌、API Key、密码、kubeconfig、请求体、AI 上下文或诊断/评价备注正文。

权限拒绝记录为 `denied`，业务校验或上游失败记录为 `failure`，2xx 记录为 `success`。删除集群后 `cluster_id` 外键置空，但 `details.cluster_id` 和资源名称快照继续保留。

每个响应包含 `X-Request-ID`。客户端可以传入不超过 128 个可见 ASCII 字符的请求 ID；缺失或非法时由服务端生成。

统一错误响应：

```json
{
  "code": "ROUTE_NOT_FOUND",
  "message": "the requested route does not exist",
  "request_id": "client-request-1"
}
```

列表接口统一支持：

```text
page、limit、sort_by、ascending、name、label_selector、field_selector
```

`page` 从 1 开始，`limit` 默认为 20 且最大为 100。列表响应统一返回 `items`、`total` 和 `remaining`。

业务接口加入后，应记录请求、响应、权限和错误码，并逐步引入 OpenAPI 文档。

## OpenAPI and Metrics

The versioned contract baseline is [openapi.yaml](openapi.yaml). It documents the
currently registered public, authentication, user, cluster, Kubernetes,
diagnosis, AI and audit route families. It is intentionally a hand-reviewed
baseline rather than generated code; handlers remain the source of truth until
contract generation is introduced.

`GET /metrics` exposes Prometheus text for internal monitoring. Labels are
limited to HTTP method, Gin route template and status class (`2xx`, `4xx`,
etc.); raw URLs, IDs and users must never become labels. The endpoint has no
user authentication, so deployments must bind or firewall it to a trusted
scrape network and must not publish it through the public ingress.

## Diagnosis notification Webhook

The outbound receiver gets a JSON envelope with `id`, `event_type`,
`occurred_at` and allowlisted `data`. Supported event types are
`diagnosis.created`, `diagnosis.status_changed` and `diagnosis.assigned`.
Evidence, workflow comments, kubeconfig, tokens and webhook secrets are not
included. Headers are `X-AIOps-Event-ID`, `X-AIOps-Event-Type` and
`X-AIOps-Signature: sha256=<hex HMAC-SHA256 of the exact body>`.

Receivers should verify the signature before parsing and treat the event ID as
an idempotency key. The worker accepts only 2xx, never follows redirects, and
retries other outcomes with capped exponential delay. Delivery list filters
are `diagnosis_id`, exact `event_type`, `status` and `limit` (1–100). API items
contain status/attempt/timestamp/error metadata but deliberately omit the
stored payload. Only `dead` deliveries can be manually requeued; disabled
runtime configuration returns `NOTIFICATIONS_DISABLED`.

## Bounded fleet health

`GET /api/v1/fleet/health?limit=20` reads only enabled clusters visible through
the existing authenticated cluster directory. `limit` must be 1 through 20.
The server processes at most four clusters concurrently, gives each cluster a
four-second total context budget and samples at most 100 Nodes, Pods,
Deployments and Events per cluster.

Each item contains stable cluster identity/version, `healthy`/`degraded`/
`partial`/`unavailable`/`timed_out` status, sampled/total/complete resource
counts, Warning samples, duration and stable failure scope/code pairs.
Truncation or one resource failure is never labeled healthy. Per-cluster
failures remain HTTP 200 items so healthy clusters are still usable; only a
platform cluster-directory failure returns `FLEET_QUERY_FAILED`. The response
does not expose API servers, upstream error text, credentials or Kubernetes
objects, and the endpoint adds no write permission or arbitrary resource path.

## Bounded global resource search

`GET /api/v1/fleet/resources/search` accepts a required trimmed `q` substring
of 2 through 64 characters, an optional valid Namespace, and an optional
comma-separated subset of `pods`, `deployments`, `services` and `ingresses`.
`cluster_limit` is 1..20 and `limit` is 1..100. Arbitrary GVK, API path, label
selector, field selector, raw object and upstream error input/output are not
part of the contract.

Enabled clusters are sorted by platform ID. At most four cluster workers run,
each with one four-second budget and sequential fixed-kind reads. A kind
contributes at most 100 candidates. Results are normalized and sorted by
cluster ID, fixed kind order, Namespace and name before the global cap is
applied. Failures are localized as `TIMEOUT` or `QUERY_FAILED`.

The response distinguishes known result truncation (`total`, `remaining`) from
cluster coverage (`clusters_total`, `clusters_searched`,
`clusters_remaining`). `complete` is false when results are truncated, an
enabled cluster is omitted, or any fixed-kind read fails. All authenticated
roles may use the route because it adds no target-cluster verb. See ADR 0026.

## User-owned global-search filters

`/api/v1/fleet/resources/search/filters` is a private preference API over the
bounded search contract, not a new Kubernetes query language. Create accepts
only `name`, `query`, `namespace` and `kinds`; name is trimmed, 1..40 Unicode
characters and unique case-insensitively for the current user. Query is 2..64
characters, Namespace is empty or a valid exact name, and kinds are a non-empty
canonical subset of Pod, Deployment, Service and Ingress. Unknown JSON fields,
partial query overwrites and extra JSON values return `INVALID_SAVED_FILTER`.

List returns `items`, `total` and the fixed `limit=20`. Create returns 201,
rename/complete overwrite returns 200 and delete returns 204. Duplicate names
and the per-user cap return `SAVED_FILTER_NAME_EXISTS` and
`SAVED_FILTER_LIMIT_REACHED` with 409. A filter ID not owned by the caller is
indistinguishable from a missing record and returns `SAVED_FILTER_NOT_FOUND`
with 404. All authenticated roles may manage only their own records.

Every item carries `schema_version`, `compatible` and an optional
`incompatibility_code` of `SCHEMA_VERSION` or `QUERY_SHAPE`. Incompatible items
remain listable, renameable, replaceable with one complete current query and
deletable, but cannot be applied directly. Applying always runs a fresh bounded
search; results are not persisted. Mutation audit actions are
`global_search_filter.create`, `.update` and `.delete`, and audit details contain
only preference identity and route metadata, never the saved query body. See
ADR 0027.

## Controlled operations

The diagnosis-originated route supports only `deployment.rollout_restart`.
Preview accepts `{"action":"deployment.rollout_restart","target_name":"api"}`
for a `confirmed` Pod diagnosis. The server reads the current Pod and
Deployment, requires the Deployment selector to match the Pod labels, captures
UID/resourceVersion, generates the patch and sends it with `dryRun=All`.

The resource-originated preview route supports exactly these request shapes:

```json
{"action":"deployment.scale","namespace":"aiops-demo","target_name":"api","desired_replicas":3}
```

```json
{"action":"cronjob.suspend","namespace":"aiops-demo","target_name":"cleanup"}
```

`cronjob.resume` uses the same CronJob shape. Strict decoding rejects unknown
or irrelevant fields. Scale accepts only integer replicas from 0 through 1000;
all three actions reject a no-change target. The service captures the current
UID/resourceVersion and typed `spec.replicas` or `spec.suspend` before/after
value, builds the complete patch and requires `dryRun=All` before creating a
ten-minute plan. `GET /clusters/{cluster_id}/operations` returns at most 50
resource-originated plans and may filter by `namespace`, `target_kind` and
`target_name`; it never returns confirmation-token hashes or raw patches.

Execution for either origin requires `Idempotency-Key` (8–128 characters) and
the one-time confirmation token in the request body. The server dispatches
only by the persisted action and reuses the previewed UID/resourceVersion and
typed value. Same-key replay returns the stored result without another patch;
another key, an expired plan, invalid token or changed target is rejected. No
API accepts a Kubernetes path, verb, raw patch or arbitrary manifest. Preview
and execution outcomes, including authorization failures, enter the audit
trail. The target-cluster remediator Role grants namespaced Deployment and
CronJob `get`/`patch`; observer access remains read-only. Deployment rollback
is deferred until an exact immutable ReplicaSet revision/template snapshot can
be bound to preview and execution. See ADR 0023 and ADR 0024.
