# M102 双环境演练刷新：rc.5-replay 跨 digest 升级/回滚/备份恢复证据

- Date: 2026-08-12
- Status: Complete
- Scope: 用 rc.4 基线 + rc.5-replay 升级目标重跑 dual-env 演练，补齐当前产物的升级/回滚/恢复证据

## Context

`v0.3.0-rc.5-replay` 已具备全新安装证据（offline-install-drill 10/10）与演示闭环证据
（demo-drill 17/17），但尚未走过升级/回滚/备份恢复路径。既有 dual-env 演练证据（10/14/16）
使用的是 rc.4→rc.5-local 组合。本回合用「rc.4 基线 → 升级到 rc.5-replay → 回滚 rc.4」+
备份恢复到第三全新环境，使当前产物补齐 M102「安装、升级、回滚、备份恢复、关键旅程」全链路。

## What Changed

- 无代码/脚本改动；`scripts/dual-env-compose-drill.sh` 保持原样，仅更换输入镜像组合复跑。
- 运行参数：`APP_BACKEND_IMAGE=k8s-aiops-backend:v0.3.0-rc.4`、
  `APP_FRONTEND_IMAGE=k8s-aiops-frontend:v0.3.0-rc.4`、
  `APP_UPGRADE_BACKEND_IMAGE=k8s-aiops-backend:v0.3.0-rc.5-replay`、
  `APP_BACKUP_RESTORE=1`。

## Verification

- `scripts/dual-env-compose-drill.sh`：全部 PASS（双全新环境各 7 项 + 备份 1 项 + 恢复 1 项）：
  - 环境 A/B：rc.4 全新安装 → 关键旅程（frontend 200 + admin 登录 → system_admin）→
    审计 marker 跨 recreate 持久化 → 升级 rc.5-replay（version `dev → v0.3.0-rc.5-replay`，
    marker 保持）→ 回滚 rc.4（version 复原，marker 保持）→ `down -v` 清理。
  - 第三全新环境：逻辑备份（244168 bytes）恢复后 marker count=1、登录 + system_admin 正常。
- 报告：`.artifacts/dual-env-compose-drill/report-20260812-225042-e68b90.json`；
  升级目标 digest `sha256:4b69f12e9f6e…`（与 offline 包 `k8s-aiops-backend.tar` 一致）。
- `scripts/scan-sensitive-fields.sh`：clean；工作树提交后干净。

## Risks / Notes

- rc.4 镜像 buildinfo 版本为 `dev`（旧构建未嵌版本号），升级断言以「版本字符串变化 +
  marker 保持」为准，脚本逻辑未变。
- 升级仅交换后端镜像（脚本既有设计）；前端契约向后兼容（replay 为新增只读路由）。
- 推送积压仍被 GitHub 凭据阻塞（本地领先 origin/main 21+ 提交与多个 baseline tag）。
