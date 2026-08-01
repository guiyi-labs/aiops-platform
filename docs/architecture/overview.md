# Architecture Overview

状态：Accepted（M38 baseline, 2026-07-31）

```text
Vue Web Console
      |
      | HTTPS / JSON
      v
Go API Service
  |-- Ordered Request Pipeline
  |-- Auth and RBAC
  |-- Cluster Registry
  |-- Kubernetes Gateway ----> Kubernetes API Servers
  |-- Diagnosis Engine
  |-- AI Provider -----------> OpenAI-compatible API
  |-- Audit Service
  |-- Notification Worker ----> Signed Webhook Receiver
      |
      v
PostgreSQL
```

平台数据库只保存用户、角色、集群配置、加密凭据、诊断、AI 调用和审计记录。Node、Pod、Deployment 和 Service 等实时对象默认从目标 Kubernetes API Server 查询。

认证令牌只携带用户 ID 等会话声明；每个受保护请求都会按 ID 读取当前用户状态、显示名和角色，再进入授权中间件。这样账号停用或角色撤销可以立即作用于已经签发的访问令牌，代价是每个认证请求增加一次用户查询。角色变更和停用同时撤销全部刷新会话，阻止旧会话继续轮换。

用户创建和更新由认证领域服务统一校验用户名、密码、显示名、状态与预置角色。角色替换与会话撤销在单个数据库事务内完成。系统管理员资格变更还取得固定 PostgreSQL advisory lock，再锁定目标用户并统计活跃系统管理员，保证并发操作不能移除最后一个管理员；接口层另外阻止当前用户停用自己或修改自己的角色。

用户凭据带单调递增的 `auth_version`。访问令牌保存签发时版本，认证阶段在加载当前用户时执行精确比较。管理员重置密码的事务同时更新 bcrypt 摘要、递增版本并撤销全部刷新令牌，因此数据库提交后，旧访问令牌、旧刷新会话和旧密码均不可继续使用。版本而非时间戳避免同一秒签发与重置时的精度竞态。
用户主动改密在领域层验证当前 bcrypt 摘要和新旧差异，仓储更新再以读取到的旧摘要作为 compare-and-swap 条件。这样管理员重置或另一次改密一旦先提交，较旧请求就不能覆盖新凭据。成功路径与管理员重置共享版本递增及全刷新会话撤销语义，前端明确结束本地会话并要求重新登录。
会话设备管理继续以只存摘要的 `refresh_tokens` 为数据源。接口按认证用户 ID 限定查询，只暴露客户端元数据；当前会话由 Cookie 原文在服务端求摘要后匹配，不向前端发送摘要。撤销操作在事务内先确认当前会话仍有效，再保护当前会话并撤销指定或全部其他会话，避免过期 Cookie 意外清空所有有效设备。

集群凭据使用应用层 AES-256-GCM 加密。集群注册表按 `cluster_id` 缓存带固定超时和 User-Agent 的 HTTP 客户端；集群启停、配置变更或删除时必须使缓存失效。连接探测调用目标 API Server 的 `/version`，并持久化 `Ready`、`CredentialValid`、`Reachable` Conditions。
凭据轮换复用严格 kubeconfig 解析器和当前加密密钥版本；新密文、API Server、探测状态与 Conditions 在同一数据库事务中替换。提交前的解析/加密失败保留原凭据，提交成功后才清理客户端缓存。轮换不隐式发起网络访问，三个 Condition 先转为 Unknown，随后由显式探测写入真实结果。
Condition 只有在布尔状态发生变化时更新 `last_transition_time`；重复探测可以更新原因和消息，但不会伪造新的状态迁移时间。

当前核心资源只读链路复用同一集群客户端注册表，请求路径由服务端固定构造，只允许对 core/v1 的 Node、Namespace、Pod、Service、Endpoints、Event、Pod log、PersistentVolumeClaim、ConfigMap、ResourceQuota、LimitRange 和 Secret，apps/v1 的 Deployment、StatefulSet、DaemonSet、ReplicaSet，batch/v1 的 Job、CronJob，autoscaling/v2 的 HPA，networking.k8s.io/v1 的 Ingress，storage.k8s.io/v1 的 StorageClass，discovery.k8s.io/v1 EndpointSlice，以及可选 metrics.k8s.io/v1beta1 Node/Pod metrics 发起 GET。客户端统一设置 TLS、凭据、超时、User-Agent 和响应大小上限。Metrics 公共模型只返回 metadata、timestamp、window、usage 与容器名称，404 映射为显式 `METRICS_API_UNAVAILABLE`，不会影响核心资源读取。公共 EndpointSlice 列表只返回 metadata、addressType、端口、endpoint readiness/node/targetRef 和从标准标签派生的 Service 身份，分页上限为 100；空集合统一规范化为数组。Service/Ingress 诊断仍只在指定 Namespace 内按 `kubernetes.io/service-name` 精确标签查询，且仅 API 不存在的 404 回退同名 Endpoints，不开放任意代理能力。筛选 selector 转发给 API Server，名称筛选和页码在 MVP 内存中处理。

