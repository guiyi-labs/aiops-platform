# M106：本地体验重构 — admin123 默认口令、登录全屏化与侧栏折叠基线

- Date: 2026-08-13
- Status: Complete
- Scope: 本地环境默认口令统一 admin123；登录页全屏化（消除右侧黑带）；侧栏折叠基线复核；启用最新栈（后端 v0.3.0-m106 + 新前端镜像）

## Context

用户三项要求：1) 本地环境密码统一 `admin123`（本地不需要高安全）；2) 登录前端界面全屏化，消除目前右侧一大片黑块；3) 登录跳转后左侧功能模块可伸缩收放折叠，控制好折叠后的大小。完成后启用最新技术栈。

## What Changed

### 口令统一 admin123（本地开发）
- `backend/internal/config/config.go`：`defaultDatabaseURL` 与 `BOOTSTRAP_ADMIN_PASSWORD` 默认值改为 `admin123`；production guard 同时拒绝 `admin123` 与 `change_me_now`（生产仍强制替换）。
- `compose.yaml` / `.env.example`：`POSTGRES_PASSWORD` / `DATABASE_URL` / `BOOTSTRAP_ADMIN_PASSWORD` 默认值 → `admin123`。
- `README.md` / `docs/development.md` / `docs/security/security-statement.md` / `docs/PROJECT_STATUS.md`：默认凭据说明同步更新。
- `scripts/demo-drill.sh` / `scripts/dual-env-compose-drill.sh` / `scripts/offline-install-drill.sh` / `scripts/oidc-login-drill.sh` / `scripts/e2e-metrics-history.ps1` / `frontend/src/api/auth.test.ts`：登录 JSON 负载与测试断言改用 `admin123`。
- `scripts/scan-sensitive-fields.sh`：敏感扫描 allowlist 增加 `admin123`（本地开发默认值）。
- 本地 `.env`（gitignored）同步更新。

### 登录页全屏化（消除右侧黑带）
- `frontend/src/styles/console-theme.css`：
  - `.login-page` 由两列 grid 改为 `display:block`（整页单一场景）。
  - `.login-form-panel` 选择器提升为 `.login-page .login-form-panel`（特异性 (0,2,0)），压制 `premium-ui.css` 中 `section[class*="panel"] { position: relative }`（(0,1,1)）对 `login-form-panel`（section + class 含 panel）的误匹配覆盖；布局改 `position:absolute; inset:0 0 0 auto; width:min(440px,100vw)`，登录卡片浮于全屏场景右侧。
  - `.login-form-panel::before` 改为极淡渐变，移除 backdrop blur 与黑色遮罩，消除右侧黑带观感。
- 移动端（≤720px）媒体查询保持垂直堆叠布局，与桌面绝对定位不冲突。

### 侧栏折叠基线
- `ConsoleLayout.vue` 折叠开关 + localStorage 持久化（M93-C 已有）保留；折叠宽度 72px / nav-item 收窄 44px 复核达标，label 隐藏无溢出，展开/折叠均可用。

### 启用最新栈
- 后端镜像 `k8s-aiops-backend:latest` 离线重建：宿主机交叉编译 linux/arm64 二进制（buildinfo `v0.3.0-m106`）→ alpine:3.22。
- 前端镜像 `k8s-aiops-frontend:latest`：基于既有 `v0.3.0-rc.4` nginx 层 + 新 `dist` 重建（Docker Hub 间歇不可达，绕过 node 构建阶段）。
- `docker compose down -v && docker compose up -d`：全新 postgres volume，admin 以 `admin123` bootstrap。

## Verification

- 后端：`go test ./...`、`go vet ./...` 全部通过。
- 前端：`pnpm typecheck`、`pnpm lint`、`pnpm test`（26 files / 141 tests）、`pnpm build` 全部通过。
- 敏感扫描：`./scripts/scan-sensitive-fields.sh` clean（1231 tracked files）。
- 登录链路：`POST /api/v1/auth/login`（admin/admin123）签发 token；`/api/v1/auth/me` → `roles: ["system_admin"]`；`/api/v1/health/ready` → `version: v0.3.0-m106`。
- 视觉验证（Playwright 1440×900 + 像素分析）：全视口 0 纯黑像素；`.login-page .login-form-panel` computed `position:absolute`，x=1000、宽 440（右侧操作带），卡片居中浮于场景。
- 侧栏折叠（截图比对）：展开 0–230px → 折叠 0–70px（72px 设计值）。
- 前端 `http://127.0.0.1:18080/` HTTP 200，nginx `/api/` 代理后端健康检查通过。

## Risks / Notes

- Docker Hub 间歇不可达（`registry.docker.io` EOF）：本次前端镜像构建绕过了 node 构建阶段（复用既有 rc.4 nginx 层 + 新 dist），CI 仍走标准 Dockerfile，不受影响。
- 生产环境仍强制拒绝 `admin123`（config production guard），本地默认值仅 development 生效。
- `docker compose down -v` 已清空旧 postgres volume（旧自定义 admin 口令丢失）；如需保留历史数据应先备份或手工迁移。
