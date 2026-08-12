# M102 已知限制（Known Limitations）

- Date: 2026-08-12
- Status: RC 基线（GA 前需逐项关闭或给出在期处置计划）

## 1. 身份与数据韧性（组织授权轨，Deferred）

| 限制 | 现状 | 影响 | 处置 |
|---|---|---|---|
| 生产 OIDC/MFA | 仅有本地预研（`scripts/oidc-login-drill.sh` 14/14）与就绪检查；真实 Provider 验收未授权 | 不能宣称生产身份完成；新登录仅本地账号 + 就绪准入 | M89 授权验收后关闭；未完成前只发布 RC |
| 生产 WAL/PITR/HA | 本地 WAL/PITR 演练 8/8 通过；真实基础设施/跨主机 HA/ENOSPC 未授权 | RPO/RTO 仅本地观测值（RPO≤2s、RTO≈1.2–2.7s），非生产声明 | M90 授权演练后以实测为准；逻辑备份继续作为独立防线 |
| 真实组织 kind/Helm 生命周期 | CI 单套 kind 生命周期（M97）+ 本地双环境 compose 演练（16/16） | 组织环境的安装/升级/回滚一致性未在组织侧复验 | M102 Gate D 前在授权环境执行两次独立全新环境演练 |

## 2. 演练工具边界

- `backend/cmd/demo-kube-mock` 只模拟演示旅程涉及的最小 Kubernetes API 面（nodes/pods/deployments/events/replicasets/metrics/`/version`）；不含 admission、watch、分批滚动状态等完整行为，**不得用于承载真实工作负载的验证**。
- `backend/cmd/oidc-provider` 是本地演练 IdP，仅驱动平台真实登录链路，**绝不可用于生产**。
- `k8s-aiops-backend:v0.3.0-rc.5-local` 等本地离线构建镜像为演练产物，不入库、不作为发布资产。

## 3. 本地环境约束（非产品限制）

- Docker（colima）2GiB/2CPU：BuildKit 并行构建会 OOM，本地镜像构建需宿主机 `GOOS=linux GOARCH=arm64` 预编译 + 小镜像封包。
- 无 kind/helm/kubectl/pwsh：真实集群 e2e 依赖 CI（`real-kind-e2e.yml`）或授权环境。
- Docker Hub 不可达：本地只能使用已有镜像（`k8s-aiops-backend:latest`、`k8s-aiops-frontend:latest`、`pgvector/pgvector:0.8.1-pg17`）。
- macOS Go 忽略 `SSL_CERT_FILE`：OIDC 演练需临时 Keychain 信任（`security add-trusted-cert` + 退出清理）。

## 4. 功能/契约限制（产品决策）

- AI 仅作解释增强：引用缺失/过期/不一致时显式降级，不生成无来源结论；`AI_ENABLED=false` 时相关路由不注册（`router.go` 条件注册）。
- 受控操作目录固定：当前 catalog 为 `deployment.rollout_restart / scale / image_update / rollback`、`cronjob.suspend / resume`（`internal/remediation/model.go`）；不新增任意命令、Pod exec、WebShell。
- 集群资源通过 API Server 实时查询，不全量写入平台数据库；无网络时相关视图降级或不可用。
- 高权限写操作要求 `system_admin`/`system_ops_admin` 角色 + 人工确认 + 审计（幂等 + confirmation token）。
- 前端规模性能以虚拟滚动与固定渲染预算控制；超大 fleet（>500 节点 / >50k Pod）为报告模式预算，未设 fail-closed 阈值。

## 5. 已知文档/证据缺口（低风险）

- M1–M20 以主题文档归档（无独立 mXX 编号文件）。
- 本地演练证据（`.artifacts/`）已 gitignore，不随仓库分发；对外复验需在 CI/组织环境重新生成。
- 覆盖率 60.03% 为本地报告值，远端以 CI 门禁为准。