Secret 使用独立的原始上游模型和公开响应模型。公开模型只保留最小 metadata、type、immutable 与排序后的 data key 名，不序列化 data、stringData、labels 或 annotations。该裁剪阻止值进入 HTTP 响应和前端状态，但 Kubernetes RBAC 不能按对象字段授权：目标集群 ServiceAccount 的 `get/list secrets` 权限仍可读取原始 Secret。因此启用 Secret 键名观察必须经过明确的威胁接受、限制 ServiceAccount 使用范围并保护平台运行时；它不能被描述为对 Secret 值没有读取能力。

资源拓扑按固定关系构造 `Ingress -> Service -> EndpointSlice -> Pod <- Deployment`：Ingress 后端、Service 标签、EndpointSlice targetRef 和 Deployment selector 均要求同 Namespace 精确匹配。Service 只有在没有匹配 EndpointSlice 时才使用完整 selector 回退；空 selector、跨 Namespace、未知 targetRef 和名称近似均不产生关系。该拓扑只读展示，不接受任意 GVK、API path、YAML 或写操作。

Pod 诊断链路为：读取 Pod → 按 UID 获取 Event → 按优先级匹配 Pending、OOMKilled、ImagePullBackOff 和 CrashLoopBackOff 规则 → 构造结构化证据 → 事务保存结论与证据 → 返回前端。Service 诊断链路为：读取 Service → 优先聚合关联 EndpointSlice、必要时回退同名 Endpoints → 排除 ExternalName 和无 selector Service → 判断 Ready address → 保存规则结论、来源 API 与证据。规则结论与“可能根因”明确分开：资源状态、调度条件、容器终止状态、上一次终止状态、Event 和地址计数是已观察证据，配置错误、依赖不可用、标签不匹配等是待验证假设。
Node 诊断按名称读取集群级资源并检查 Ready Condition；缺失或非 True 时保存完整 Conditions 后命中 `node.not_ready.v1`。Deployment 诊断按名称读取 namespaced apps/v1 对象，按 Kubernetes 默认副本数语义比较 desired、Ready 与 Available，并保存全部 rollout 计数后命中 `deployment.replicas_unavailable.v1`。两条链路继续使用同一诊断持久化、SLA、审计和 AI 解释边界，不增加写权限。

M18 将同一边界扩展到四类当前状态证据。Ready Node 仅在 MemoryPressure、DiskPressure 或 PIDPressure 为 True 时命中压力规则，NotReady 仍由更高优先级规则解释；Pending PVC 必须同时存在按对象 UID 精确查询的 Warning Event，避免误报正常的 WaitForFirstConsumer；HPA 必须已达到 maxReplicas 且 ScalingLimited=True/TooManyReplicas；Ingress 先提取并去重同 Namespace Service 后端，再完整读取 Service 与 EndpointSlice/Endpoints，任何采集失败都终止诊断而不输出部分结论。平台没有 Pod 时序采样窗口，单次累计 restartCount 不能证明持续重启，因此当前不实现 sustained-restart 规则。

诊断历史列表只读取结论摘要；用户打开详情时再按诊断 ID 读取证据行。该边界降低列表负载，同时保留规则版本、资源 UID、观察时间和证据快照的完整追溯能力。

人工处置与规则输出分离：规则结论和证据是不可变诊断快照，当前状态、负责人、状态活动和准确性反馈是后续人工数据。状态流转在数据库事务内锁定诊断记录，校验状态机后更新当前状态并追加活动，避免并发操作产生不可解释的跳转。人工反馈采用追加写入，不反向修改规则命中结果。

