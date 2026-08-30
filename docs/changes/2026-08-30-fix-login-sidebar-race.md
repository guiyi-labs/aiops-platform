# fix-login-sidebar-race：修复首次登录后侧栏导航失效

- Date: 2026-08-30
- Status: Complete
- Scope: 修复 auth store 在登录成功后未标记 initialized，导致路由守卫在登录跳转时发起多余的 refreshSession 请求竞态清除认证状态

## Context

用户报告"首次登录后点击左侧任务栏不会跳转"。排查发现根因在 `frontend/src/stores/auth.ts` 的 `login()` 函数：登录成功后设置了 `accessToken` 和 `user`，但未将 `initialized` 标记为 `true`。

当 `LoginView` 执行 `router.replace(redirect)` 后，路由守卫 `beforeEach` 调用 `await auth.restore()`，由于 `initialized` 为 `false`，`restore()` 会发起一次多余的 `refreshSession()` HTTP 请求。若该请求失败（服务端 session 尚未传播、cookie 时序问题等），catch 块会清空 `accessToken` 和 `user`，导致 `isAuthenticated` 变为 `false`，守卫随即重定向回登录页或侧栏导航状态异常。

## What Changed

### 前端认证 store
- `frontend/src/stores/auth.ts`：在 `login()` 函数成功调用 `applySession()` 后，立即设置 `initialized.value = true`，阻止后续路由守卫触发不必要的 `restore() → refreshSession()` 调用链。

## Verification

- `cd frontend && pnpm typecheck`：通过（vue-tsc 无错误）
- `cd frontend && pnpm build`：通过（vite build 成功，1815 modules）
- `git diff -- frontend/src/stores/auth.ts`：仅新增 4 行（含注释），无其他文件变动

## Risks / Notes

- 改动极小（1 个文件、4 行），不影响未登录用户的鉴权语义（`restore()` 仍会在页面刷新时正常工作，因为 store 重建后 `initialized` 回到 `false`）。
- 若后续需要在登录后主动刷新 session（如多 tab 同步），可考虑在 `login()` 中不设置 `initialized` 并改为其他机制，但当前场景下这是正确的修复。
