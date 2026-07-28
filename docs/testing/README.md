# Testing Strategy

## API Contract and Observability

- `backend/internal/httpserver/metrics_test.go` verifies route-template labels,
  status classes and duration aggregates.
- `backend/internal/httpserver/router_test.go` verifies the `/metrics` content
  type and scrape output without requiring authentication.
- `TestRegisteredRoutesMatchOpenAPI` builds the complete conditional Gin route
  registry and compares it bidirectionally with `docs/api/openapi.yaml`. Adding,
  removing or changing a method/path on only one side fails `go test ./...`.
- A production scrape test must confirm the endpoint is reachable only from
  the monitoring network; this is deployment validation and is not claimed by
  the local unit suite.
- `backend/internal/deployment/kubernetes_test.go` parses the Kustomize
  resources and checks Secret separation, internal Services, frontend-only
  Ingress, default-deny policy, probes, resource limits and restricted pod
  security contexts.
- `kubectl kustomize deploy/kubernetes` is the offline manifest rendering gate.
  Applying to a cluster and kind scenario tests remain separate environment
  gates and must not be reported as passed without a current context.

测试分为四层：

1. Go 领域与 HTTP 单元测试。
2. PostgreSQL、Kubernetes fake client 和 API 集成测试。
3. Vue 组件与状态测试。
4. Playwright 关键工作流和 kind 故障场景测试。

每个阶段的变更记录必须写明实际执行的命令和结果，未执行的测试不得标记为通过。

## Bounded Multi-Cluster Health Tests

- Service tests must verify enabled-cluster filtering, stable ID order, the
  20-cluster hard cap, four-worker maximum concurrency and one timeout shared by
  all fixed resource reads for a cluster.
- Node/Pod/Deployment health must report sampled, total and complete values.
  Empty Node collections are degraded; any truncated sample is partial and
  must not be labeled healthy.
- One resource failure or timeout must return a sanitized scope/code in that
  cluster item without discarding successful peers. Only cluster-directory
  failure may fail the complete request.
- HTTP/OpenAPI tests must reject `limit` outside 1..20 and keep the endpoint
  authenticated for all existing logged-in roles. No cluster credential, API
  server, upstream error body or Kubernetes object may enter the response.
- Dashboard acceptance must cover status/count/latency display, row-driven
  selected-cluster switching, local table scrolling at 390x844 and zero
  document-level horizontal overflow.

## Bounded Global Resource Search Tests

- Service tests cover enabled-cluster filtering, stable ID/kind/Namespace/name
  ordering, 20-cluster admission, four-worker concurrency, one timeout per
  cluster, global result truncation and explicit omitted-cluster coverage.
- The accepted query shape contains only a 2..64 character name substring, one
  valid Namespace and a unique subset of Pod, Deployment, Service and Ingress.
  Duplicate or unknown kinds, invalid Namespace and out-of-range limits fail.
- Per-kind failures stay local and expose only `TIMEOUT` or `QUERY_FAILED`;
  upstream errors, credentials, API Servers and raw Kubernetes objects must not
  enter the response.
- Frontend/API tests cover fixed query serialization. Browser acceptance must
  cover URL restoration, kind selection, request replacement, coverage/failure
  summaries, table-local scrolling and exact Workloads deep-link navigation at
  desktop and 390x844 mobile widths.
- `scripts/e2e-global-search-kind.ps1` is the isolated physical two-cluster
  gate. It must cover anonymous/invalid requests, fixed-kind cross-cluster
  matches, stable ordering, canonical kind selection, cluster/result limits,
  coverage, timeout/recovery/query-failure isolation and positive/negative
  observer RBAC, then restore the initial kind cluster set and remove every
  disposable platform/runtime/credential asset.

## User-Owned Saved Search Filter Tests

- Service tests cover Unicode name length and trimming, fixed-query
  normalization, canonical kind order, invalid/partial overwrite rejection,
  repository error propagation, actor ownership forwarding and stale
  schema/query compatibility projection.
- HTTP tests inject the authenticated actor through `requestctx`, require
  strict JSON, verify 200/201/204 behavior and stable 400/404/409/500 codes.
  Another user's ID must follow the same owner predicate and not-found response.