诊断 SLA 以规则观察时间为起点，在创建时按严重级别写入固定截止时间。逾期是查询时根据当前状态、截止时间和数据库时钟计算的派生状态，解决/驳回不会改写截止时间，重新打开也不会获得新的时间预算。负责人转派在同一事务内锁定诊断行、更新当前负责人并追加名称快照历史；可转派目录只暴露活跃的系统管理员和运维管理员。

Dashboard 只读取当前选择的单个集群，不跨集群扫描。核心 Node、Pod、Deployment、Service、Event 态势与诊断汇总独立于 Metrics 请求加载；Metrics API 缺失或失败只影响 CPU/内存卡片和消费排行。前端 quantity 解析覆盖 CPU `n/u/m/core` 与内存二进制、十进制单位，聚合 Node 和所有 Pod container 的绝对用量。Node 利用率只使用名称完全匹配且有效的 `status.allocatable` 分母；缺失、非法或为零时不计算百分比。Pod 消费排行按 CPU/内存分别排序，只展示有界前五名并陈述已加载样本数和集群返回总数；排行深链继续显式携带 cluster、kind、Namespace 和 name。

平台审计由统一 HTTP 中间件在受控写路由完成后同步追加。中间件读取认证阶段写入的操作者上下文、集群/资源定位和请求 ID，将 2xx、401/403、其他错误分别映射为 success、denied、failure。处理器只补充非敏感资源定位，不把请求体传给审计层。审计查询只对系统管理员和安全审计员开放。

审计记录与业务对象使用外键弱关联和字符串快照并存：对象存在时可按外键筛选，对象删除后仍能从 details 与资源字段解释原操作。当前同步追加不与集群或诊断业务事务形成跨服务原子提交；审计写失败会进入结构化服务日志。需要强原子保证时采用事务 outbox，而不是让审计层读取敏感业务请求体。

审计 CSV 导出复用仓储筛选并固定最大 5000 行，在内存中完成有界编码后一次性响应，避免部分文件与无界查询。列顺序稳定，时间统一为 UTC RFC3339，UTF-8 BOM 兼容常用电子表格软件；所有可能触发电子表格公式解析的文本单元格均被中和。导出沿用系统管理员/安全审计员权限，并作为 `audit.export` 追加审计，因此读取和外带行为本身也可追溯。

AI 解释位于确定性诊断之后，采用可注入的 Responses-compatible Provider。输入只来自持久化规则结果和证据：服务端分配证据 ID、裁剪总上下文并递归脱敏敏感键；请求使用 `store=false` 与严格 JSON Schema。返回结果再次校验所有引用 ID，只有通过校验的解释才追加保存。Provider 禁用、超时、限流、非结构化输出或虚构引用都只影响本次解释请求，不修改诊断主记录。

AI 运行护栏分两层。进程内非阻塞 semaphore 限制同时进入 Provider 的请求数，满载时立即返回可重试的 `AI_BUSY`。每日 token 预算使用 PostgreSQL 预留表：事务先取得固定 advisory lock、清理过期预留、汇总 UTC 当日实际用量与所有有效预留，再决定是否插入本次预留。请求完成或失败都会释放预留，成功解释按 Provider usage 追加记账；异常退出由过期时间兜底。该设计防止多实例同时越过预算检查，同时保留确定性诊断的可用性。`max_output_tokens` 同时进入上游请求和预算估算，限制单次最坏输出规模。

AI 质量反馈与解释正文分离保存。所有登录角色都可以评价，以覆盖只读排障参与者；数据库唯一约束保证每位用户对每条解释至多一票。解释列表只暴露聚合计数和当前用户自己的评价，不泄露其他评价者的备注。全局质量接口按模型汇总原始三档计数与严格“有帮助”比例，作为离线评估输入，不自动修改提示词、规则或模型路由。反馈提交经过统一审计，但备注正文不进入审计详情。

诊断链路遵循：异常发现、规则诊断、证据聚合、按需 AI 解释、人工确认、审计归档。

请求链路遵循：Recovery、Request ID、Request Metadata、Authentication、Authorization、Audit、Metrics/Logging、Handler。集群、资源和操作者信息通过请求上下文向后传递。

HTTP metrics are collected after the handler completes. The middleware records
only method, registered route template and status class, with request count and
duration aggregates. It never uses the raw URL path, query string, user ID or
request body as a label. `/metrics` is an internal scrape surface and must be
network-restricted at deployment boundaries.

The Kubernetes deployment keeps PostgreSQL and the Go API behind `ClusterIP`
Services. The Ingress targets only the frontend Nginx service; Nginx proxies
`/api/` but deliberately does not proxy `/metrics`. NetworkPolicies allow the
backend from frontend pods and a labeled monitoring namespace, and deny all
other namespace traffic by default. See ADR 0021.

