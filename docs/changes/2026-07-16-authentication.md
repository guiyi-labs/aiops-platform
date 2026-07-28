# 2026-07-16 Authentication Baseline

## Scope

- 增加 bcrypt 密码摘要、JWT 短期访问令牌和随机刷新令牌。
- 增加刷新令牌摘要存储、事务轮换、注销撤销和禁用用户拦截。
- 空数据库首次启动时创建系统管理员，并保留四类平台角色。
- 增加登录、刷新、注销和当前用户 API，以及认证与角色中间件。
- 前端增加 Pinia 会话状态、Vue Router 路由守卫、登录页和退出登录。
- 访问令牌仅保存在运行时内存，刷新令牌使用 HttpOnly、SameSite=Strict Cookie。

## Verification

- 项目内 Go 1.24.4 工具链：`go test ./...` 通过，`go build ./cmd/server` 通过。
- 后端认证测试覆盖密码摘要、JWT 声明、登录失败、刷新令牌非明文存储和禁用用户刷新。
- `pnpm typecheck` 通过。
- Vitest：2 个测试文件、4 个测试通过。
- `pnpm build` 通过，登录与控制台视图按路由拆分为独立产物。
- 使用运行中的 PostgreSQL 17 完成真实 API 验证：登录和 `/auth/me` 成功，刷新 Cookie 完成轮换且为 HttpOnly、Path=`/api/v1/auth`，注销返回 204。
- 浏览器验证登录、控制台跳转和退出登录；桌面与 390px 移动端均无横向溢出，控制台无错误或警告。
- `docker compose config --quiet` 通过；Docker Hub OAuth 端点在当前网络不可达，未完成后端容器镜像重建。

## Deferred

- MFA 与企业身份源接入。管理员重置、用户主动改密和会话设备管理已由后续独立阶段完成。
- 登录会话设备列表和用户主动撤销其他会话。

原延后项“用户与角色管理页面”“登录和权限拒绝审计落库”“集群凭据加密与探测”均已在后续阶段完成，详见对应变更记录与 ADR。
