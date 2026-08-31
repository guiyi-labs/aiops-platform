# login-sidebar-navigation：修复登录后侧边栏导航不更新

- Date: 2026-08-31
- Status: Complete
- Scope: 修复首次登录进入控制台后，侧边栏 URL 已变化但页面视图仍停留在总览的问题。

## Context

登录成功后点击左侧菜单，路由地址会变更，但页面组件可能继续显示首次加载的总览视图。
问题由认证状态恢复竞态与控制台壳层中的嵌套路由出口共同放大：首次进入控制台时，页面子组件的
Teleport 可能早于顶部目标节点挂载，更新异常会阻止旧视图被替换。

## What Changed

### 前端路由与壳层
- `frontend/src/App.vue`：保留稳定的 `.route-viewport`，认证路由统一交给控制台壳层处理。
- `frontend/src/components/ConsoleLayout.vue`：从当前 `route.matched` 直接解析页面组件，并以
  `route.fullPath` 作为实例键，确保侧栏导航后实际页面同步切换；为页面操作区的 Teleport 启用
  `defer`，等待壳层目标节点完成挂载。
- `frontend/src/stores/auth.ts`：登出后保持 `initialized` 为 true，避免登录页跳转再次触发
  refresh cookie 恢复并重新进入控制台。

### 回归覆盖
- `frontend/e2e/smoke.spec.ts`：新增真实登录后依次点击“集群”和“事件中心”的桌面/移动端回归场景，
  同时断言 URL 和页面标题。
- `CHANGELOG.md`：在 Unreleased 区段登记用户可见的导航修复。
- `docs/README.md`：增加本变更记录的文档索引入口。
- 交付证据保存在 `.artifacts/sidebar-navigation-fix/`：修改副本、差异、验证报告和回滚脚本。

## Verification

### BASELINE

- Command: `pnpm typecheck`; Input: none; Literal output: `$ vue-tsc -b`; Result: exit 0.
- Command: `pnpm test -- --run`; Input: frontend unit suite; Literal output: `Test Files 29 passed (29)` / `Tests 160 passed (160)`; Result: exit 0.
- Command: `pnpm exec playwright test e2e/smoke.spec.ts --project='Desktop Chrome' --grep='Successful login|Route navigation'`; Input: desktop smoke subset; Literal output: `1 passed, 1 failed`, `Expected: false`, `Received: true` for `gapDetected`; Result: exit 1.

### MODIFIED

- Command: `pnpm typecheck`; Input: none; Literal output: `$ vue-tsc -b`; Result: exit 0.
- Command: `pnpm lint`; Input: frontend source tree; Literal output: `$ eslint .`; Result: exit 0.
- Command: `pnpm test -- --run`; Input: frontend unit suite; Literal output: `Test Files 29 passed (29)` / `Tests 160 passed (160)`; Result: exit 0.
- Command: `pnpm build`; Input: production frontend build; Literal output: `1818 modules transformed`, `✓ built in 2.62s`; Result: exit 0.
- Command: `pnpm exec playwright test e2e/smoke.spec.ts`; Input: Desktop Chrome + Mobile Chrome smoke suite, including `playwright-admin` / `playwright-password` login and menu clicks; Literal output: `28 passed (18.5s)`; Result: exit 0.

### ROLLBACK

- Command: `./.artifacts/sidebar-navigation-fix/ROLLBACK.sh ./.artifacts/sidebar-navigation-fix/rollback-test/App.vue`; Input: a copy of `MODIFIED_FILE`; Literal output: `restored ./.artifacts/sidebar-navigation-fix/rollback-test/App.vue from /Users/zjw/Desktop/codex/aiops-platform/.artifacts/sidebar-navigation-fix/BASELINE_App.vue`; Result: exit 0.
- Restored behavior/status: the rollback copy byte-matches `BASELINE_App.vue`; the working `frontend/src/App.vue` and `MODIFIED_FILE` remain on the changed branch.

## Risks / Notes

- The route component is intentionally keyed by `route.fullPath`, so query/hash changes remount the active page and avoid stale view state.
- The original browser-cache headers fix remains in `frontend/nginx.conf`; this record covers the source-level rendering/auth fix and its regression proof.
- The exact artifact paths and command evidence are also recorded in `.artifacts/sidebar-navigation-fix/VERIFICATION.txt`.
