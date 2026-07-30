# New Environment Bootstrap And Continuation Runbook

- Updated: 2026-07-30
- Primary validated host: Windows 11, Windows PowerShell 5.1, Docker Desktop Linux containers
- Baseline tag: `baseline-m25-20260730`

本手册用于在一台新的、稳定的开发设备上从零恢复项目。首选“新克隆、新密钥、新容器、
新缓存”，不要复制旧设备的 Docker Desktop WSL 数据、Go module/toolchain 缓存、
`node_modules`、`.tools`、`.env` 或 `.artifacts`。

## Supported Target

### 推荐设备

- Windows 11 23H2 或更新版本，启用虚拟化、WSL2 和长路径。
- 12 个或更多逻辑 CPU、32 GiB RAM、稳定 SSD 上至少 100 GiB 可用空间。
- Docker Desktop WSL2 后端；为 Docker 分配至少 8 CPU、12 GiB RAM 和 60 GiB 磁盘。
- 仓库、Docker 数据和语言缓存放在稳定内置 SSD，不使用易断连的移动盘。

### 最低可用配置

- 8 个逻辑 CPU、16 GiB RAM、60 GiB 可用 SSD。
- 一次只运行一个真实 kind 套件。M24/M25 会同时创建两个单节点 control plane；
  内存不足时不应降低验收语义，而应关闭其他工作负载或换更高配置。

Linux 可以运行 Compose 和普通 Go/Node 门禁，但当前最终 kind 脚本与自托管 runner 的
权威宿主是 Windows PowerShell 5.1。Linux 结果在单独完成兼容验收前不能替代 Windows
真实 kind 证据。

## Toolchain Contract

| 工具 | 要求 | 基线使用 |
|---|---|---|
| Git | 2.44+，支持长路径 | 2.44.0.windows.1 |
| Docker Desktop / Engine | Desktop 4.x，Linux Engine 29+，Compose v2 | Engine 29.6.1 |
| Go | 1.25+；优先使用干净官方安装 | `go 1.25.0` module，容器 `golang:1.25-alpine` |
| Node.js | 22 LTS+ | 前端镜像 22.13.1 |
| pnpm | 必须按 lockfile 使用 11.7.0 | `packageManager: pnpm@11.7.0` |
| kubectl | 与目标 Kubernetes 相差不超过一个 minor | kind 基线 Kubernetes 1.34 |
| kind | 0.30.0 | 一次性真实集群 |
| PowerShell | Windows PowerShell 5.1；可辅以 PowerShell 7 | 验收脚本保持 5.1 兼容 |

安装 kind/kubectl 时从官方发布页下载，并用官方同版本 checksum 校验。可将
`kind.exe` 放入 PATH；脚本也会寻找仓库忽略目录中的 `.tools\kind-v0.30.0.exe`，
但新克隆默认不会包含该文件。

预检：

```powershell
git --version
docker version
docker compose version
go version
node --version
corepack enable
corepack prepare pnpm@11.7.0 --activate
pnpm --version
kubectl version --client
kind version
wsl --status
```

## Do Not Transfer From The Old Device

禁止直接复制：

- `.env`、kubeconfig、ServiceAccount token、SSH/API 私钥和浏览器会话。
- Docker Desktop 的 `docker-desktop` WSL 磁盘、容器层、volume 或 containerd 元数据。
- `.tools/gomodcache`、用户 Go module/toolchain cache、`node_modules`、pnpm store。
- `.artifacts`、`frontend/dist`、临时数据库 dump、验收生成镜像和 kind 集群状态。

旧设备曾发生磁盘闪断并损坏 Docker/containerd 与 Go 自动工具链缓存，因此新设备必须
重新下载和构建。需要保留的长期内容只允许是 Git 已提交源码/文档，以及经过明确批准、
加密和校验的数据库备份。

## Fresh Clone

