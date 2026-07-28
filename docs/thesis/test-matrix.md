# Test Matrix

更新时间：2026-07-27

测试结论以本轮命令输出和 `.artifacts/` 中的脱敏证据为准。未实际执行的环境测试不得标记为通过。

## 自动化层级

| 层级 | 范围 | 主要入口 | 当前覆盖 |
|---|---|---|---|
| Backend 单元/集成 | 领域规则、仓储事务、HTTP/RBAC、OpenAPI、部署合同、fleet fan-out、全局搜索与私有保存筛选器 | `go vet ./...`、`go test -p=1 -count=1 ./...` | 152 个 Go `Test*` 入口，包含 PostgreSQL、httptest、Kubernetes fake/loopback 测试 |
| Frontend 单元 | API 参数序列化、认证、集群、资源健康、资源指标、拓扑、事件、诊断、审计、通知、受控操作、fleet、全局搜索与保存筛选器 | `pnpm typecheck`、`pnpm test -- --run` | 14 个 Vitest 文件，当前基线 59 个用例 |
| Build | Go 二进制、Vue 生产资源、Docker 镜像 | `scripts/verify.ps1` | Go 1.25 容器构建、Vite build、Compose build |
| Manifest contract | 平台部署、目标集群 RBAC、演示与独立诊断清单 | `kubectl kustomize` + `backend/internal/deployment` | Secret 分离、探针、资源限制、NetworkPolicy、最小 RBAC、演示和合成故障场景 |
| Real kind E2E | 真实 Kubernetes API、短期凭据、7 条保留环境诊断、2 条独立诊断、处置和权限边界 | `scripts/e2e-kind.ps1`、`scripts/e2e-diagnosis-kind.ps1` | `kind-aiops-test` + 一次性 kind / Kubernetes v1.34.0 |

## 关键需求追踪

