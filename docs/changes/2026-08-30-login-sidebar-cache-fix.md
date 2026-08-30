# login-sidebar-cache-fix：修复登录后侧边栏"点不动"经缓存复活

- Date: 2026-08-30
- Status: Complete
- Scope: 修正前端 nginx 缓存策略，使"侧边栏登录后不跳转"的修复在重新部署后不再被浏览器缓存的旧 bundle 复活。

## Context

用户在 `http://127.0.0.1:18080/login` 登录后，左侧任务栏点击无法跳转。
排查结论：

1. "侧边栏不跳转"的根因已在 `3cb752f`（fix(frontend): sidebar not navigating after login）修复——`router.beforeEach` 原本在**每次**导航都 `await auth.restore()`（HTTP refreshSession），修复后改为 `if (!auth.initialized)` 才恢复会话，登录后的侧边栏点击变为纯内存态同步读取，不再阻塞。该修复已包含在当前源码与 18080 实际部署的构建产物中（`index-C33s3cFn.js` 内容哈希一致，证明线上代码已含修复）。
2. 真正导致"修好了又坏"的是 `frontend/nginx.conf` **没有任何缓存控制头**。浏览器对 `index.html`（HTML shell）做了启发式长缓存且不主动 revalidate，于是重新部署后用户浏览器仍加载旧的 `index.html` + 旧哈希 JS，修复前的旧 bundle 持续复活，表现为侧边栏点击无反应。

因此本次改动只动 `nginx.conf` 的缓存头（不碰业务代码），从源头杜绝旧 shell 复用。

## What Changed

### frontend/nginx.conf
- 新增 `location /assets/`：对内容寻址的哈希资源（文件名随内容变化）下发 `Cache-Control: public, max-age=31536000, immutable`，可安全长效缓存。
- 修改 `location /`：对 HTML shell 下发 `Cache-Control: no-cache, no-store, must-revalidate`，浏览器永不从缓存提供旧 shell，每次都重新校验。

## Verification

- 容器内 `nginx -t` 语法校验通过；`nginx -s reload` 平滑重载成功。
- 响应头实测：
  - `GET /` → `Cache-Control: no-cache, no-store, must-revalidate`（Content-Type: text/html）
  - `GET /assets/index-C33s3cFn.js` → `Cache-Control: public, max-age=31536000, immutable`
- 浏览器端到端（全新会话 + 真实 UI 登录 + 真实鼠标点击）验证侧边栏跳转：
  - 真实点击「进入控制台」→ 跳转 `/`（控制台），侧边栏 34 项，无报错。
  - 真实鼠标点击「集群」→ `/clusters`；「事件」→ `/events`（事件中心）；「告警」→ `/event-stream`（事件流与告警）；`window.__errs` 始终为空。
- 证据截图：`/tmp/sidebar-verify.png`（控制台 + 侧边栏导航态）。

## Risks / Notes

- 本改动只改缓存头，不动前端业务代码；与 `3cb752f` / `40818f3` 的路由/认证修复互补——前者修代码、后者修交付。
- 已登录用户若浏览器仍缓存着修复前的旧 `index.html`，需在本次部署后**硬刷新一次**（Cmd/Ctrl+Shift+R）以丢弃旧 shell；此后因 `no-store` 不会再复发。
- 若前端经反向代理/SSH 隧道访问且代理层另有缓存，需在代理侧同步禁用 `index.html` 缓存。
- `frontend/dist` 经 `docker cp` 就地热更新即可（本环境无法拉取 node 基础镜像做 `docker compose build`，采用本地 `pnpm build` + `docker cp` 到 `k8s-aiops-frontend-1` 的 `/usr/share/nginx/html`，再 reload nginx）。本次源码构建产物哈希与线上一致，故 dist 未强制重拷，仅更新了 nginx 配置。