Diagnosis notifications use a transactional outbox rather than remote HTTP in
the diagnosis transaction. A PostgreSQL trigger appends an allowlisted event
when a diagnosis is created, changes status or changes assignee. Workers claim
due rows with `FOR UPDATE SKIP LOCKED`, sign the exact JSON body with
HMAC-SHA256, and persist delivered/retry/dead state. Stale claims are
recoverable after process failure. The outbound payload excludes evidence,
comments, credentials and user contact data, and the administrative API exposes
delivery metadata only. This boundary publishes facts for external automation
but grants no permission to mutate a Kubernetes cluster; remediation remains a
separate confirmed and audited workflow. See ADR 0022.

The controlled-operations boundary is a fixed catalog with two origins.
`deployment.rollout_restart` remains diagnosis-bound: a confirmed Pod diagnosis
must map by namespace and selector to its current Deployment. Resource detail
views may originate exactly `deployment.scale`, `cronjob.suspend` and
`cronjob.resume`; they do not fabricate diagnosis records. M23 extended the
catalog with `deployment.image_update` and `deployment.rollback`, both bound to
an exact ReplicaSet revision and Pod-template snapshot captured at preview time
so confirmation cannot drift to a different revision. M28-M31 added
Velero Backup creation, Node cordon/uncordon/eviction and isolated restore
rehearsal under the same preview/confirm/idempotency/audit discipline. Every
preview reads the target, captures UID/resourceVersion and a typed before/after
value, builds the complete patch on the server and submits it with Kubernetes
`dryRun=All` before creating an expiring plan. Execution requires a one-time
token and an idempotency key, dispatches only by the persisted action and
reuses the captured preconditions. History is bounded, safe metadata is
readable by authenticated roles, and write access remains system/operations
administrator only.

The API accepts no Kubernetes path, verb, YAML, raw patch or arbitrary GVK. The
target-cluster remediator Role grants namespaced Deployment and CronJob
`get`/`patch`, while observer permissions remain read-only. See ADR 0023,
ADR 0024, ADR 0040 and `deploy/managed-cluster/`.

Fleet health is a separate read-only aggregation boundary. One authenticated
request lists the currently visible enabled clusters, selects at most 20 in
stable platform-ID order and runs at most four cluster workers. Each worker has
one four-second context and performs fixed Node, Pod, Deployment and Event
reads sequentially, sampling at most 100 objects per kind. Results retain only
counts, coverage, stable failure codes and duration; raw objects and upstream
errors never cross the fleet response model.

Per-cluster timeout, query failure and truncated coverage remain local to that
cluster and do not suppress successful peers. A cluster cannot be labeled
healthy when any resource sample is incomplete. The endpoint reuses the
existing authenticated cluster visibility and target observer credentials; it
adds neither a write path nor an arbitrary Kubernetes query surface. See ADR
0025.

Global resource search is a second read-only fleet boundary. It accepts only a
bounded name substring, one optional exact Namespace and a fixed subset of Pod,
Deployment, Service and Ingress. Enabled clusters use the same stable-ID,
20-cluster, four-worker and four-second admission model; kind reads remain
sequential and each kind contributes at most 100 normalized candidates. The
response returns navigation metadata and bounded health summaries, never raw
objects, API Server addresses or upstream errors.

Known matches, returned matches and enabled-cluster coverage are separate
fields. Result truncation, omitted clusters and localized `TIMEOUT` or
`QUERY_FAILED` entries all make `complete=false`. The browser persists only the
fixed query shape in its URL and deep-links a selected match to the existing
resource drawer. A separate user-preference boundary persists that same
reviewed shape as `saved_global_search_filters`. Records are owner-scoped,
capped at 20 per user under a per-user advisory lock, case-insensitively named
and versioned for stale-shape detection. Only the current user may list or
mutate them; another user's ID is returned as missing. Create/update/delete
enter unified audit without request-body capture.

An incompatible record remains visible and may be renamed, overwritten with a
complete current query or deleted, but cannot be applied. Applying a compatible
record updates the fixed URL state and runs a fresh bounded search; no result or
raw Kubernetes object is stored. Sharing, schedules, arbitrary selectors,
GVKs, paths, YAML and bulk operations remain excluded. See ADR 0026 and ADR
0027.

