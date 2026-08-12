# M102 兼容范围（Compatibility）

- Date: 2026-08-12
- Status: RC 基线声明

## 1. 平台与工具链

| 项 | 兼容版本 | 说明 |
|---|---|---|
| Go | 1.26.x（go.mod `go 1.26.0`） | 后端编译/测试 |
| Node.js / pnpm | Node 22 / pnpm 11.7.0（`frontend/package.json` packageManager） | 前端构建（CI 镜像内） |
| Kubernetes API | client-go `v0.36.3`（k8s.io/apimachinery/client-go） | 目标 k8s 1.36 契约；对旧版本仅承诺已实现资源路径的兼容读取 |
| PostgreSQL | 17 + pgvector 0.8.1（`pgvector/pgvector:0.8.1-pg17`） | 数据库与向量扩展；迁移在启动时自动执行（`backend/migrations/`） |
| 前端框架 | Vue 3 + Vite + TypeScript | 浏览器双视口：Desktop Chrome + Pixel 7（Playwright 配置） |

## 2. 部署形态

| 形态 | 位置 | 状态 |
|---|---|---|
| Docker Compose | `compose.yaml`（本地测试栈） | 支持（含 `scripts/dual-env-compose-drill.sh` 隔离多环境） |
| Kubernetes 清单 | `deploy/kubernetes/`（`kubectl kustomize` 离线渲染门禁） | 支持 |
| Helm 3 | `deploy/helm/aiops-platform/`（Chart.yaml、values.schema.json、templates/） | 支持（组织环境生命周期演练待授权） |
| kind 真实集群 e2e | `deploy/kind/` + `scripts/e2e-*.ps1` | CI `real-kind-e2e.yml` |
| 离线包 / OCI 资产 | M97 release（Helm/Kustomize/离线包、SHA256SUMS、SBOM、provenance、签名） | `v0.3.0-rc.4` 发布；本地演练产物 `v0.3.0-rc.5-replay` 离线包（`.artifacts/offline-install-drill/bundle/`） |
| 镜像架构 | 双架构（amd64/arm64）release 资产；本地离线镜像 arm64 | M97 |

## 3. 环境变量与配置面（`compose.yaml` / 后端 env）

- 基础：`APP_ENV`、`HTTP_ADDR`、`DATABASE_URL`、`SHUTDOWN_TIMEOUT`。
- 认证：`JWT_SIGNING_KEY`、`ACCESS_TOKEN_TTL`（15m）、`REFRESH_TOKEN_TTL`（168h）、`BOOTSTRAP_ADMIN_USERNAME/PASSWORD`、`CREDENTIAL_ENCRYPTION_KEY/KEY_VERSION/DECRYPTION_KEYS`。
- 集群：`CLUSTER_PROBE_TIMEOUT`。
- 指标历史：`METRICS_HISTORY_ENABLED/RETENTION`、`METRICS_COLLECTION_INTERVAL/TIMEOUT`、`METRICS_CLEANUP_INTERVAL`、`METRICS_MAX_CLUSTERS`（20）、`METRICS_MAX_CONCURRENCY`（4）。
- AI（可选增强）：`AI_ENABLED`（默认 false）、`AI_BASE_URL`、`AI_API_KEY`、`AI_MODEL`、`AI_REQUEST_TIMEOUT`、`AI_DAILY_TOKEN_BUDGET`、`AI_MAX_CONCURRENT_REQUESTS`、`AI_MAX_OUTPUT_TOKENS`。
- 通知：`NOTIFICATION_ENABLED`（默认 false）、`NOTIFICATION_WEBHOOK_URL/SECRET` 等。
- 端口约定：开发栈 15432/8080/18080；双环境演练 A=25432/28080/28081、B=26432/29080/29081、恢复=27432/27080/27081；演示演练 21432/21080/21081 + mock 21443。

## 4. 数据与恢复兼容

- 逻辑备份：`pg_dump`（M20 Phase 8；`scripts/e2e-postgres-backup-restore.ps1`、`dual-env-compose-drill.sh` `APP_BACKUP_RESTORE=1`）。
- WAL/PITR/备库：`scripts/wal-pitr-drill.sh`（PG17 archive + `pg_promote()`）。
- 迁移：启动时自动执行 `backend/migrations/000001_init_schema.up.sql` 与后续迁移；全新环境含 initdb 预种子，恢复环境使用空库 + `pg_dump` 还原。

## 5. 明确不兼容/未支持

- 非 HTTPS 的 kubeconfig 服务器（`ParseKubeconfig` 强制 HTTPS）。
- 除 PostgreSQL 外的数据库后端。
- 在无组织授权前提下的 GA 宣称（M89/M90/M102 Gate D 未满足时版本保持 RC）。
- 直接连接 Docker Hub 拉取镜像的离线环境（需本地已有镜像或离线包）。
