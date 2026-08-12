# M102 离线演练闭环：安装/加载改用已发布产物

- Date: 2026-08-12
- Status: Complete
- Scope: 修复离线演练证据缺口——加载与安装必须使用发布的离线包，而非临时组装目录

## Context

上一轮把迁移文件纳入离线包后，演练流程仍存在证据缺口：`docker load` 与
`compose up` 使用临时组装目录 `$BUNDLE`（`$WORK/...`），而发布的"可复用离线包"
`$BUNDLE_STABLE`（`.artifacts/offline-install-drill/bundle/...`）只被 SHA256 校验，
从未被真实加载并安装。这使"可复用离线安装包"声明缺少"发布产物可安装"的直接证据。
本回合把加载与安装环节切到发布产物，形成 组装 → 校验 → 发布 → 加载发布产物 → 安装发布产物 的完整闭环。

## What Changed

- `scripts/offline-install-drill.sh`：
  - publish 完成后 `COMPOSE` 重指向 `$BUNDLE_STABLE/deploy/compose.offline.yaml`。
  - `docker load` 遍历 `$BUNDLE_STABLE/images/*.tar`（发布产物）。
  - 后续 install / key journey / durability / cleanup 全部使用发布产物的 compose 文件。
  - scenario 标题更新为 "Load images from published bundle"、"Install from published bundle"。

## Verification

- `scripts/offline-install-drill.sh`（`v0.3.0-rc.5-replay`）：10/10 PASS，
  报告 `.artifacts/offline-install-drill/report-20260812-230945-4bb47d.json`：
  bundle 7 文件（含 migrations）、SHA256 校验 OK、加载发布产物镜像 digest 不变、
  发布产物全新安装 ready、关键旅程、数据持久化跨 recreate、清理无残留。
- `bash -n scripts/offline-install-drill.sh` 通过；`scripts/scan-sensitive-fields.sh` clean。

## Risks / Notes

- 发布产物与组装目录内容一致（copytree），本改动消除的是"安装来源"的语义缺口，
  不改变产物内容。
- 后续可选增量：从离线包（而非本地镜像 tag）直接做跨 digest 升级/回滚演练。
