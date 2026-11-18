# Spatial cluster display

- Date: 2026-11-18
- Status: Complete
- Scope: 在系统概览中增加立体集群场景与独立大屏展示。

## Context

节点归属图适合快速理解集群，展示时需要更清晰的空间层次、状态和资源细节。

## What Changed

- `frontend/src/components/ClusterScene.vue`：立体场景、机柜底座、Pod 单元、归属流线、旋转视角、节点巡览及真实指标详情，支持暂停与减少动态效果设置。
- `frontend/src/views/DashboardView.vue`：接入现有数据，提供立体大屏和综合大屏切换，沿用刷新、退出与键盘操作。
- `docs/console-demo-guide.md`：补充立体场景的展示路径与操作说明。
- 立体机柜按节点 Ready 状态呈现，Pod 单元按已读取资源健康状态着色；选中节点展示匹配的 CPU/内存用量与可分配占比、Pod 样本和证据入口。未知数据保留未知值。
- 场景最多绘制 6 个节点、每节点 18 个 Pod 单元，数量统计仍基于已读取样本。支持拖动与方向键旋转、自动环绕、六秒节点巡览、动效暂停和视角复位；隐藏、离屏及系统减少动态效果时暂停自动变化。
- 桌面立体大屏适配视口高度，保留退出与控制按钮；综合大屏和常规概览仍可使用。返回时恢复触发按钮焦点。
- 对应浏览器用例验证数据切换、缺失值、节点交互与展示模式。

## Verification

- `pnpm typecheck`、`pnpm lint`、161 项单元测试、生产构建、`pnpm bundle:gate`、`pnpm style:audit` 通过；后端 `go test ./...` 通过。
- 完整 Playwright 桌面/移动回归 116 项通过。调整视口高度后再次验证场景与概览 14 项通过，包含节点/指标/集群切换、键盘交互、全屏退出、缺失值、绘制上限、暂停及减少动态效果。
- 本地与演示机实际登录，读取三节点、39 个 Pod 的真实集群。概览及立体大屏在桌面/375px 与明暗主题共 16 次检查，无严重/致命 axe 问题、页面横向溢出或脚本异常。另验证自动环绕、暂停、巡览、选中节点证据链接与 Escape。
- 已重建本地前端；演示机前端镜像 `aiops-demo-frontend:spatial-cluster-20261118`，使用已有 `18080` 入口，后端及数据库保持运行。原镜像与 Compose 配置可用于回退。
- 证据目录：`.artifacts/spatial-cluster-20261118/`，包含 `all-e2e.log`、`browser-tests.log`、`go-test.log`、构建日志、`live-verification.json` 和截图；交付源码及验收摘要同步到演示机。

## Risks / Notes

- 以投影几何呈现立体空间，不增加 WebGL 依赖。连线表达集群归属，不表示实际网络流量或物理机房位置；Pod 以已读取样本为准。
- 仅更新前端，保留既有后端、数据与集群配置。
- 未新增业务模块及付费模型调用；立体场景复用现有只读资源接口。独立动画几何增加约 5KB gzip 的脚本，已通过体积门禁。