- PostgreSQL runtime acceptance applies migration 000015, verifies the
  case-insensitive expression index and sends 22 concurrent creates for an
  empty user: exactly 20 must succeed and two must return the cap conflict.
- Mutation audit acceptance checks create/update/delete success and failure
  entries and confirms details contain only route metadata and filter identity,
  never the saved query body.
- Frontend/API tests cover the fixed CRUD payloads. Browser acceptance covers
  save, apply, rename, complete overwrite, URL restoration, limit/incompatible
  presentation and responsive layout. Applying is disabled for incompatible
  items, while complete overwrite remains available to repair them.

## Authentication and User Management Tests

- 创建用户测试必须断言用户名/显示名规范化、bcrypt 摘要不等于明文、角色去重排序和非法角色拒绝。
- 更新测试必须覆盖当前用户自我保护、最后一个活跃系统管理员保护，以及角色变更/停用后的刷新会话撤销。
- 认证测试必须确认服务端使用数据库中的当前状态与角色，而不是继续信任访问令牌签发时的旧角色；停用账号的未过期访问令牌应立即返回 403。
- PostgreSQL/API 验证需覆盖重复用户名 409、非法输入 400、并发管理员降权后仍保留至少一个活跃系统管理员，以及 `user.create`/`user.update` 的 success/failure 审计。
- 页面验收至少覆盖系统管理员可见导航、账号创建、角色展示、启停操作、自身危险操作禁用和无前端运行错误。
- 密码重置测试必须断言新值以 bcrypt 摘要保存、`auth_version` 原子递增、全部刷新令牌撤销、旧访问令牌立即 401、旧密码失败而新密码成功。
- 管理员不得通过管理接口重置自己的密码；成功和失败请求都应记录 `user.password.reset`，且审计详情不得包含密码。
- 主动改密必须覆盖错误当前密码 401、重复密码 409、成功 204、旧访问令牌/刷新会话/旧密码失效、新密码登录成功和凭据版本递增。
- compare-and-swap 测试需确认并发管理员重置后，旧改密请求不能覆盖新摘要；`auth.password.change` 审计不得包含两种密码。
- 会话管理测试必须使用至少两个独立 Cookie，断言唯一当前标记、当前会话 409 保护、指定撤销后刷新 401、批量撤销保留当前刷新，以及匿名拒绝。
- 会话响应和审计不得包含 `token_hash`、Cookie 或令牌原文；跨用户 session ID 必须表现为不存在。

## Kubernetes Gateway Tests

- 单元测试使用内存 Gateway stub 验证 cluster ID、Namespace 路径、selector、分页与日志参数。
- TLS 集成测试使用 `httptest` 验证 bearer token、`/version` 和客户端缓存失效。
- 端到端开发验证可启动仅监听 `127.0.0.1` 的临时 HTTPS API，返回固定 Namespace、Pod、Event 和日志；验证完成后必须删除临时集群记录并停止进程。
- 真实集群验收仍需 kind 或 Docker Desktop Kubernetes，临时 API 不能替代 RBAC、超时和版本兼容验证。
- 凭据轮换必须覆盖非法配置不落库、密文不含明文 token、API Server 原子更新、版本/探测时间清空、三个 Unknown Condition、缓存失效和显式重新探测。
- 系统管理员可轮换；其他角色和匿名请求必须拒绝。success/failure/denied 均记录 `cluster.credentials.rotate`，审计详情不得包含 kubeconfig 或 token。

## Common Workload and Policy Resource Tests

