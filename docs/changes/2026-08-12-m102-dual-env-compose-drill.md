# m102-dual-env-compose-drill：双全新环境一致安装演练（M102 本地轨道）

- Date: 2026-08-12
- Status: Complete
- Scope: 用两套完全隔离的全新 compose 环境安装同一不可变镜像（backend/frontend/postgres digest），验证安装、关键旅程、数据持久化与清理一致性，作为 M102「两套全新环境安装/升级/回滚」的第一道本地证据。

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
