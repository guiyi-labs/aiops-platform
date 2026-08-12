# M100-B：会话撤销 / 密码 / auth_version 失效旅程复验与缺口修复

- Date: 2026-08-12
- Status: Complete
- Scope: M100 第二步——按路线“会话撤销、密码变更、auth_version 失效旅程复验”全面核对安全相关变更（禁用、角色变更、本人改密、管理员重置）对存量 access token / refresh session 的失效契约，修复失效不一致缺口，并以单测 + 运行时冒烟固化旅程。

## Context

M100 要求“未授权读写路径的契约测试覆盖率达到 100%”。M100-A 已确认 `ResetPassword`/`ChangePassword` 都会 bump `auth_version` 并撤销全部 refresh session，但 `UpdateUser` 的 `securityChanged` 分支（禁用用户、角色变更）只撤销 refresh session、**没有 bump `auth_version`**——存量 access token 在禁用/角色变更后仍可继续使用（仅角色变更场景：token 里的旧角色仍被中间件信任），与改密/重置的失效强度不一致。

## What Changed

### 失效契约补齐（backend/internal/auth/repository.go）

- `UpdateUser` 的 `securityChanged` 分支新增 `auth_version = auth_version + 1`，与 `ResetPassword`/`ChangePassword` 对齐：禁用或角色变更后，所有存量 access token 立即以 `ErrInvalidAccessToken` 拒绝，全部 refresh session 撤销，用户必须重新认证。
- 契约语义：`securityChanged` = 置为 disabled 或角色集合变更（与既有判定一致；重新启用不视为 security change——禁用时已 bump + 撤销，启用后的旧 token 本就因版本失配失效）。

### 旅程测试（backend/internal/auth/service_test.go）

- `repositoryStub` 扩展为建模仓库失效契约：`UpdateUser`（禁用/角色变更）、`ResetPassword`、`ChangePassword` 均 bump `AuthVersion` 并清空 refresh sessions。
- 新增 4 个旅程测试：
  - `TestServiceSecurityJourneyDisableRevokesSessionsAndRejectsTokens`：登录发 token → 管理员禁用 → `Authenticate` 返回 `ErrUserDisabled`、refresh sessions 清空、禁用后登录返回 `ErrUserDisabled`。
  - `TestServiceSecurityJourneyRoleChangeInvalidatesOutstandingAccessTokens`：角色变更 → `auth_version` 1→2 → 旧 access token 以 `ErrInvalidAccessToken` 拒绝、sessions 清空。
  - `TestServiceSecurityJourneyPasswordChangeRevokesRefreshAndRejectsOldPassword`：本人改密 → 旧 access token 401、旧 refresh 失效、旧密码登录 `ErrInvalidCredentials`、新密码登录成功。
  - `TestServiceSecurityJourneyAdminResetBumpsAuthVersionAndRevokesSessions`：管理员重置 → `auth_version` bump、旧 token 拒绝、旧密码失效、新密码可登录。

## Verification

- Backend 门禁：`go test ./...`（auth 包新增 4 个旅程测试）、`go vet ./...`、`golangci-lint run ./...` 全绿（0 issues）。
- 运行时冒烟（compose 栈 healthy，zsh 脚本，新建 smoke 用户完整旅程 14 项全部符合预期）：
  - 登录 → `/me` 200；本人改密 204 → 旧 access token 401、旧 refresh cookie 401、旧密码登录 401；新密码登录 `/me` 200。
  - 管理员重置 200 → 旧 access token 401、旧密码登录 401；重置密码登录 `/me` 200。
  - 管理员禁用 200 → 存量 access token `/me` 403 `USER_DISABLED`、禁用后登录 403。
  - 冒烟中发现并规避了 macOS bash 3.2 嵌套引号导致 curl `-d` 截断的脚本缺陷（改用 zsh 执行，非后端问题）。
  - 所有 smoke/dbg 测试用户已禁用清理。

## Risks / Notes

- 重新启用（disabled → active）不额外 bump `auth_version`：禁用时已 bump 并撤销全部会话，启用后用户需重新登录，旧 token 因版本失配失效，无回退窗口。
- `auth_version` 为全用户级版本号：任何安全变更使该用户全部设备 token 失效，与当前会话管理模型一致（未做按会话粒度 access token 撤销）。
- 下一步：M100-C（敏感字段静态扫描与日志脱敏门禁）、M100-D（依赖/SBOM 差异门禁）。