- StatefulSet、DaemonSet、ReplicaSet、Job、CronJob、HPA、ResourceQuota、LimitRange 和 Secret 必须分别覆盖固定 list/detail 路径、Namespace、名称、分页以及 OpenAPI 路由漂移。
- 新工作负载的 Pod template 只保留 container name/image；不得把 template labels、annotations 或任意未声明字段带入公开模型。
- HPA 公开模型不得包含 metric selector、behavior 或当前 metric 内部细节；Conditions 与空 metrics 集合必须保持可解释且可安全序列化。
- Secret 响应只能包含最小 metadata、type、immutable 和排序后的 `dataKeys`。测试必须对原始 JSON 断言不存在 `data`、`stringData`、labels、annotations 以及 fixture 中的诱饵值。
- 目标集群 observer RBAC 对新增资源仅允许 `get/list`；至少使用 SubjectAccessReview 证明 Secret/HPA 可读、Secret create 被拒绝。文档必须同时声明 RBAC 无法按字段裁剪 Secret 值的剩余风险。
- `/workloads` 必须为九类资源提供精确 URL 深链、类型化摘要、独立详情/关联 Event 加载，并保持 Secret 详情无 Labels 区域。
- 1280x720 验收需覆盖多行资源类型标签、代表性列表和详情；390x844 需确认分类/类型标签完整换行、宽表只在面板内滚动、详情抽屉等于可用视口宽度、HPA Conditions 和 Secret key 名无重叠或页面级溢出。

## Kubernetes Event Center Tests

- 后端 Event 合同必须覆盖 `action`、`eventTime`、`reportingComponent`、
  `reportingInstance` 与 `series.count/lastObservedTime`，同时保留旧版时间和
  次数字段的兼容读取。
- 资源名称筛选必须转换为 Kubernetes Event field selector，Namespace 与名称
  组合时不得丢失任一查询条件。
- 前端 API 测试必须断言 Namespace 和资源名称查询序列化；事件页面需按最新
  观察时间排序，并以 series 字段优先计算最后时间与次数。
- 页面验收至少覆盖集群/Namespace/类型/资源类型/资源名筛选、摘要计数、
  Warning/Normal 展示和详情抽屉的首次/最后时间、次数、Action、上报组件与实例。
- 390x844 验收必须确认页面本身无横向溢出、宽表只在事件面板内部滚动、
  抽屉宽度为可用视口宽度且详情字段单列；1440x1000 验收必须确认长消息
  截断和导航高亮。
- `/events` 对所有已登录角色可读；`/notifications` 是独立的诊断通知投递页，
  继续只允许系统管理员和安全审计员访问。

## Operations Cockpit and Resource Topology Tests

- Dashboard 必须并发读取所选集群的 Node、Pod、Deployment、Service 和 Event，
  单个失败以明确错误状态呈现，不得把 0 伪装为健康数据。
- Node/Pod Metrics 必须使用固定 `metrics.k8s.io/v1beta1` 路径、100 条分页上限与
  10 MiB 上游响应上限；404 必须返回 `424 METRICS_API_UNAVAILABLE`，不得伪装为
  空列表或拖垮核心 Dashboard。
- quantity 测试必须覆盖 CPU `n/u/m/core`、内存 Ki/Mi/Gi 与十进制单位、非法值、
  多容器聚合和显式格式单位；没有真实分母时禁止输出利用率百分比。
- available path 必须使用名称完全匹配的 Node `status.allocatable` 作为分母；缺失、
  非法或零 allocatable 时继续显示绝对用量但不得显示百分比。
- Pod 消费排行必须分别覆盖 CPU/Memory 排序、有界前五名、`loaded/total` 采样覆盖率
  和携带 cluster/kind/namespace/name 的资源深链；不得把未加载 Pod 伪装成零用量。
- 目标集群 RBAC 只允许 metrics.k8s.io Node/Pod 的 get/list，SubjectAccessReview
  必须确认 create 被拒绝。真实无 Metrics Server 环境需同时确认核心资源 200、
  两个 Metrics 端点 424 和 Dashboard 明确不可用状态。
- 真实 Metrics Server 环境需确认直连与平台 Node/Pod Metrics 均非空、默认部署不
  引入 Metrics Server，并同时回归诊断、受控处置和 unavailable path。
- Pod 健康算法必须覆盖 Running/Ready、Succeeded、Pending、Unknown、Failed、
  ImagePullBackOff、ErrImagePull 和 CrashLoopBackOff；容器级严重状态优先于 phase。
- Deployment 健康算法必须覆盖全部就绪、部分就绪、全不可用和 scale-to-zero。
- selector 关系只有在键值全部匹配且资源与 Pod 位于同一 Namespace 时成立；
  空 selector、缺失 labels 和跨 Namespace 同标签必须不匹配。
