# fix-login-sidebar-race：修复首次登录后侧栏不会跳转（并发还原竞态）

- Date: 2026-08-30
- Status: Complete
- Scope: 修复 `auth.restore()` 与 `login()` 并发竞态导致的首次登录后侧栏点击不跳转

## Context

用户反馈「首次登录后点击左侧功能栏不会切换」。`router.beforeEach` 在每次导航都会 `await auth.restore()`，
而 `restore()` 原本会直接 `POST /api/v1/auth/refresh` 并在失败时清空 `accessToken/user`。
登录成功动画 `router.replace(redirect)` 后触发的守卫会并发到 `login()` 成功后刚写入的会话，
若刷新会话不存在/失败，`catch { clear }` 会把刚登录成功的 `isAuthenticated` 清回 `false`，
导致守卫重定向回 `/login`，侧栏看起来"点不进去"。

`da04df1` 已在 `login()` 后置 `initialized = true` 阻断后续 `restore()`，但并发窗口仍存在：
如果 `restore()` 的 `refreshSession()` 已经在 inflight，而 `login()` 后置 `initialized` 才到达，
两个异步分支会交叉：前者失败的 `catch` 可能晚于后者的写入到达并清除会话。

## What Changed

### frontend/src/stores/auth.ts
- 新增并发去重：`restorePromise: Promise<void> | null`，重复 `restore()` 调用返回同一 Promise，
  避免每次导航都扇出一次 `refreshSession()`。
- `restore()` 内部：
  1) inflight 期间若 `initialized && accessToken && user`（说明 `login()` 已获胜），则**不**再
     `applySession(refreshResult)` 覆盖 fresher 会话；
  2) 失败路径仅在仍无认证会话时才清空 `accessToken/user`，避免把 `login()` 刚写入的会话擦掉；
  3) `finally { initialized = true; restorePromise = null }` 保证一次性门闩语义。

### Verification
- `pnpm typecheck` 0、`pnpm lint` 0、`pnpm build` 绿（1818 modules）。
- 交互验证：`auth.restore()` 两次并发调用仅触发一次 `refreshSession()`；`login()` 与
  `restore()` 并发时最终 `isAuthenticated === true` 且侧栏可导航。

## Risks / Notes

- 仅改 `frontend/src/stores/auth.ts`，后端不变。
- 不引入持久化存储，仅进程内 Promise 去重与"失败不覆盖已认证会话"守则，符合现有无本地持久化
  约束。
