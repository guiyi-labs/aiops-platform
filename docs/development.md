# Development Guide

## Prerequisites

- Docker Desktop 29+
- Go 1.25+
- Node.js 22+
- pnpm 10+
- kubectl
- kind（进入多集群开发阶段前安装）

只运行整套系统时可仅依赖 Docker Desktop。直接运行前后端时需要本机 Go 和 Node.js。

## Configuration

从模板创建本地配置：

```powershell
Copy-Item .env.example .env
```

`.env`、kubeconfig、私钥和令牌禁止提交。新增配置项时必须同步更新：

1. `.env.example`
2. `backend/internal/config/config.go`
3. 本文档

PostgreSQL 默认映射到宿主机 `15432`，以避开 Windows/Hyper-V 常见的保留端口；容器网络内仍使用 `5432`。

认证相关配置：

| Variable | Purpose | Development default |
|---|---|---|
| `JWT_SIGNING_KEY` | HMAC access token signing key，至少 32 字符 | development-only value |
| `ACCESS_TOKEN_TTL` | 短期访问令牌有效期 | `15m` |
| `REFRESH_TOKEN_TTL` | HttpOnly 刷新会话有效期 | `168h` |
| `BOOTSTRAP_ADMIN_USERNAME` | 空数据库首次启动管理员 | `admin` |
| `BOOTSTRAP_ADMIN_PASSWORD` | 空数据库首次启动管理员口令 | `admin123` |
| `CREDENTIAL_ENCRYPTION_KEY` | Base64 编码的 32 字节 AES 密钥 | development-only value |
| `CREDENTIAL_KEY_VERSION` | 凭据密钥版本标识 | `v1` |
| `CREDENTIAL_DECRYPTION_KEYS` | 最多 8 个旧版本到 Base64 32 字节密钥的 JSON 映射，仅用于过渡解密 | `{}` |
| `CLUSTER_PROBE_TIMEOUT` | Kubernetes API 探测超时 | `10s` |
| `AI_ENABLED` | 是否启用引用式 AI 解释 | `false` |
| `AI_BASE_URL` | Responses-compatible API 基础地址 | `https://api.openai.com/v1` |
| `AI_API_KEY` | Provider 密钥；只来自环境变量 | 空 |
| `AI_MODEL` | 解释模型标识 | `gpt-5.4-mini` |
| `AI_REQUEST_TIMEOUT` | 单次 Provider 请求超时 | `30s` |
| `AI_DAILY_TOKEN_BUDGET` | UTC 自然日内允许提交的输入与输出 token 总量；`0` 表示不限 | `100000` |
| `AI_MAX_CONCURRENT_REQUESTS` | 单进程同时执行的 Provider 请求上限 | `2` |
| `AI_MAX_OUTPUT_TOKENS` | 发送给 Provider 的单次最大输出 token 数 | `1200` |
| `NOTIFICATION_ENABLED` | 是否启用诊断事件 outbox 与 Webhook worker | `false` |
| `NOTIFICATION_WEBHOOK_URL` | 诊断事件接收地址；生产环境必须使用 HTTPS | 空 |
| `NOTIFICATION_WEBHOOK_SECRET` | HMAC-SHA256 签名密钥；启用时至少 32 字符 | 空 |
| `NOTIFICATION_POLL_INTERVAL` | worker 扫描到期投递的间隔 | `5s` |
| `NOTIFICATION_REQUEST_TIMEOUT` | 单次 Webhook 请求超时 | `10s` |
| `NOTIFICATION_RETRY_BASE` | 指数重试的基础延迟 | `10s` |
| `NOTIFICATION_MAX_ATTEMPTS` | 进入 dead 状态前的最大尝试次数，1–20 | `5` |
| `NOTIFICATION_BATCH_SIZE` | 每轮 claim 上限，1–100 | `10` |

生产环境拒绝使用默认管理员口令。访问令牌只保存在前端内存中；刷新令牌由同源 HttpOnly、SameSite=Strict Cookie 保存。

