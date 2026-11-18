# 诊断控制台功能精简

- Date: 2026-11-18
- Status: Complete
- Scope: 将本地应用收敛到诊断与案例复用，移除综合运维的界面与运行接线。

## Context

精简前代码已推送至 GitHub，并由 `baseline-pre-slim-20261118` 标记（`cf8998b`）。
配套材料仓库同名标签为 `6b9fbd5`。依据当前范围约定，只做单集群范围内的诊断，不执行集群修复。

## What Changed

- Web 导航收敛为系统概览、集群与资源、故障诊断、历史案例、系统设置。视图文件从 38 个减至 16 个（删除 26 个、增加 4 个）；二级标签与详情页包含在 16 个文件中，不等同于一级模块数。
- 后端主进程只装配诊断、指标、信号、关联、拓扑、案例、模型配置和基础认证审计。
- 移除发布、修复、治理、联邦、协作、巡检、通知等扩展服务的生产初始化、HTTP 注册与后台任务；清理 10 个不再使用的主进程适配器及对应测试文件。
- 历史案例通过已有仓库和混合检索管线提供列表、检索、来源诊断跳转；查询最长 500 字符，补充规则更名后仍可召回旧案例的接口测试。
- 保留 CLI、MCP、基准评测与历史数据库迁移；未按包名盲删诊断依赖。
- 三个 UI 检查脚本使用统一的现行路由目录；截图基线缩至 12 页 × 2 视口。
- 浏览器检查更新为当前模块，移除仅服务于已删模块的用例，新增案例检索/来源链接检查。Playwright 可通过 `PLAYWRIGHT_CHANNEL=chrome` 使用本机 Chrome。
- 本地真实会话检查发现账号会话列表文字对比度不足，改用现有的次级文字 token；截图和 axe 脚本增加临时 profile 清理重试。

## Verification

- `go test ./...`：在闲置适配器清理后全量通过；Linux arm64 编译通过。
- `go vet ./...`：在最终后端版本通过。
- `pnpm typecheck`、`pnpm lint`、`pnpm test`（160 项）、`pnpm build`、`pnpm bundle:gate`：通过。
- `PLAYWRIGHT_CHANNEL=chrome pnpm test:e2e`：64 项通过，覆盖桌面与手机，包括主入口、案例查询、证据回放、失效路由和无障碍检查。
- CSS token 检查：通过，replaceable=0。
- 真实登录会话调用：诊断、案例列表/混合检索、关联规则均为 200；事故、自动化、fleet、SLO、通知及巡检为 404。
- 真实 API 的 axe 审计：初轮除账号页外 11 页 × 2 视口均无 serious/critical/app errors；账号页的已发现对比度问题修复后单独复查通过（两个视口均为 0 serious/critical/app errors）。
- 本地截图和完整检查日志位于 `docs/ui-baselines/` 与 `.artifacts/console-slim/`。截图使用真实数据，未宣称像素比较门禁已通过。

## Risks / Notes

- 精简仅在本地 独立精简分支进行，不推送精简结果。
- 数据库不清空、不删除历史业务数据；旧平台源代码可通过精简前标签恢复。
- 本地 Compose 前后端已重建并启动，地址 `http://localhost:18080`；数据库容器未重建。
- 常规 Docker 构建受本机失效代理影响（Docker Hub 代理连接拒绝），本次使用主机编译产物和已有运行镜像在断网构建模式下更新；标准 Dockerfile 保持源码构建路径。
- 旧运行镜像另打 `k8s-aiops-backend:pre-slim-20261118` / `k8s-aiops-frontend:pre-slim-20261118` 标签。
- 数据库运行前快照仅本地保存于 `.artifacts/console-slim/database-before-local-rebuild.dump`，不会推送凭据或数据库内容到 GitHub。
- 本轮完成入口合并和生产接线精简；后端库、全量历史 OpenAPI 和数据库迁移仍包含原版扩展模块。未强制限制数据库只能登记一个集群，当前流程针对选定集群执行，未实现跨集群聚合。
- 配套材料仓库在备份后未由本次精简流程修改，用户后续编辑不纳入本轮代码提交。
- 按用户要求，精简提交及阶段标签只保留本地，不推送 GitHub。
