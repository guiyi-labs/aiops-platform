# m102-dual-env-compose-drill：双全新环境一致安装演练（M102 本地轨道）

- Date: 2026-08-12
- Status: Complete
- Scope: 用两套完全隔离的全新 compose 环境安装同一不可变镜像，验证安装、关键旅程、数据持久化、跨 digest 升级/回滚与清理一致性，作为 M102「两套全新环境安装/升级/回滚」的完整本地证据。

## Context

M102 GA 封口要求「两套全新环境执行相同 release manifest 的安装、升级、回滚、备份恢复和关键旅程，结果一致」。CI 单套 kind 生命周期（M97）已证明单环境的安装/升级/回滚；本机网络受限（GitHub/Docker Hub 不可达）、无 kind/helm/kubectl 二进制，无法在 CI 触发第二套环境。因此用离线可复现的 compose 布局在本机建立「第二套全新环境一致性」证据，不依赖外部网络，为 M102 汇合积累确定性增量。

## What Changed

- `scripts/dual-env-compose-drill.sh`：M102 本地轨道双环境演练。
  - 每次运行生成两套完全隔离的环境（独立 project name、postgres 端口 25432/26432、backend 端口 28080/29080、frontend 端口 28081/29081、独立 postgres volume 与 compose network），与正在运行的开发 compose 栈（15432/8080/18080）无共享状态。
  - 每套执行 install → backend `health/ready` + frontend 200 → admin 登录 + `/api/v1/auth/me` 断言 `system_admin` → 写入确定性的 `audit_logs` 标记 → `--force-recreate` 到同一不可变镜像后验证标记持久（count=1）→ `down -v` 完整清理。
  - 报告落 `.artifacts/dual-env-compose-drill/report-<run>.json`（schema `aiops.dual-env-compose-drill/v1`），记录三个不可变镜像 digest。幂等可重复运行。
  - 与既有的 `k8s-aiops-*` compose 栈完全隔离：仅用本地已有镜像 `k8s-aiops-backend:latest` / `k8s-aiops-frontend:latest` / `pgvector/pgvector:0.8.1-pg17`，离线可复现。

## Verification

- `./scripts/dual-env-compose-drill.sh`：10/10 PASS（环境 a 与 b 各 5 项：install、key_journey、data_durability、cleanup），报告 `report-20260812-213243-bf91f0.json` 确认。
  - 两套环境 backend、frontend、postgres 各自与运行中开发栈完全隔离；结束后无残留容器/卷/网络。
- `bash -n scripts/dual-env-compose-drill.sh`：语法通过。
- `scripts/scan-sensitive-fields.sh`：clean（1187 tracked files）。
- 证据：`.artifacts/dual-env-compose-drill/report-20260812-213243-bf91f0.json`（已 gitignore，留在本地）。

## Risks / Notes

- 本演练用同一不可变 digest 做「两套全新环境」一致安装与关键旅程/持久化；跨 digest 的升级/回滚一致性仍由 CI release 生命周期（M97）与两套 kind 环境覆盖，需在授权/网络恢复后在组织环境补齐。
- M102 仍要求 Gate A–D、两次全新环境演练、零未解释 critical、M89/M90 完成；当前 M89/M90 仍为组织授权阻塞，版本保持 RC，不宣称 GA。
- 演练约定为本地 offline evidence，不作为生产安装声明。


## v2 更新（2026-08-12，跨 digest 升级/回滚）

- 扩展 `scripts/dual-env-compose-drill.sh`：新增 `APP_UPGRADE_BACKEND_IMAGE` 可选输入。当设置一个与基线不同 digest 的后端镜像时，每套环境在「安装→关键旅程→数据持久化→清理」之外额外执行**跨 digest 升级**（version 变更 + audit 标记保持）与**回滚**（回到基线 digest，version 还原 + 标记保持），并在报告中记录升级镜像 digest。
- 新增宿主机离线构建第二个 arm64 后端镜像（本地源码 + `VERSION=v0.3.0-rc.5`，`GOOS=linux GOARCH=arm64`，封入 `k8s-aiops-backend:v0.3.0-rc.5-local`，digest `f0a27bcf…`），与基线 `k8s-aiops-backend:latest`（`2e8a3813…`）构成两个不同不可变产物，绕过 colima 2GiB/2CPU 下 BuildKit 多二进制并行编译的 OOM。该镜像为本地演练产物，不入库。

### Verification（v2）

- `./scripts/dual-env-compose-drill.sh`（无升级镜像）：10/10 PASS，报告 `report-20260812-213243-bf91f0.json`。
- `APP_UPGRADE_BACKEND_IMAGE=k8s-aiops-backend:v0.3.0-rc.5-local ./scripts/dual-env-compose-drill.sh`：14/14 PASS（环境 a/b 各 7 项），报告 `report-20260812-214043-17a17c.json`；两端均确认 version `dev → v0.3.0-rc.5 → dev`、audit 标记各阶段保持 count=1。

### Risks / Notes（v2）

- 本演练在本地两套隔离环境补齐「跨 digest 升级/回滚 + 数据持久化」的一致性证据；真实组织 kind/Helm 安装、升级、回滚与备份恢复仍需在授权/网络恢复后由 CI 或组织环境执行。
- `k8s-aiops-backend:v0.3.0-rc.5-local` 与本机顶层临时 `/tmp/dual-rc5` Dockerfile 为演练产物，不入库；复跑需先离线重建或从 CI 拉取正式镜像。


## v3 更新（2026-08-12，逻辑备份/恢复）

- `scripts/dual-env-compose-drill.sh`：新增 `APP_BACKUP_RESTORE=1` 可选开关。开启后在环境 A 的数据持久化检查后做**逻辑备份**（`pg_dump`，记录字节数），再在**第三套全新环境**（`aiops-dual-recover`，独立 project/端口 27432/27080/27081/卷/网络、全新空库不含 initdb 预种子）还原备份，并断言 audit 标记 count=1 与登录后 `/me → system_admin` 关键旅程。
- 修复：恢复目标必须是全新空库（关闭 initdb 预种子挂载，否则 `pg_dump` 完整备份与预建表冲突报 `relation "audit_logs" already exists`）。
- report 新增 `backup_restore_enabled`、`backup`、`restore` 字段。

### Verification（v3）

- `APP_UPGRADE_BACKEND_IMAGE=k8s-aiops-backend:v0.3.0-rc.5-local APP_BACKUP_RESTORE=1 ./scripts/dual-env-compose-drill.sh`：16/16 PASS（环境 a/b 各 7 项 + 备份 + 第三环境恢复），报告 `report-20260812-215101-1fcddd.json`；备份 244169 字节，恢复环境 marker count=1 且登录+system_admin 通过。
- 运行后无残留容器/卷/网络；`go test ./...` 通过、敏感字段扫描 clean。

### Risks / Notes（v3）

- 备份为 `pg_dump` 逻辑备份（与 M20 Phase 8 独立防线一致）；真实组织环境的完整备份恢复与 WAL/PITR（M90 授权轨）仍需组织环境补齐。
