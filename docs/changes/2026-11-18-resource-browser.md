# Resource browser: keep list status visible beside details

- Date: 2026-11-18
- Status: Complete
- Scope: 资源列表与详情并排浏览、连续切换和就地诊断反馈。

## Context

详情遮罩覆盖资源列表，查看状态和切换资源需要先关闭详情。

## What Changed

- `frontend/src/views/WorkloadsView.vue`：将详情改为工作区内的非模态面板，保留关键列表列、选中标记、窄屏切换器及独立详情入口；诊断反馈在当前操作区域展示，隔离过期请求。
- 桌面端并排展示名称、命名空间和状态，其他信息放在详情中；Node 使用其就绪状态列。关闭详情恢复完整表格。窄屏保留可滚动列表与详情切换器，浏览器前进/后退同步当前选择。
- 修正虚拟列表滚动元素绑定，持续使用有限行渲染；修正详情、指标覆盖率与单位文字的主题对比度。独立详情链接只在已支持的类型展示。
- `frontend/e2e/resource-browser.spec.ts`：验证并排状态、鼠标/键盘切换、窄屏、诊断反馈和过期请求。

## Verification

- `pnpm lint`、`pnpm typecheck`、`pnpm test`（161 项）、`pnpm build`、`pnpm bundle:gate`、`pnpm style:audit` 通过；`backend` 的 `go test ./...` 通过。
- Playwright 桌面与窄屏共 108 项通过，包含新增的状态可见、鼠标/键盘/下拉切换、浏览器返回、100 条长列表、Node、诊断等待/未命中/失败/重试及过期响应隔离用例。
- 本地与演示机使用实际账号登录、读取真实集群并切换 Pod。1440/375 宽度与明暗主题共 8 次检查，无严重/致命 axe 问题、页面横向溢出或脚本异常。
- 仅重建并替换本地与演示机前端。演示机镜像 `aiops-demo-frontend:resource-browser-20261118`，入口端口保持 `18080`；前端健康检查通过，后端与 PostgreSQL 保持运行。
- 证据目录：`.artifacts/resource-browser-20261118/`，包含 `all-e2e-final.log`、`browser-tests.log`、`go-test.log`、`build.log`、`live-verification.json` 和真实界面截图。源码与检查摘要同步到演示机部署目录。

## Risks / Notes

- 仅更新前端；保留独立详情路由，演示机更新不涉及数据库或集群工作负载。
- 演示机原前端镜像和 Compose 备份保留，可用 `private/backups/compose-before-resource-browser-20261118.yaml` 恢复前端镜像配置。旧资源链接仍可使用。