```powershell
New-Item -ItemType Directory -Path D:\workspace -Force | Out-Null
Set-Location D:\workspace
git clone https://github.com/guiyi-labs/aiops-platform.git
Set-Location .\aiops-platform
git fetch --tags --prune origin
git switch main
git pull --ff-only
git status --short --branch
git log -3 --oneline --decorate
git rev-parse 'baseline-m25-20260730^{}'
```

预期 baseline tag 解引用为：

```text
62320fcac3bbb50b33b7cd6945495264b04b026c
```

如只复现基线可创建临时分支：

```powershell
git switch --create reproduce-m25 baseline-m25-20260730
```

正常开发不要停留在 detached HEAD，应返回最新 `main`。

## Fresh Local Configuration

```powershell
Copy-Item .env.example .env
```

在密码管理器中为新设备生成并写入 `.env`：

- `POSTGRES_PASSWORD`：随机 24+ 字符；同步更新 `DATABASE_URL` 中的密码。
- `JWT_SIGNING_KEY`：随机 32+ 字节，不复用旧设备值。
- `BOOTSTRAP_ADMIN_PASSWORD`：随机 20+ 字符。
- `CREDENTIAL_ENCRYPTION_KEY`：32 个随机字节的 Base64。
- `AI_API_KEY`、Webhook、OIDC 和云凭据默认留空；本地基线保持 `AI_ENABLED=false`、
  `NOTIFICATION_ENABLED=false`。

可用 PowerShell 生成 Base64 32 字节密钥；只直接写入密码管理器和 `.env`，不要发送到
聊天、Git、截图或命令输出归档：

```powershell
$credentialBytes = New-Object byte[] 32
[Security.Cryptography.RandomNumberGenerator]::Fill($credentialBytes)
[Convert]::ToBase64String($credentialBytes)
```

确认 `.env` 被忽略：

```powershell
git check-ignore -v .env
git status --short
docker compose config --quiet
```

## Clean Compose Deployment

```powershell
docker compose pull postgres
docker compose up -d --build
docker compose ps
Invoke-RestMethod http://127.0.0.1:8080/api/v1/health/ready
Invoke-RestMethod http://127.0.0.1:18080/api/v1/health/ready
```

访问：

- 控制台：`http://127.0.0.1:18080`
- 后端 readiness：`http://127.0.0.1:8080/api/v1/health/ready`
- PostgreSQL 宿主机端口：默认 `15432`

核对 migration：

```powershell
docker compose exec -T postgres psql -U aiops -d aiops -Atc `
  "SELECT version FROM schema_migrations ORDER BY applied_at DESC LIMIT 5"
```

新数据库最新应包含 `000019_cross_cluster_promotion.up.sql`；应用进程启动时会按文件名
稳定顺序应用未执行 migration。

## Dependency Bootstrap

主流程可以只依赖 Docker。需要本机快速反馈时再安装依赖：

```powershell
Set-Location backend
$env:GOTOOLCHAIN = 'local'
go mod download
go test ./...

Set-Location ..\frontend
pnpm install --frozen-lockfile
pnpm typecheck
pnpm test -- --run
Set-Location ..
```

`GOTOOLCHAIN=local` 用于避免自动下载工具链缓存损坏时劫持构建；本机 Go 必须满足
`go.mod`。若需要自动工具链，应在稳定磁盘使用全新 cache，不复制旧 cache。

## Acceptance Ladder

必须按顺序升级验证，前一层失败时不要继续制造更多环境状态：

### L0 — 静态与运行时预检

- `git status` 干净；远端和标签正确。
- Docker Engine/Compose 可响应；端口 8080/18080/15432 未冲突。
- `.env` 被忽略且 `docker compose config --quiet` 通过。

### L1 — 快速反馈

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-fast.ps1
```

通过标准：Go vet/test、前端 typecheck/Vitest、Compose/Kustomize 合同全绿。