- 真实页面至少选择一个 Service，断言只高亮实际关联的 Service、Pod、Deployment，
  并确认检查器关联数量与可见高亮一致。
- 390x844 下页面 `scrollWidth` 必须等于 `clientWidth`；宽拓扑只允许在画布内部
  横向滚动，侧栏、顶栏、筛选器、指标和检查器不得造成页面级溢出。
- Ingress 后端只允许连接同 Namespace、同名称 Service，并保留命名端口或数字端口。
- EndpointSlice 只允许按 `kubernetes.io/service-name` 派生的精确 Service 身份连接；
  endpoint 只允许连接 kind=Pod、名称和 Namespace 均精确匹配的 targetRef。
- Service 只有在不存在匹配 EndpointSlice 时才允许 selector 回退；存在空或未知
  targetRef 的匹配 EndpointSlice 时不得绕过 discovery 数据推测 Pod。
- `ready=false` endpoint 必须继续显示并标记为注意状态；零 endpoint 的切片标记为异常。
  上游 `ports` 或 `endpoints` 为 null 时公共响应必须规范化，页面不得白屏。

## Diagnosis Rule Tests

- 每条规则至少包含一个命中样例和一个不命中样例。
- 断言规则 ID、资源引用、严重级别和证据数量，不只断言自然语言摘要。
- CrashLoopBackOff 必须断言上一次终止状态进入证据，普通 Completed Pod 不命中。
- Pending 必须断言 PodScheduled/FailedScheduling 证据，Running Pod 不命中。
- OOMKilled 必须覆盖当前终止状态和上一次终止状态，且保留退出码与重启次数。
- Service 无 Endpoint 必须覆盖 selector Service 命中，以及 ExternalName、无 selector、存在 Ready address 三类不命中边界。
- Node NotReady 必须覆盖 Ready=False、Ready=True 和 Ready Condition 缺失，并保留完整 Conditions。
- Deployment 副本不可用必须覆盖显式期望副本、Kubernetes 默认 1 副本、健康副本和 scale-to-zero 语义。
- EndpointSlice 测试必须覆盖 `ready=true`、`ready=false`、`ready=nil` 与多地址计数；只允许 discovery 404 触发 Endpoints 回退，403 等错误不得回退。
- Service 诊断证据继续提供 ready/not-ready/group 数量，并标记 `discovery.k8s.io/v1` 或 `core/v1` 来源。
- 端到端测试同时校验 API 证据数量与数据库 `diagnosis_evidence` 行数，验证事务持久化。
- 历史详情测试必须验证列表不展开证据、详情能按记录 ID 恢复全部证据。
- 状态机测试必须覆盖所有合法边和至少一个跨级非法跳转；非法跳转返回 409 且不得追加活动。
- 数据库验证应确认首次流转设置负责人、每次流转追加活动、反馈只追加，以及删除测试集群后全部级联清理。
- SLA 测试必须覆盖严重级别时限、`overdue=true/false` 筛选、逾期汇总、解决停止计时和重新打开沿用原截止时间。
- 转派测试必须覆盖活跃角色过滤、首次分配、再次转派、重复负责人 409、名称快照历史和成功/失败审计。
- Dashboard 验证应使用诊断汇总 API，不以目标 Kubernetes API 可达作为加载前提。

## Audit Tests

- 单元测试覆盖写路由到 action 的映射、success/failure/denied 结果映射，以及读请求不进入审计。
- 集成验证必须至少制造一次成功、业务失败和权限拒绝，并检查请求 ID、操作者与 HTTP 状态。
- 删除测试集群后，审计外键应置空，但资源字段和 `details.cluster_id` 快照必须保留。
- 审计详情和日志输出不得出现密码、token、Cookie、kubeconfig 或请求体。
- 安全审计员与系统管理员可读；运维管理员和只读用户不得访问审计列表。
- 规则不得把预设根因写入 evidence；evidence 只能来自本次采集的资源状态、Event、日志或指标。
- CSV 导出测试必须断言稳定列顺序、UTF-8 BOM、UTC 时间、筛选复用和 5000 行上限/截断响应头。
- 操作者、资源、请求 ID、来源和 User-Agent 以 `=`、`+`、`-`、`@` 开头或前置空白时，导出必须中和电子表格公式。
- 系统管理员和安全审计员可导出；只读用户、运维管理员与匿名请求必须拒绝，成功/失败/拒绝结果均写入 `audit.export`。

