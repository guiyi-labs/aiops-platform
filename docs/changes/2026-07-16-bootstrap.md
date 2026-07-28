# 2026-07-16 Project Bootstrap

## Scope

- 建立独立工程目录和 Git 忽略规则。
- 建立文档索引、目录规范、开发指南和首个 ADR。
- 创建 Go 后端健康检查基础工程。
- 创建 Vue 3 前端控制台基础工程。
- 创建 PostgreSQL 与应用的 Docker Compose 编排。
- 审计 KubeSphere 的请求过滤链、多集群客户端、Condition、查询和质量门禁设计。
- 增加请求 ID、统一错误响应、请求上下文和统一列表查询模型。

## Verification

- Go 1.24.4：`go test ./...` 通过，`go build ./cmd/server` 通过。
- 后端通过包：`apiquery`、`config`、`httpserver`、`requestctx`。
- Node.js 24.14.0 / pnpm 11.7.0：`pnpm typecheck` 通过。
- Vitest：1 个测试文件、2 个测试通过。
- Vite：生产构建通过，JavaScript gzip 体积约 28.72 kB。
- Docker Hub 在当前环境不可达，改用仓库内临时 Go 工具链完成后端验证；工具链和缓存均由 `.gitignore` 排除。
- Docker Compose 配置校验通过；PostgreSQL 17 在宿主机 `15432` 启动并通过健康检查。
- 初始迁移成功创建 `users`、`roles`、`user_roles`、`clusters`、`cluster_credentials`、`audit_logs`。
- 后端 `http://localhost:8080/api/v1/health/ready` 返回 200 和 `X-Request-ID`。
- 前端 `http://localhost:5173` 返回 200，并通过 Vite 代理访问后端。
- 浏览器验证桌面端 `1280x720` 与移动端 `390x844`，无横向溢出、无控制台错误或警告。

## Deferred

- 用户认证与角色权限。
- kubeconfig 加密和多集群接入。
- Kubernetes 资源查询。
- Event 规则诊断与 AI 分析。