| 需求 | Backend | Frontend | Real kind | 通过标准 |
|---|---|---|---|---|
| 登录与当前角色生效 | auth service/router tests | `auth.test.ts` | API 登录 | 停用、改角色或改密后旧会话立即失效 |
| 集群凭据安全 | credential/cluster repository tests | `clusters.test.ts` | 短期 ServiceAccount token 接入 | 明文不落库、不回显，探测返回 Ready Conditions |
| 多集群只读资源 | Kubernetes gateway/sanitization tests | `kubernetes.test.ts`、`kubernetes-events.test.ts` | 17 类列表/详情与 Event 查询 | 所有请求显式绑定 `cluster_id`；ConfigMap/Secret 值、StorageClass 参数和 Secret labels/annotations 不回显；Event 精确匹配资源 |
| 常用工作负载与策略 | 九类固定路由、公开模型、空集合和 RBAC tests | `kubernetes.test.ts`、`kubernetes-events.test.ts` | 九类真实 fixture 与 list/detail 深链 | StatefulSet/DaemonSet/ReplicaSet/Job/CronJob/HPA/ResourceQuota/LimitRange/Secret 均可读；Secret 只返回 key 名；无任意 GVK/写代理 |
| 真实资源指标 | fixed Metrics contracts/error tests | `resource-metrics.test.ts`、`kubernetes.test.ts` | Metrics API 缺失/可用 + SubjectAccessReview | unavailable path 保持 424；available path Node/Pod Metrics 非空；利用率仅使用同名 Node allocatable；Pod 排行有界且显示覆盖率；get/list 允许而 create 拒绝 |
| ImagePullBackOff | rule and diagnosis tests | `diagnosis.test.ts` | 故障 Pod | 命中 `pod.image_pull_backoff.v1` 并保存证据 |
| CrashLoopBackOff | rule and previous-state tests | `diagnosis.test.ts` | 故障 Pod | 命中 `pod.crash_loop_backoff.v1` 且包含上次终止状态 |
| Pod Pending | scheduling condition/event rule tests | `diagnosis.test.ts` | Pending Pod | 命中 `pod.pending.v1` 并保存 PodScheduled/FailedScheduling 证据 |
| Pod OOMKilled | termination state rule tests | `diagnosis.test.ts` | OOMKilled Pod | 命中 `pod.oom_killed.v1` 并保存退出码、重启次数和 Event 证据 |
| Service 无就绪端点 | EndpointSlice/Endpoints tests | `diagnosis.test.ts` | 错误 selector Service | 命中 `service.no_ready_endpoints.v1` |
| Node NotReady | Node Condition rule/gateway tests | `diagnosis.test.ts` | 独立合成 Node，Ready=False | 命中 `node.not_ready.v1` 并保存 2 条 Condition 证据；健康 Node 不命中 |
| Deployment 副本不可用 | replica-count rule/gateway tests | `diagnosis.test.ts` | 独立停滞 Deployment，2/2/0/0/2 | 命中 `deployment.replicas_unavailable.v1` 并保存全部副本计数 |
| Node 压力 | `m18_rules_test.go`、Node pressure evaluator | `diagnosis.test.ts`、Workloads UI | 保留 kind 集群上的临时 Ready + MemoryPressure Node | 命中 `node.pressure.v1`，NotReady 优先且保留压力 Condition |
| PVC Pending | exact UID Event、PVC evaluator/fixture tests | `diagnosis.test.ts`、Workloads UI | 缺失 StorageClass PVC + 显式 Warning Event | 命中 `persistentvolumeclaim.pending.v1`；无 Warning 的 WaitForFirstConsumer 不命中 |
| HPA 饱和 | HPA status/metric evaluator/fixture tests | `diagnosis.test.ts`、Workloads UI | maxReplicas + TooManyReplicas status snapshot | 命中 `horizontalpodautoscaler.saturated.v1`；TooFewReplicas 不命中 |
| Ingress 后端不可用 | route extraction, dedup and endpoint evaluator/fixture tests | `diagnosis.test.ts`、Workloads UI | 指向零 Ready Endpoint 的 Service | 命中 `ingress.backend_unavailable.v1`，采集错误不输出部分结论 |
| 人工流程与 SLA | state machine/assignment tests | diagnosis UI/API tests | 确认 ImagePullBackOff | 非法跳转 409，活动和负责人历史追加保存 |
| AI 解释护栏 | schema/citation/budget/concurrency tests | diagnosis API tests | 本地 E2E 默认关闭 | Provider 失败不影响规则；引用必须对应证据 ID |
| 审计与导出 | middleware/repository/CSV tests | `audit.test.ts` | 写操作审计 | RBAC 正确、敏感字段不进入记录、CSV 公式安全 |
| 通知 outbox | trigger/worker/retry tests | `notifications.test.ts` | 本地 E2E 默认关闭 | 事务入队、签名、重试、dead/requeue 行为稳定 |
| 受控 rollout restart | preview/execute/idempotency tests | `diagnosis.test.ts` | preview + execute + replay | 一次 dry-run、一次真实 patch、同键重放不二次写入 |
| Deployment scale | strict request/repository/service/HTTP tests | `diagnosis.test.ts`、Deployment detail | 1→2、同键重放、受控恢复到 1 | 仅接受 0..1000 整数；diff、UID/resourceVersion、dry-run、确认和历史完整；no-change 拒绝 |
| CronJob suspend/resume | strict request/repository/service/gateway tests | `diagnosis.test.ts`、CronJob detail | resume→suspend 并恢复原值 | 状态感知操作、布尔 diff、UID/resourceVersion、dry-run、确认和历史完整；no-change 拒绝 |
| 目标集群最小权限 | deployment contract tests | 不适用 | `kubectl auth can-i` | observer 保持只读；remediator 仅可在 `aiops-demo` get/patch Deployment/CronJob；删除 Pod 和修改 `kube-system` 均拒绝 |
| 有界多集群健康比较 | fleet ordering/concurrency/timeout/partial tests | `fleet.test.ts`、Dashboard | 保留 kind 真实数据路径；并发语义由确定性 stub 验证 | 最多 20 集群/4 并发/4 秒/每类 100 样本；失败局部化、覆盖率显式、无任意查询或新增写权限 |
| 有界全局资源搜索 | global search validation/ordering/concurrency/timeout/coverage tests | `global-search.test.ts`、全局搜索页 | 保留 kind 上的固定 Pod/Deployment/Service/Ingress 读取路径 | 名称 2..64、可选精确 Namespace、20 集群/4 并发/4 秒/每类与全局 100；失败与未搜索集群显式、无任意 GVK/selector/原始对象 |
| 当前用户保存筛选器 | normalization/ownership/compatibility/strict HTTP/audit tests | `global-search.test.ts`、全局搜索页保存筛选器区 | PostgreSQL 000015、API 并发 22 请求与真实 kind 查询复用 | 每用户最多 20 条、大小写不敏感名称唯一、完整覆盖修复不兼容记录、他人 ID 等同 404、审计无查询正文、无共享/selector/GVK/结果持久化 |

