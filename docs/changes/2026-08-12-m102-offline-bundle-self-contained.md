# M102 离线包自包含化：迁移文件入包，安装不再依赖宿主机源码

- Date: 2026-08-12
- Status: Complete
- Scope: 修复离线安装包非自包含缺陷，使 bundle 可在不含仓库源码的另一台机器完成初始化

## Context

既有 `offline-bundle`（基于 `scripts/offline-install-drill.sh`）被定义为"可复用离线安装包"，
但 `compose.offline.yaml` 的 Postgres 初始化挂载指向宿主机绝对路径
`$ROOT/backend/migrations/000001_init_schema.up.sql`，且镜像依赖宿主机预先 `docker load`。
这意味着把 bundle 拷到一台**不含仓库源码、未预加载镜像**的机器上无法完成全新数据库初始化，
与"允许安装/可离线分发"声明相矛盾。本回合将迁移文件纳入 bundle 并以相对路径挂载。

## What Changed

- `scripts/offline-install-drill.sh`：
  - bundle 组装阶段新增 `migrations/` 目录并把 `backend/migrations/000001_init_schema.up.sql`
    复制为 `migrations/000001_init_schema.sql`。
  - `compose.offline.yaml` 的 initdb 挂载由 `$ROOT/...` 改为 bundle 内相对路径
    `../migrations/000001_init_schema.sql`（相对 `deploy/`，指向 bundle 根）。
  - `OFFLINE-SHA256SUMS` 由 `find . -type f` 自动纳入迁移文件（6 → 7 文件）。

## Verification

- `scripts/offline-install-drill.sh`（`v0.3.0-rc.5-replay`）：10/10 PASS，升级后
  bundle 为 7 文件（含 `migrations/000001_init_schema.sql`），完整性校验 OK，
  全新隔离环境（pull_policy=never）安装 -> 关键旅程 -> 数据持久化全部通过；
  报告 `.artifacts/offline-install-drill/report-20260812-230655-98e83c.json`。
- 已发布 bundle `aiops-platform-offline-v0.3.0-rc.5-replay/` 内确认
  `migrations/000001_init_schema.sql` 存在且 compose 引用相对路径。
- `bash -n scripts/offline-install-drill.sh` 通过；`scripts/scan-sensitive-fields.sh` clean。

## Risks / Notes

- 预加载镜像依赖（`pull_policy: never` + 先 `docker load images/*.tar`）为既有设计：
  离线分发的正确顺序是 加载镜像 -> `docker compose up`。本次修复消除的是**源码绝对路径
  依赖**（迁移文件已随包分发）。
- 跨 digest 升级/回滚演练仍以 compose（非离线包）形式验证；后续可扩展为从离线包做
  升级回滚演练。