## M33-M38 Capability Plane and Delivery Hardening

### M33 Restricted client-go migration

The raw HTTP Kubernetes gateway was replaced by an official, bounded
`client-go` v0.34.x transport layer. A `ClusterClientProvider` caches
sanitized `rest.Config`, typed clientset, dynamic client (only for code-owned
CRD GVRs), discovery client and server version keyed by `cluster_id` and
credential generation. Concurrent first use builds one client only; credential
rotation, cluster disable and deletion invalidate after the database commit
and close idle connections. The strict kubeconfig parser, fixed QPS/Burst,
timeout, User-Agent, response cap and per-cluster concurrency are preserved.
The public raw `Registry.Patch/Create` surface is no longer reachable. See
ADR 0048.

### M34 RouteDescriptor contract and RBAC inventory

An immutable `RouteDescriptor` is now the single source of truth for routing,
authentication, role requirements, audit classification and low-cardinality
metrics. The same descriptor drives Gin route registration, the OpenAPI
document, the frontend client/types and the audit middleware. Duplicates,
missing audit actions and unclassified routes fail closed at startup or in
contract tests. ADR 0039's promised bounded RBAC read-only inventory exposes
fixed projections of Role, ClusterRole, RoleBinding and ClusterRoleBinding.
Public projections include safe metadata, Role rules and Binding
subjects/roleRef, and never resolve ServiceAccount tokens, Secret values or
impersonate a subject. See ADR 0049.

### M35 Lightweight cluster and namespace access grants

Two grant tables (`user_cluster_grants`, `user_namespace_grants`, migration
`000025_access_grants`) introduce the platform's first *resource-scope*
authorization dimension on top of the four global platform roles. A single
policy evaluator (`authz.Service`) answers cluster access, namespace access
and visible-cluster filtering questions; `requireClusterAccess` and
`requireNamespaceAccess` middleware wire the evaluator into fleet, search and
resource routes carrying `:cluster_id` or `:namespace` path parameters.
Authorization failures return 404 (not 403) to avoid leaking the existence
of hidden clusters or namespaces. SystemAdmin bypasses all grants; other
roles see only granted scope. Fleet and global search silently omit
unauthorized clusters from results, counts and errors. See ADR 0050.

### M38 Engineering, delivery and supply-chain hardening

**CI completeness (M38A).** The pull-request gate now runs the race detector
(`go test -race`), `golangci-lint@v2.12.2` with `.golangci.yml`, `pnpm lint`
with the ESLint flat config, a 50.0% backend coverage baseline and
`oasdiff breaking --fail-on ERR` against the base branch OpenAPI. The
real-kind E2E workflow covers the M23-M31 disposable acceptance suites in
addition to diagnosis, fleet, search and M21-history. `pull_request_target`
and `secrets.*` are forbidden in CI workflows.

**Helm chart (M38B).** An official Helm 3 chart under
`deploy/helm/aiops-platform/` provides a parameterized, schema-validated
deployment path alongside the existing kustomize baseline. The chart never
renders a Secret; operators provide an existing Secret named
`aiops-secrets`. `values.schema.json` enforces required fields, replica
bounds (1-20) and the `pullPolicy` enum. Templates reproduce the security
baseline from `deploy/kubernetes`: non-root containers, read-only root
filesystem, `drop: [ALL]` capabilities, `automountServiceAccountToken: false`,
`seccompProfile: RuntimeDefault` and the `restricted` pod security namespace
labels. Ten Go contract tests guard the chart structure, values and security
baseline.

**Supply chain (M38C).** Releases build multi-architecture OCI images
(`linux/amd64`, `linux/arm64`) with `docker buildx` and QEMU, generate SPDX
SBOMs with `syft v1.27.0`, and bundle the Helm chart, license allowlist,
OpenAPI, dependency licenses and SHA256 manifest. `docker push` remains
forbidden; the release is package-only per ADR 0028. A license allowlist
(`docs/security/license-allowlist.json`) admits `MIT`, `ISC`,
`BSD-2-Clause`, `BSD-3-Clause` and `Apache-2.0` only; reciprocal and unknown
licenses fail the gate. `SECURITY.md` documents the supported version policy,
private vulnerability reporting channels, disclosure timeline, threat-model
boundaries and supply-chain controls. `CHANGELOG.md` follows Keep a Changelog
1.1.0 / SemVer 2.0.0. Both are enforced as tracked delivery assets. See ADR
0051.