## AI Explanation Tests

- Provider 单元测试必须验证 `/responses`、Bearer Header、`store=false`、严格 JSON Schema、响应 ID和 token 用量解析。
- 提示构建测试必须验证敏感键脱敏、UTF-8 安全裁剪和总上下文上限。
- 任何不存在的 evidence ID、空引用或非法优先级都必须拒绝入库。
- 集成测试使用本地模拟 Provider，不依赖真实 API Key；成功结果应可从历史读取并与原证据映射。
- 停止模拟 Provider 后应返回 502，解释历史数量不变，同时产生 failure 审计。
- 页面测试应验证解释边界提示、引用标签、重新生成追加历史，以及规则证据始终可见。
- Provider 请求必须断言 `max_output_tokens`；配置测试覆盖预算、并发与输出上限的默认值和非法范围。
- 并发测试应阻塞首个 Provider 调用，确认第二个调用返回 `AI_BUSY`，且未进入 Provider。
- 预算测试应确认请求执行期间 `active_requests` 与 `reserved_tokens` 可观测；完成后预留清零、实际 usage 计入 UTC 当日用量，余额不足时返回 `AI_BUDGET_EXCEEDED` 且历史不增加。
- PostgreSQL 集成验证必须覆盖多个请求共享的 advisory-lock 预算检查，并确认失败、成功和过期路径均不留下永久预留。
- AI 质量反馈测试必须覆盖三种合法结论、非法结论 400、同一用户重复提交 409 和不存在解释 404。
- 解释列表只能返回聚合统计与当前用户自己的评价；不同用户读取同一解释时 `my_feedback` 必须隔离。
- 质量汇总应核对总数、严格有帮助率、贡献者、已评价解释数与按模型分组；反馈不得修改解释正文或诊断证据。
- 页面验收应使用只读角色提交评价，确认该能力不依赖诊断处置权限，并在提交后锁定本人的评价入口。

## Diagnosis Notification Tests

- Migration verification must create one outbox row for diagnosis creation,
  status change and assignment change in the same transaction; unrelated
  updates and disabled settings must not enqueue rows.
- Worker tests must assert the exact body HMAC, stable event ID/type headers,
  2xx success, non-2xx retry, maximum attempts, capped delay, stale-claim
  recovery and redirect rejection.
- Persisted payload and last-error text must not contain evidence, workflow
  comments, credentials, signing secret, response body or full webhook URL.
- Multi-worker repository tests must preserve disjoint `SKIP LOCKED` claims.
  Manual retry may transition only `dead` to `pending`.
- API tests must cover system-admin/security-auditor list access,
  system-admin-only retry, anonymous denial, disabled configuration and audit
  results. Delivery list JSON must never expose `payload`.
- Frontend tests must cover filter serialization, empty 202 handling, role
  visibility and retry controls. Real receiver smoke tests must use loopback or
  an isolated endpoint and remove all QA events afterward.

## Controlled Operations Tests

- Diagnosis-bound rollout preview must require a confirmed Pod diagnosis, read
  the current Pod and Deployment, enforce namespace/selector/UID binding, and
  send exactly one `PATCH` with `dryRun=All`.
- Resource-origin preview accepts only `deployment.scale`, `cronjob.suspend` or
  `cronjob.resume`. Tests must reject unknown/irrelevant JSON fields, replica
  counts outside 0..1000 and no-change requests.
- The persisted plan must contain only a catalog action, target
  UID/resourceVersion, typed before/after values, expiry and token hash. A
  resource-origin plan must have no diagnosis ID. API responses and audit
  details must not contain the token hash, raw token, idempotency key,
  kubeconfig or arbitrary patch.
- Deployment and CronJob patches must be server-generated, include captured UID
  and resourceVersion, pass `dryRun=All`, and dispatch only from the persisted
  action. Operation history must be resource-filtered and bounded to 50.
