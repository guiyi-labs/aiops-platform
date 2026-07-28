# M9 Node/Deployment 真实 kind 端到端验证

- 日期：2026-07-26
- 里程碑：M9
- 状态：Accepted
- 范围：独立 kind 夹具、只读目标集群凭据、诊断证据、RBAC 边界、清理契约和完整质量门禁

## 目标

补齐 M8 两条新规则的真实 Kubernetes API 验证，确认 Node Condition 与
Deployment status 能经由短期只读凭据进入平台、命中规则并持久化为可追溯证据。
本阶段不修改 `deploy/demo-scenarios`、`scripts/e2e-kind.ps1` 或答辩环境的
准备/清理脚本。

## 实现

1. 新增 `deploy/diagnosis-e2e` 独立 Kustomize 夹具：
   - `synthetic-not-ready` 为不可调度的合成 Node，状态子资源由测试脚本写入
     `Ready=False / SyntheticKubeletUnavailable`；
   - `stalled-deployment` 固定期望 2 个副本，并使用不会匹配任何 Node 的
     `nodeSelector`，由真实 Deployment controller 形成 `2/0/0/2` 状态；
   - 夹具与 `aiops-demo` Namespace 和答辩三场景完全隔离。
2. 新增 `scripts/e2e-diagnosis-kind.ps1`：
   - 使用仓库内 `kind v0.30.0` 创建时间戳命名的一次性集群；
   - 只应用 `deploy/managed-cluster/observer.yaml`，不应用处置 Role；
   - 创建 30 分钟 ServiceAccount token，只在内存 kubeconfig 中交给平台；
   - 从平台资源 API 读取 Node/Deployment，执行诊断并回读持久化详情；
   - 在 `finally` 删除平台集群、级联诊断记录、kind 集群、临时 kubeconfig
     与非敏感 Node 状态补丁文件；
   - 证据 JSON 不包含密码、token、CA、Cookie 或 kubeconfig。
3. 新增夹具合同测试与交付资产合同，将 `deploy/diagnosis-e2e` 纳入
   `scripts/verify.ps1` 的 Kustomize 门禁。

## 真实环境结果

端到端运行于：

- kind：`v0.30.0`（Windows/amd64）
- Kubernetes：`v1.34.0`
- 一次性集群：时间戳命名，验证后删除
- 原有集群：`aiops-test`，验证前后均保留

实际断言结果：

| 项目 | 结果 |
|---|---|
| Node 状态 | `Ready=False`，Reason 为 `SyntheticKubeletUnavailable` |
| Deployment 状态 | desired/current/ready/available/unavailable = `2/2/0/0/2` |
| Node 规则 | 命中 `node.not_ready.v1`，持久化 2 条 `node_condition` 证据 |
| Deployment 规则 | 命中 `deployment.replicas_unavailable.v1`，持久化 1 条 `deployment_status` 证据 |
| 只读 RBAC | list Nodes=`yes`，get Deployments=`yes` |
| 写入 RBAC | patch Deployments=`no`，patch Nodes=`no` |
| 清理 | 平台集群已删、该集群诊断记录为 0、kind 集群已删、两个临时文件已删 |

脱敏证据：
`.artifacts/diagnosis-e2e/diagnosis-e2e-20260726-193724.json`。

## 完整质量门禁

`scripts/verify.ps1` 于 2026-07-26 19:42:37 +08:00 通过：

- Go 1.25 Docker 工具链全包 `vet`、`test` 和 server build 通过；
- 前端 typecheck、8 个 Vitest 文件/27 个用例和 Vite 生产构建通过；
- PostgreSQL、Backend、Frontend 三个 Compose 服务均为 healthy；
- Kustomize 渲染资源数为 `16 / 5 / 7 / 3`；
- Backend ready、Frontend HTTP 200、Frontend API proxy ready。

完整证据：`.artifacts/verification/verify-20260726-194237.json`。

## 边界

- 合成 Node 仅用于独立 E2E，不进入答辩演示数据，也不污染真实控制平面 Node。
- 目标集群凭据严格只读；平台没有通过该凭据执行 Node 或 Deployment 写入。
- 本阶段没有增加任意 YAML、任意 Patch、Pod Exec、WebShell 或新的处置动作。
- 答辩三条稳定诊断场景及其准备/清理契约未修改。
