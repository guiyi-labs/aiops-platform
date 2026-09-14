# 前端依赖组升级（Dependabot #26）：Vue 补丁与测试工具链同步

- Date: 2026-09-14
- Status: Complete
- Scope: 合入 Dependabot 分组 PR #26，升级 `frontend/` 下 5 个依赖，均属补丁/次版本跨度。

## Context

Dependabot 在 2026-09-14 打开 PR #26（`frontend-packages` 分组），一次性提出
`frontend/` 目录下 5 个依赖的版本升级。该 PR 的其余 CI 作业（Backend、
Backend race、Frontend、Compose runtime 及各 drill）全部通过，唯独
`Change-record archive gate` 失败——因为 Dependabot 只改锁文件与清单，不会附带
`docs/changes/` 归档件，而本仓库 AGENTS.md §1 铁律要求：改动含非文档代码文件时，
同一次改动必须带 change-record。

本记录即为该 PR 补的归档件，使 PR 能按仓库规则合入，而不是绕过门禁强合。

## What Changed

### frontend（依赖版本）

- `frontend/package.json` / 锁文件：以下 5 处升级（均为补丁或次版本跨度）
  - `vue` 3.5.40 → 3.5.42（补丁）
  - `@axe-core/playwright` 4.12.1 → 4.13.0（次版本）
  - `@playwright/test` 1.62.1 → 1.63.0（次版本）
  - `typescript-eslint` 8.65.0 → 8.70.0（次版本）
  - `vue-tsc` 3.3.8 → 3.3.11（补丁）

- 本记录 `docs/changes/2026-09-14-dep-refresh-frontend-packages.md`：新增。

未涉及任何业务源码改动，应用行为预期不变。

## Verification

- PR #26 CI（run 34828529584）：`Backend` pass、`Backend race` pass、
  `Frontend` pass、`Compose runtime` pass、`M96 Gate B` pass、
  `Dependency & supply chain` pass、各项 drill pass。
  唯一失败项为 `Change-record archive gate`（本记录即为其修复）。
- 补记录后重跑 CI：见 PR #26 的 checks（本记录推送后触发）。
- 本地：`bash .git/info/redline-guard.sh audit` → 全库 0 命中。

## Risks / Notes

- 升级跨度为补丁/次版本，无 major 跳跃，回退方式为 revert 本 PR 的 squash 提交。
- `@playwright/test` 与 `@axe-core/playwright` 的次版本升级可能影响 E2E 与
  可访问性快照；CI 中 `Frontend` 与 `Compose runtime` 作业已覆盖该路径。
- 同类分组 PR #23（Go modules）另行归档；#25（GitHub Actions 分组）因
  `TestCIWorkflowContractsAreParseableAndBounded` 失败而**未合入**，见
  `docs/changes/2026-09-14-dep-refresh-go-modules.md` 的说明。
