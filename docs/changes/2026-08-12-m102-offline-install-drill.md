# m102-offline-install-drill：离线安装包分发→校验→加载→全新环境安装演练

- Date: 2026-08-12
- Status: Complete
- Scope: M102「允许安装/可复现安装证据」本地轨道——补齐 release 离线包「分发 → 完整性校验 → docker load → 全新隔离环境安装 → 关键旅程 → 清理」全链路的本地确定性证据，安装强制 `pull_policy: never`，证明离线可安装、不依赖网络。

## Context

M97 release 已产出离线包（`aiops-platform-offline-<version>`：images/ + deploy/ + config/ + docs/ + OFFLINE-SHA256SUMS），但此前本地演练（dual-env-compose-drill）只证明「镜像已在本地时全新环境可安装」，未证明「只有离线包（tar + 校验和）时能完成分发→加载→安装」。本演练补齐该链路：模拟异地/隔离传输（SHA256 校验）、从 bundle `docker load`、以 `pull_policy: never` 安装（镜像缺失即失败，反证无需网络），与 M97 离线包布局对齐。

## What Changed

### 新增脚本
- `scripts/offline-install-drill.sh`：9 项断言——
  1. bundle-assemble：`docker save` 三个镜像（backend/frontend/pgvector）到 `images/*.tar`，写 `deploy/compose.offline.yaml`（全部服务 `pull_policy: never`）、`config/env.example`、`docs/release-candidate-operations.md`，生成 `OFFLINE-SHA256SUMS`（6 文件）；
  2. sha256-verify：`shasum -a 256 -c` 全部 OK（模拟空气隔离传输后的完整性校验）；
  3. image-load：逐 tar `docker load` 且加载后镜像 digest 与加载前一致；
  4. install：`docker compose -f deploy/compose.offline.yaml up -d`（`pull_policy: never`，离线反证）→ backend `health/ready`；
  5. key-journey：frontend 200 + admin 登录 + `/me` → `system_admin`；
  6. durability-write / durability-restart：确定性 audit 标记写入 + backend `--force-recreate` 后 count=1；
  7. cleanup：`down -v` 无残留。
- 隔离端口 22432/22080/22081，独立 project/卷/网络，与开发栈（15432/8080/18080）、双环境演练（25432+）、演示演练（21432+）零共享。

## Verification

- `./scripts/offline-install-drill.sh`：**9/9 PASS，连续两轮一致**（`report-20260812-222514-a0fbf8.json`、`report-20260812-222537-4e6c8d.json`）。
- 关键证据：镜像 digest 加载前后一致（backend `2e8a3813…`、frontend `75b1db2d…`、pg `3e8b3adf…`）；bundle 6 文件 SHA256 校验全 OK；`pull_policy: never` 安装成功（镜像缺失会失败，反证离线充分性）；关键旅程 + 持久化 count=1；结束后无残留容器/卷/网络。
- `bash -n scripts/offline-install-drill.sh`：语法通过；`scripts/scan-sensitive-fields.sh`：clean。
- 证据：`.artifacts/offline-install-drill/report-*.json`（已 gitignore，留在本地）。

## Risks / Notes

- bundle 用 `docker save` 普通 tar（非 buildx OCI 格式）；与 release 工作流的 `*-oci.tar` 布局对齐但格式不同，均兼容 `docker load`。真实发布离线包仍以 release 资产为准。
- 演练证明「同一 daemon 内 bundle→load→install」链路与离线充分性；跨主机传输、真实 kind/Helm 生命周期与 M89/M90 授权轨仍需组织环境补齐；未完成前版本保持 RC，不宣称 GA。