## M5 交付门禁

| 门禁 | 证据 | 状态 |
|---|---|---|
| 交付资产合同测试 | `TestDeliveryAssetsCoverVerificationAndThesisMaterials` | 2026-07-26 通过，已纳入全量 Go suite |
| 一键质量门禁 | `.artifacts/verification/verify-20260726-171602.json` | 通过，135.2 秒，三服务 healthy |
| M8 一键质量门禁 | `.artifacts/verification/verify-20260726-190540.json` | 通过，291.75 秒；七条规则代码基线、27 个前端用例、三服务 healthy、Kustomize 16/5/7 |
| M9 Node/Deployment kind E2E | `.artifacts/diagnosis-e2e/diagnosis-e2e-20260726-193724.json` | 通过，Kubernetes v1.34.0；两规则、证据 2/1、RBAC yes/yes/no/no、全量临时资源清理 |
| M9 一键质量门禁 | `.artifacts/verification/verify-20260726-194237.json` | 通过，283.95 秒；108 个 Go 测试入口、27 个前端用例、三服务 healthy、Kustomize 16/5/7/3 |
| M10 一键质量门禁 | `.artifacts/verification/verify-20260727-094104.json` | 通过，142.28 秒；Event API 合同、28 个前端用例、三服务 healthy、Kustomize 16/5/7/3 与运行态 HTTP 检查通过 |
| M11 一键质量门禁 | `.artifacts/verification/verify-20260727-100626.json` | 通过，235.92 秒；资源健康/Namespace 拓扑合同、33 个前端用例、三服务 healthy、Kustomize 16/5/7/3 与运行态 HTTP 检查通过 |
| M12 一键质量门禁 | `.artifacts/verification/verify-20260727-103859.json` | 通过，186.24 秒；四类固定详情 API 与深链接工作台、35 个前端用例、三服务 healthy、Kustomize 16/5/7/3 与运行态 HTTP 检查通过 |
| M13 一键质量门禁 | `.artifacts/verification/verify-20260727-115131.json` | 通过，154.08 秒；八类固定详情、安全裁剪、关联事件、112 个 Go 测试入口、38 个前端用例、三服务 healthy、Kustomize 16/5/10/3 与运行态 HTTP 检查通过 |
| M13 retained kind demo | `.artifacts/e2e-kind/e2e-kind-20260727-114800.json` | 通过，Kubernetes v1.34.0；三规则、处置幂等、RBAC yes/yes/no/no，并保留 `demo-kind-20260727-114759` |
| M14 一键质量门禁 | `.artifacts/verification/verify-20260727-132355.json` | 通过，154.69 秒；固定 EndpointSlice 列表、空集合回归、完整五类拓扑、114 个 Go 测试入口、44 个前端用例、三服务 healthy、Kustomize 16/5/10/3 与运行态 HTTP 检查通过 |
| M14 retained kind demo | `.artifacts/e2e-kind/e2e-kind-20260727-130455.json` | 通过，Kubernetes v1.34.0；真实 Ingress、2 个业务 EndpointSlice、三规则、处置幂等、RBAC yes/yes/no/no，并保留 `demo-kind-20260727-130453` |
| M15 一键质量门禁 | `.artifacts/verification/verify-20260727-135642.json` | 通过，165.03 秒；固定 Node/Pod Metrics、discovery 确认的专用 424、quantity 聚合、119 个 Go 测试入口、12 个 Vitest 文件/49 个用例、三服务 healthy、Kustomize 16/5/10/3 与运行态 HTTP 检查通过 |
| M15 retained kind demo | `.artifacts/e2e-kind/e2e-kind-20260727-134216.json`、`.artifacts/demo/demo-ready-20260727-134216.json` | 通过，Kubernetes v1.34.0；核心 Node 200/1、Node/Pod Metrics 均 424、SAR get/list=true/create=false，三规则和处置幂等通过，并保留 `demo-kind-20260727-134215`（cluster ID 33） |
| M15 Dashboard browser | `docs/changes/2026-07-27-real-resource-metrics-foundation.md` | 通过；1280x720 文档 1265/1265，390x844 文档 375/375；不可用状态、核心资源数据和六张指标卡完整，无 warning/error 日志 |
| M16 Metrics available E2E | `.artifacts/metrics-e2e/metrics-e2e-20260727-142714.json`、`.artifacts/e2e-kind/e2e-kind-20260727-142714.json` | 通过，Kubernetes v1.34.0 / Metrics Server v0.8.0；直连与平台均返回 1 Node/12 Pods，cluster ID 34，三规则和处置幂等继续通过 |
| M16 一键质量门禁 | `.artifacts/verification/verify-20260727-143242.json` | 通过，242.56 秒；119 个 Go 测试入口、12 个 Vitest 文件/51 个用例、三服务 healthy、Kustomize 16/5/10/3 与运行态 HTTP 检查通过 |
| M16 Dashboard browser | `docs/changes/2026-07-27-real-metrics-utilization-consumers.md` | 通过；1280x720 文档 1265/1265，CPU/Memory 利用率、12/12 排行、指标切换和 Pod 深链正确；390x844 文档 375/375、排行单列，无 warning/error 日志 |
| M17 一键质量门禁 | `.artifacts/verification/verify-20260727-155239.json` | 通过，148.86 秒；九类固定 list/detail/OpenAPI/RBAC 合同、121 个 Go 测试入口、12 个 Vitest 文件/54 个用例、三服务 healthy、Kustomize 16/5/19/3 与运行态 HTTP 检查通过 |
| M17 Metrics + real kind E2E | `.artifacts/metrics-e2e/metrics-e2e-20260727-152830.json`、`.artifacts/e2e-kind/e2e-kind-20260727-152830.json`、`.artifacts/api-m17/api-m17-20260727-154748.json` | 通过，Kubernetes v1.34.0 / Metrics Server v0.8.0；九类 fixture list/detail、Secret `dataKeys=[example-key]`、RBAC Secret list=yes/create=no、HPA list=yes、三规则和处置幂等通过；保留 cluster ID 35 |
| M17 workbench browser | `.artifacts/browser-m17/`、`docs/changes/2026-07-27-common-workload-policy-coverage.md` | 通过；1280x720 四分类/八工作负载标签、代表性深链和零溢出；390x844 两列标签、面板内表格滚动、375px 抽屉、Secret 脱敏和 HPA Conditions 修复后无重叠；无 warning/error 日志 |
| M18 real kind diagnosis | `.artifacts/e2e-kind/e2e-kind-20260727-165019.json` | 通过，Kubernetes v1.34.0；7 条规则包含 4 条 M18 规则、Metrics、处置幂等、RBAC 通过；临时压力 Node 清理完成，保留 cluster ID 39 |
| M18 workbench browser | `docs/changes/2026-07-27-evidence-based-diagnosis-expansion.md` | 通过；Ingress/PVC/HPA 入口与 Ingress 证据抽屉可见，390x844 body/dialog 375/375 无横向溢出，浏览器错误日志为空 |
| M18 一键质量门禁 | `.artifacts/verification/verify-20260727-172323.json` | 通过，105.17 秒；123 个 Go 测试入口、12 个 Vitest 文件/55 个用例、Docker 镜像、三服务 healthy、Kustomize 16/5/22/3 与运行态 HTTP 检查通过 |
| M19 一键质量门禁 | `.artifacts/verification/verify-20260727-180428.json` | 通过，143.85 秒；固定操作目录、严格请求/迁移/OpenAPI/RBAC 合同、128 个 Go 测试入口、12 个 Vitest 文件/56 个用例、三服务 healthy、Kustomize 16/5/22/3 与运行态 HTTP 检查通过 |
| M19 real kind operations | `.artifacts/e2e-kind/e2e-kind-20260727-180557.json` | 通过，Kubernetes v1.34.0；7 条诊断与 rollout 回归、Deployment 1→2/同键重放/恢复 1、CronJob resume/suspend/恢复、`aiops-demo` allow 与 `kube-system` deny 均符合预期 |
| M19 workbench browser | `docs/changes/2026-07-27-controlled-operations-catalog.md` | 通过；1280x720 与 390x844 的 Deployment scale、CronJob 状态操作、类型化 diff、确认和资源历史可用；移动端仅一条 overlay 滚动条，无横向溢出或 warning/error 日志 |
| M20 Phase 1 一键质量门禁 | `.artifacts/verification/verify-20260727-190133.json` | 通过，104.53 秒；有界 fleet/OpenAPI/交付合同、133 个 Go 测试入口、13 个 Vitest 文件/57 个用例、三服务 healthy、Kustomize 16/5/22/3 与运行态 HTTP 检查通过 |
| M20 fleet runtime/browser | `docs/changes/2026-07-27-bounded-multi-cluster-health.md` | 通过；两个已启用平台记录中的不可用记录被局部隔离，当前真实 kind 路径返回 1/1 Node、12/15 Pod、5/7 Deployment、10 Warning；1280x720 文档 1265/1265，390x844 文档 375/375 且 780px 表格仅在 277px 容器内滚动，无 warning/error 日志；不声明两个物理集群 |
| M20 Phase 2 双集群 fleet E2E | `.artifacts/fleet-e2e/fleet-e2e-20260727-193711.json` | 通过；两个独立 Kubernetes v1.34.0 kind 集群与隔离平台运行时，直接/平台 Node、Pod、Deployment、Event 计数一致；ID 排序、limit、401/400、只读 RBAC、4003ms `timed_out`、恢复、`unavailable` 局部隔离及八项清理断言全部通过 |
| M20 Phase 2 最终质量门禁 | `.artifacts/verification/verify-20260727-194724.json` | 通过，223.18 秒；Go vet/全包测试/server build、13 个 Vitest 文件/57 个用例、前端生产构建、三服务 healthy、Kustomize 16/5/22/3 与运行态 HTTP 检查通过 |
| M20 Phase 3 全局搜索门禁 | `.artifacts/verification/verify-20260727-210308.json` | 通过，168.62 秒；固定四类搜索、覆盖率/OpenAPI/交付合同、140 个 Go 测试入口、14 个 Vitest 文件/58 个用例、前端生产构建、三服务 healthy、Kustomize 16/5/22/3 与运行态 HTTP 检查通过 |
| M20 Phase 3 search browser | `docs/changes/2026-07-27-bounded-global-resource-search.md` | 通过；真实 kind 返回 Pod/Deployment/Service/Ingress 四类 `nginx` 匹配，过期 peer 局部失败，Pod 深链精确打开；1280x720 文档 1265/1265，390x844 文档 375/375 且 760px 表格仅在 279px 容器内滚动，无 warning/error 日志 |
| M20 Phase 4 saved filters runtime/browser | `docs/changes/2026-07-27-user-owned-global-search-filters.md` | PostgreSQL/API 通过：并发 22 次创建严格为 20 成功/2 冲突并清零；浏览器创建、重命名、覆盖、应用与 URL 联动通过，桌面 1265/1265、移动 375/375，760px 结果表仅在 279px 容器内滚动且无 warning/error；原生删除确认因浏览器控制超时未声明 UI 完整通过，DELETE API 已通过并完成清理 |
| M20 Phase 4 最终质量门禁 | `.artifacts/verification/verify-20260727-222753.json` | 通过，351.1 秒；151 个 Go 测试入口、14 个 Vitest 文件/59 个用例、生产构建、三服务 healthy、Kustomize 16/5/22/3、OpenAPI/交付合同和运行态 HTTP 检查通过 |
| M20 Phase 5 双集群 search E2E | `.artifacts/search-e2e/search-e2e-20260727-225358.json` | 通过；两个独立 Kubernetes v1.34.0 kind 集群与隔离平台运行时，9 条固定四类资源按 cluster/kind/Namespace/name 稳定排序；401/400、默认 2/2 覆盖、`cluster_limit=1`、9→3 全局截断、四类 `TIMEOUT`、恢复、四类 `QUERY_FAILED`、健康 peer 结果、只读 RBAC 与八项清理断言全部通过 |
| M20 Phase 5 最终质量门禁 | `.artifacts/verification/verify-20260727-230204.json` | 通过，158.94 秒；151 个 Go 测试入口、14 个 Vitest 文件/59 个用例、生产构建、三服务 healthy、Kustomize 16/5/22/3、交付合同和运行态 HTTP 检查通过 |
| M20 Phase 6 CI/release 合同 | `backend/internal/deployment/ci_workflows_test.go`、`docs/changes/2026-07-28-versioned-ci-release-pipeline.md` | 通过；3 个 workflow 和 Dependabot/actionlint 配置均可解析，官方 Actions 固定 SHA，PR 只读且无 secrets，手动发布只产包、tag 才可发布，自托管 kind 作业非取消式运行；actionlint 1.7.7 零告警 |
| M20 Phase 6 最终质量门禁 | `.artifacts/verification/verify-20260728-100752.json` | 通过，180.85 秒；152 个 Go 测试入口、14 个 Vitest 文件/59 个用例、生产构建、三服务 healthy、Kustomize 16/5/22/3、CI/release 交付合同和运行态 HTTP 检查通过 |
| M20 Phase 6 首次托管 CI | `https://github.com/guiyi-labs/aiops-platform/actions/runs/30325194933` | 通过；revision `648aea6c94fbc29fbf21d1f799df29880099d454` 的 Backend、Frontend、Manifests、Compose runtime 全部成功，运行态健康、脱敏证据上传和 teardown 通过 |
| M20 Phase 8 PostgreSQL 恢复演练 | `.artifacts/postgres-recovery/postgres-recovery-20260728-131325.json`、`docs/changes/2026-07-28-postgres-backup-restore.md` | 本地与托管 CI `30331048635` 均通过；PostgreSQL 17 源实例应用 15 个迁移并写入身份/RBAC/集群凭据/诊断/审计/筛选器合成数据，custom dump 导出后源实例先销毁，再在全新实例恢复；迁移和表级计数一致、外键异常为 0，容器/匿名卷/临时备份/进程凭据清理通过 |
| M20 Phase 8 托管 CI | `https://github.com/guiyi-labs/aiops-platform/actions/runs/30331048635` | 通过；revision `24ed4af7b74ec85438c0c8cc005f27ecf6e74886` 的 Backend、Frontend、Manifests、PostgreSQL recovery、Compose runtime、脱敏证据上传和 teardown 全部成功 |
| M20 Phase 8 本地质量门禁 | `.artifacts/verification/verify-20260728-125500.json` | 通过，278.81 秒；Go 1.25 容器全包测试与构建、14 个 Vitest 文件/59 个用例、前端生产构建、三服务 healthy、Kustomize 16/5/22/3、运行态 HTTP 和 actionlint 1.7.7 零告警通过 |
| M20 Phase 9 应用密钥再加密 | `.artifacts/credential-reencryption/credential-reencryption-20260728-141330.json`、`.artifacts/verification/verify-20260728-141111.json`、`docs/changes/2026-07-28-credential-key-reencryption.md` | 本地隔离实体验证通过；2 条 v1 凭据 dry-run 保持不变，损坏第二行使整批回滚且首行摘要不变，修复后 2 条转为 v2，v2-only 后端解密成功，五项清理断言通过；288.9 秒完整门禁含 163 个 Go 测试入口、14/59 前端测试、三服务 healthy 与 Kustomize 16/5/22/3 |
| M20 Phase 9 托管 CI | `https://github.com/guiyi-labs/aiops-platform/actions/runs/30334216631` | 通过；revision `151bc7ee848391e37b74d59f489bbe804d9234ff` 的 Backend、Frontend、Manifests、隔离凭据再加密、PostgreSQL 恢复、随机生产配置 Compose、HTTP、脱敏上传与 teardown 全部成功 |
| M20 Phase 10 签名审计归档 | `.artifacts/audit-archive/audit-archive-20260728-154047.json`、`.artifacts/verification/verify-20260728-153059.json`、`docs/changes/2026-07-28-signed-audit-archives.md` | 本地隔离 PostgreSQL 演练通过；2 条合成脱敏审计行按 ID 升序归档并由外部可信公钥验签，3 条候选在 `max-records=2` 时拒绝且无文件，一字节篡改被拒绝，五项清理通过；361.34 秒完整门禁含 167 个 Go 测试入口、三个后端二进制、14/59 前端测试、三服务 healthy 与 Kustomize 16/5/22/3；托管 runs `30338972042`/`30339580960` 暴露并清理 Linux 可移植性边界，最终 run `30340088789` 四个 job、三项数据库演练、Compose/HTTP/上传/teardown 全部通过 |
| M20 Phase 11 OIDC/MFA 就绪门禁 | `.artifacts/identity-readiness/identity-readiness-20260728-165405.json`、`.artifacts/verification/verify-20260728-165939.json`、`docs/changes/2026-07-28-identity-readiness-gate.md` | 本地与托管 CI `30345051371` 通过；离线严格解析 policy/discovery/JWKS，14 项检查覆盖 HTTPS issuer/endpoint、Code + PKCE S256、scope/签名/JWKS、claim/MFA、不可变 subject 绑定、session/logout 与 break-glass；无网络演练拒绝 issuer/PKCE 和 MFA/email-linking 降级且清理完整；300.97 秒本地全量门禁含 171 个 Go 测试入口、四个后端构建目标、14/59 前端测试、三服务 healthy 与 Kustomize 16/5/22/3；Ubuntu 四个 job、脱敏上传和 teardown 全部通过，不声明生产 SSO 已启用 |
| Real kind E2E | `.artifacts/e2e-kind/e2e-kind-20260726-171621.json` | 通过，三规则、处置幂等、RBAC 与默认自动清理均符合预期 |
| 敏感信息扫描 | `docs/changes/2026-07-26-delivery-packaging.md` | 通过，未匹配私钥、长 token、CA payload 或 JWT bearer material |
| 答辩环境冷启动/清理 | `.artifacts/demo/demo-ready-20260726-170602.json` | 通过，从空 kind 环境重建；全清理后数据库三类 QA 行为 0 |
| 答辩截图 | `docs/thesis/screenshots/capture-metadata.json` | 通过，4 张 1440x1000 页面截图已人工检查；待按当前远端 revision 重采集绑定证据 |

## 故障注入数据

`deploy/demo-scenarios` 固化四组工作负载：健康 Nginx、健康 Service 后端、不存在镜像触发的 ImagePullBackOff、启动后退出触发的 CrashLoopBackOff，以及 selector 不匹配的 Service；M13 另加健康 Ingress、32Mi PVC 和只含普通运行配置的 ConfigMap。M17 再加入 scale-to-zero StatefulSet、DaemonSet、ReplicaSet、暂停的 Job/CronJob、HPA、ResourceQuota、PVC-only LimitRange 和只含诱饵测试数据的 immutable Secret。M18 加入缺失 StorageClass 的 Pending PVC、断后端 Ingress 和可注入状态的饱和 HPA；压力 Node 只在 E2E 期间创建。这些新增 fixture 不额外启动 Pod。所有演示对象位于 `aiops-demo` Namespace，可独立清理。

`deploy/diagnosis-e2e` 另行固化合成 NotReady Node 与 2 副本停滞 Deployment，
仅由一次性 M9 kind 门禁使用，不加入答辩三条诊断场景。