### L2 — 完整本地门禁

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1
```

通过标准：Go vet/test/build、前端生产构建、当前镜像重建、三服务 healthy、Kustomize
和直连/代理 readiness 全绿，并生成 `.artifacts/verification/*.json`。

### L3 — 基线真实 kind（新机首次接管）

先设置当前新 `.env` 中的管理员密码，只在进程环境保留：

```powershell
$env:AIOPS_ADMIN_PASSWORD = '<new-local-admin-password>'
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m23-release-lifecycle-kind.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m24-cross-cluster-promotion-kind.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m25-workload-protection-kind.ps1
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\e2e-m21-history-kind.ps1
Remove-Item Env:AIOPS_ADMIN_PASSWORD
```

四个套件串行执行。M21 最慢且会验证真实 Metrics API 中断/恢复和后端重启；不要以
`-SkipBackendRestart` 的结果代替最终验收。

### L4 — 远端门禁

推送后等待 CI 的统一 `CI result` 作业成功。普通本地 kind 通过不能替代 Hosted
Backend/Frontend/Manifests/Compose 与安全/恢复演练。

## Optional State Transfer

推荐使用新数据库。只有用户明确要求保留旧开发数据时才迁移：

1. 在旧设备停写并记录 HEAD、migration 列表、数据库摘要和凭据 key version。
2. 用 `pg_dump` 生成逻辑备份，立即加密并计算 SHA-256；不要复制 Docker volume。
3. 通过独立安全通道传输数据库备份和所需旧解密 key，二者不要放在同一归档。
4. 在隔离的新 PostgreSQL 恢复，验证 migration、行数、外键、加密凭据可解密和审计摘要。
5. 运行 `scripts/e2e-postgres-backup-restore.ps1` 作为恢复机制回归；它的合成数据证据
   不能替代真实数据迁移核对。
6. 新环境稳定后按 `docs/database/credential-key-rotation.md` 轮换应用凭据主密钥。

未明确批准时，不迁移旧数据库、旧管理员密码、历史 token 或 retained kind 集群。

## Known Recovery Procedures

### Docker API 500 或 Engine 一直等待 `_ping`

先保存工作并做只读检查；不要删除 Docker 数据目录。Windows 可依次：

```powershell
& "$env:ProgramFiles\Docker\Docker\DockerCli.exe" -Shutdown
wsl --shutdown
Start-Process "$env:ProgramFiles\Docker\Docker\Docker Desktop.exe" -WindowStyle Hidden
docker version
docker compose ps
```

若仍失败，收集 Docker Desktop 日志并停止验收；不要通过删除 volume 掩盖问题。

### Go 报 `package unicode/... is not in std`

这是工具链 cache 损坏，不是业务代码错误。停止使用对应 cache，将精确损坏目录重命名
留作审计，在稳定网络/磁盘重新下载；或用满足版本的本机 Go + `GOTOOLCHAIN=local`，
也可依赖 `golang:1.25-alpine` 容器。不要把旧设备 cache 带到新机。

### PowerShell 阻止脚本执行

不要修改系统永久策略，使用一次性进程策略：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify-fast.ps1
```

### kind 残留

先运行 `kind get clusters`，仅删除本次脚本唯一前缀的集群。不得批量删除 `aiops-test`
或名称不匹配的用户集群。平台残留注册也要按 run ID 精确核对后再清理。

## New Device Handoff Evidence

新设备首次接管完成后，新增一份 `docs/changes/YYYY-MM-DD-new-environment-acceptance.md`，
至少记录：

- 设备/OS/工具版本和资源配置（不含序列号、用户名、IP、密钥）。
- 克隆 HEAD、baseline tag 和远端 CI URL。
- L1/L2 耗时与脱敏证据路径。
- L3 每个套件结果、清理断言和 `aiops-test` 保留情况。
- Docker/Go/cache 异常及恢复动作。
- 本地数据库是全新还是经批准迁移；若迁移，记录校验摘要而非数据内容。
- 下一里程碑、负责人、非目标和未决外部审批。