- Execution tests must cover explicit confirmation, exact resourceVersion
  precondition, success, sanitized Kubernetes failure, expiry, invalid token,
  short/long idempotency key, same-key replay without a second Kubernetes
  request, different-key conflict and stale-lease recovery.
- API/RBAC tests must cover system/operations administrators, viewer read-only
  plan history, anonymous denial, and success/failure/denied audit outcomes.
- A loopback Kubernetes API smoke must assert one server-side dry-run and one
  real patch for a successful plan, then remove the test cluster, diagnosis,
  plan and temporary credentials.

## Real kind diagnosis and controlled-operations gate

The repeatable fixtures under `deploy/demo-scenarios` are covered by Go
contract tests and were most recently exercised on 2026-07-27 against a real
kind Kubernetes v1.34.0 API. A complete run must verify:

1. both healthy Nginx Deployments become Ready;
2. the image and crash Pods reach the intended waiting states;
3. the Service has zero ready EndpointSlice addresses;
4. the platform matches all seven deterministic diagnosis rule IDs;
5. confirmed remediation passes server dry-run, executes once and replays the
   same idempotency key without a second patch;
6. Deployment scale executes once, same-key replay does not patch again, and a
   second controlled plan restores the original replica count;
7. CronJob resume and suspend both execute through confirmed plans and restore
   the original suspend state;
8. the remediator can patch Deployments and CronJobs only in the explicitly
   approved Namespace, while Pod deletion and `kube-system` patches are denied;
9. all nine M17 fixtures pass fixed list/detail reads, Secret exposes only the
   expected key name, and Secret create remains denied while HPA list is allowed;
10. credentials and QA database rows are removed after the default run, unless
   the explicit retained-demo mode is requested for browser acceptance.

The current result is archived in
`.artifacts/e2e-kind/e2e-kind-20260727-180557.json` and
`docs/changes/2026-07-27-controlled-operations-catalog.md`.

## M18 evidence-based diagnosis gate

The M18 extension is covered by `backend/internal/diagnosis/m18_rules_test.go`
and `testdata/m18-fixtures.json`. Positive and negative snapshots cover Node
pressure, PVC Pending with exact UID-linked Warning Events, HPA saturation and
Ingress backends without Ready addresses. A complete real-kind run must verify
all seven expected rule IDs, including the four M18 IDs, and must delete the
synthetic pressure Node in every success or failure path. The HPA status is
patched immediately before diagnosis because its controller may replace a
synthetic status. The current run is archived in
`docs/changes/2026-07-27-evidence-based-diagnosis-expansion.md`.

The gate deliberately does not claim sustained restarts: the platform has no
temporal Pod sampling window or two-snapshot comparison. A cumulative
`restartCount` is insufficient evidence for that conclusion.

## M19 controlled-operations gate

M19 must keep the rollout regression while independently covering both
resource kinds and all three resource actions. Service and HTTP tests cover
strict request decoding, role boundaries, typed response diffs and bounded
history. Repository/migration tests cover nullable diagnosis origin, the four
valid action/parameter shapes and rejection of every mixed shape. Kubernetes
gateway tests cover exact Deployment/CronJob paths, UID/resourceVersion patches,
server-side dry-run and sanitized upstream failures.

The accepted full gate is
`.artifacts/verification/verify-20260727-180428.json`: 128 Go test entries,
12 Vitest files / 56 tests, Kustomize 16/5/22/3 and three healthy Compose
services with migration `000014` applied. The accepted real-kind run is
`.artifacts/e2e-kind/e2e-kind-20260727-180557.json`; it confirms scale/replay,
fixture restoration, CronJob resume/suspend and namespaced RBAC. Desktop and
390x844 browser acceptance, including the single-overlay-scrollbar regression,
is archived in `docs/changes/2026-07-27-controlled-operations-catalog.md`.

## Real kind Node and Deployment diagnosis gate

`deploy/diagnosis-e2e` and `scripts/e2e-diagnosis-kind.ps1` form a separate,
disposable gate for the M8 Node/Deployment rules. A complete run must verify:

1. a synthetic unschedulable Node reports Ready=False through the status subresource;
2. the stalled Deployment reaches desired/current/ready/available/unavailable = 2/2/0/0/2;
3. the platform resource APIs can read both resources through a 30-minute observer credential;
4. `node.not_ready.v1` and `deployment.replicas_unavailable.v1` are persisted with `node_condition` and `deployment_status` evidence;
5. observer RBAC returns yes/yes/no/no for list Nodes, get Deployments, patch Deployments and patch Nodes;
6. platform cluster/diagnosis rows, the timestamped kind cluster and both temporary files are removed;
7. the pre-existing kind cluster set is unchanged.

The 2026-07-26 run passed on kind v0.30.0 / Kubernetes v1.34.0. It is
archived in `docs/changes/2026-07-26-node-deployment-real-kind-e2e.md`. The
fixtures are intentionally excluded from the retained defense demo.

## Application Credential Key Re-encryption Tests

- Unit tests cover active/legacy key selection, invalid versions, dry-run
  no-write behavior, plaintext-preserving conversion, sanitized unknown-version
  failure, full-batch rollback and both preflight/concurrent record limits.
- Migration and repository behavior are exercised against isolated PostgreSQL
  by `scripts/e2e-credential-reencryption.ps1`, including real API-created
  ciphertext and the `FOR UPDATE SKIP LOCKED` apply path.
- A passing physical run requires v1 rows to remain unchanged after dry-run, a
  corrupt second row to leave the first row unchanged, successful v1-to-v2
  conversion, zero remaining rows, v2-only backend decryption, three sanitized
  audit rows and all cleanup assertions.
- Tests and evidence must never print or retain keys, plaintext kubeconfigs,
  ciphertext, database URLs or raw database/application errors.

## Signed Audit Archive Tests

- Unit tests verify Ed25519 success with a separately trusted public key,
  payload tamper rejection, signer mismatch rejection, strict metadata/range
  checks and non-overwriting output.
- Repository selection must use ascending positive IDs in a read-only
  repeatable-read snapshot, count before loading and reject bounds outside
  1..10000. An over-limit selection must create neither payload nor manifest.
- The isolated PostgreSQL gate must create only synthetic sanitized audit rows,
  prove trusted verification and one-byte tamper rejection, and delete the
  private seed, trusted-key file, archives, container, network, image and
  process environment in `finally`.
- Retained evidence may contain only format-independent counts, booleans and
  cleanup state. It must not contain payloads, manifests, keys, database URLs,
  request bodies, credentials or raw errors.

## Delivery verification entry points

- `.github/workflows/ci.yml` is the hosted regular gate. It reproduces backend,
  frontend, Kustomize, isolated credential re-encryption, identity/recovery
  readiness, isolated PostgreSQL backup/restore, Compose health and HTTP checks
  on Ubuntu with no PR secret and unconditional ephemeral runtime teardown.
- `.github/workflows/release.yml` must call the complete reusable CI, reject
  non-semantic versions, keep manual runs package-only and produce
  `SHA256SUMS` before a verified tag may create a GitHub Release.
- `.github/workflows/real-kind-e2e.yml` runs only disposable suites on the
  dedicated `aiops-kind` self-hosted Windows runner. It is scheduled/manual,
  non-cancelling and may upload only sanitized JSON evidence.
- `ci_workflows_test.go` parses workflow/Dependabot/actionlint YAML and enforces
  triggers, least permission, action pins, runner labels, cleanup markers and
  the absence of `pull_request_target`, PR secrets, registry push and retained
  demo mode. Workflow changes also require actionlint.
- `scripts/verify.ps1` is the local release gate. It runs backend vet/test/build,
  frontend typecheck/test/build, Compose config/build/health, four Kustomize
  renders and backend/frontend/proxy HTTP checks. Sanitized results are written
  to `.artifacts/verification`.
- `scripts/e2e-kind.ps1` is the real-cluster gate. It applies the isolated demo
  and RBAC fixtures, imports an in-memory short-lived credential, matches all
  seven diagnoses, executes/replays rollout restart and Deployment scale,
  restores the original replica count, resumes then re-suspends the CronJob,
  checks positive/negative Deployment and CronJob RBAC, verifies all M17 fixed
  resources and restores fixtures again in `finally`. It removes the platform
  cluster record unless retained-demo mode is explicit. Sanitized results are
  written to `.artifacts/e2e-kind`.
