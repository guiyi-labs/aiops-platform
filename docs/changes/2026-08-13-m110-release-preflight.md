# M110 RC-6 发布预检：本地全项验证通过，release infrastructure 确认就绪

- Date: 2026-08-13
- Status: Complete
- Scope: 在触发真实发布前，逐项验证 M110 Release 所需的本地预检，确保 release workflow 能成功跑完。

## Context

M109 工程卓越收口后进入 M110（RC 刷新 v0.3.0-rc.6）。发布依赖 GitHub Release run（不可逆），需先确认所有本地验证项通过。本项为纯验证，不修改运行时代码。

## What Changed

- 新增 `docs/m110-release-preflight.md`：记录 15 项预检结果（后端编译/测试、前端 typecheck/lint/build、release-manifest 测试、Dockerfile/Helm/kustomize 存在性、迁移自包含、release.yml 质量门自动继承 ci.yml M109 门禁、历史 rc.4 release run 对照）。
- 附带触发方式与发布后必做事项清单。

## Verification

所有预检项已在本地运行并确认通过（见 `docs/m110-release-preflight.md` 表格）。
- `go build ./...` / `go test ./... -short`（后端）
- `pnpm typecheck` / `pnpm lint` / `pnpm build`（前端）
- `node --test scripts/release-manifest.test.mjs`（release 工具）

## Risks / Notes

- 真实发布需用户授权 push `v0.3.0-rc.*` tag；`docs/m110-release-preflight.md` 作为发布前检查单，用户可随时参考。
- Docker Hub 不可达时，release workflow 仍可通过 buildx multi-platform（需 QEMU/Buildx）或走离线重建路径。