生产环境同样拒绝使用默认凭据加密密钥。生成新密钥示例：

```powershell
$bytes = New-Object byte[] 32
[Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
[Convert]::ToBase64String($bytes)
```

应用主密钥轮换必须先部署“新主密钥 + 旧解密密钥”的重叠配置，执行默认
dry-run，再显式使用 `/app/credential-reencrypt --apply`。完整操作顺序、失败
回滚和旧密钥退役条件见 `docs/database/credential-key-rotation.md`。不要直接替换
主密钥后删除旧密钥，否则历史集群凭据将不可恢复。

审计归档使用生产镜像内的离线 `/app/audit-archive`，不通过 HTTP 暴露。
创建时必须显式提供 ID 范围、1..10000 行上限、新输出路径和 Ed25519 私钥文件；
验签时必须从独立渠道提供 `--trusted-public-key-file`，不能信任归档旁的内嵌
公钥。密钥、payload 和 detached manifest 均不得进入仓库或 CI 产物。完整
操作与失败处理见 `docs/database/audit-archive.md`。

AI 默认关闭。启用远程 Provider 时必须配置 API Key，生产环境的 `AI_BASE_URL` 必须使用 HTTPS。本地 `localhost`/loopback Provider 可以无 Key 运行，便于离线模型和集成测试。API Key 不进入数据库、审计、日志或前端。

生成前会按“估算输入 token + `AI_MAX_OUTPUT_TOKENS`”预留每日预算；Provider 返回后删除预留并按实际 usage 记账。预留带过期时间，进程异常退出不会永久占用预算。每日统计按 UTC 计算，多个后端实例通过 PostgreSQL advisory lock 串行预算检查；并发上限当前是单进程门控，横向扩容时总并发约为实例数乘以配置值。

## Run With Docker Compose

OIDC/MFA is not enabled by runtime environment variables. Before adding a real
provider, complete the offline policy and metadata admission gate described in
`docs/security/identity-readiness.md`. The readiness files must never contain a
client secret, token or private key, and a passing report does not enable SSO.

Production recovery is also not enabled by an application environment variable.
Use `docs/database/recovery-readiness.md` to review explicit RPO/RTO, storage,
PITR, HA, drill and cutover decisions against the newest logical-restore
evidence. A passing report permits implementation work; it does not claim
production PITR, failover or measured objectives.

```powershell
docker compose up --build
```

首次启动会创建 PostgreSQL 数据库并执行 `backend/migrations/000001_init_schema.up.sql`。

## Backend

```powershell
Set-Location backend
go mod download
go test ./...
go run ./cmd/server
```

## Frontend

```powershell
Set-Location frontend
pnpm install
pnpm dev
```

## Verification

提交代码前至少执行：

```powershell
Set-Location backend
gofmt -w .
go test ./...

Set-Location ..\frontend
pnpm typecheck
pnpm test
pnpm build
```

涉及用户工作流时补充 Playwright 测试；涉及 Kubernetes 客户端时补充 fake client 或 kind 集成测试。

## Kubernetes manifest verification

The deployment baseline is rendered without a live cluster:

```powershell
kubectl kustomize deploy/kubernetes
Set-Location backend
go test ./internal/deployment
```

Before applying, copy and edit `deploy/kubernetes/secret.example.yaml` outside
the repository, create the TLS Secret expected by the Ingress, and replace the
development image tags with registry digests or kind-loaded images. A real
`kubectl apply`/kind run is only considered verified when a current context is
available.

## Diagnosis notification Webhook

Notifications are disabled by default. When enabled, keep the signing secret in
the runtime secret store, not in ConfigMap, source control or command history.
The receiver must verify `X-AIOps-Signature` as HMAC-SHA256 over the exact body,
deduplicate by `X-AIOps-Event-ID`, and return a 2xx status only after accepting
the event. Redirects are intentionally rejected. Delivery metadata can be
inspected in the Event Center; stored payloads are never returned by its API.