- `scripts/e2e-metrics-kind.ps1` verifies the checksum-pinned optional Metrics
  Server available path, then invokes the complete real-cluster gate; its
  sanitized results are written to `.artifacts/metrics-e2e`.
- `scripts/e2e-diagnosis-kind.ps1` is the isolated read-only Node/Deployment
  gate. It creates and destroys its own timestamped kind cluster and writes
  sanitized results to `.artifacts/diagnosis-e2e`.
- `scripts/e2e-fleet-kind.ps1` is the isolated two-cluster fleet gate. It builds
  the current backend source, starts a private PostgreSQL/backend runtime and
  two digest-pinned kind clusters, compares direct fixed-resource totals,
  injects timeout and unavailable states, verifies recovery/RBAC/ordering and
  requires complete cleanup. Sanitized results are written to
  `.artifacts/fleet-e2e`.
- `scripts/e2e-global-search-kind.ps1` is the isolated two-cluster fixed-kind
  search gate. It builds the current backend in a private runtime, creates two
  digest-pinned kind clusters and controlled Pod/Deployment/Service/Ingress
  matches, verifies ordering/coverage/truncation and fault isolation, and
  requires complete cleanup. Sanitized results are written to
  `.artifacts/search-e2e`.
- `scripts/e2e-postgres-backup-restore.ps1` is the isolated PostgreSQL 17
  recovery gate. It applies every migration and synthetic relational fixtures,
  destroys the source after a custom-format dump, restores a fresh target,
  compares sanitized invariants and requires complete cleanup. Results are
  written to `.artifacts/postgres-recovery`; the dump itself is never retained.
- `scripts/e2e-credential-reencryption.ps1` is the isolated application-key
  gate. It creates v1 ciphertext through the real API, proves dry-run no-write
  behavior and whole-batch rollback, applies v1-to-v2 conversion, verifies a
  v2-only backend decrypt path and requires complete runtime cleanup. Only
  sanitized counts and error codes are written to
  `.artifacts/credential-reencryption`.
- `scripts/e2e-audit-archive.ps1` is the isolated signed-audit gate. It seeds
  synthetic sanitized rows in a private PostgreSQL instance, signs and verifies
  two records against an externally supplied public key, proves record-limit
  refusal leaves no files, rejects a byte mutation and deletes all key/archive,
  image, network, container and process material. Only sanitized booleans and
  counts are written to `.artifacts/audit-archive`.
- `scripts/e2e-identity-readiness.ps1` is the offline identity admission gate.
  It runs the production-image command with networking disabled, accepts a
  complete synthetic OIDC/MFA contract, rejects issuer/PKCE and MFA/email-linking
  downgrades, deletes every temporary provider snapshot and image, and writes
  only sanitized booleans/counts to `.artifacts/identity-readiness`.
- `scripts/e2e-recovery-readiness.ps1` is the offline recovery-policy admission
  gate. It consumes the newest real logical-restore evidence, runs the
  production command without networking, accepts 15 implementation-readiness
  checks, rejects inadequate copies/stale evidence/retained dump/incomplete
  cleanup, and writes only sanitized booleans/counts to
  `.artifacts/recovery-readiness` while keeping production validation false.
- Saved-filter runtime acceptance uses the retained development PostgreSQL and
  authenticated API because the state is a platform preference and does not
  require a second target cluster. Test-created filters must be removed after
  the run; observer credentials remain short-lived.
- `scripts/generate-license-report.ps1` refreshes the thesis dependency-license
  inventory after module or package changes.
- `scripts/demo-up.ps1` retains a successful real E2E as populated defense data;
  `scripts/demo-down.ps1` removes only the namespaced demo lifecycle and
  `demo-kind-*` platform records.
- `scripts/capture-thesis-screenshots.ps1` uses the installed Edge/Chrome CDP
  endpoint to capture authenticated pages without installing browser packages.
- The current M5 result is archived in
  `docs/changes/2026-07-26-delivery-packaging.md`; thesis-facing coverage is in
  `docs/thesis/test-matrix.md`.
