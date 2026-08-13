# Roadmap Post-M106：后续开发路线规划（前端优化并行轨 + M107–M110 主线 + 授权轨）

- Date: 2026-08-13
- Status: Complete
- Scope: 新增 `docs/development-roadmap-post-m106.md`，规划 M106 之后的前端优化并行轨、事故协作/关联归一/工程收口/RC 刷新主线，以及 M89/M90 授权轨

## Context

用户提出另一 Agent 正在优化前端界面，要求规划后续开发路线。M106（本地体验重构）已收口，
M102 及之前的执行计划（`docs/next-long-term-plan.md`）已过时，需要一份 M106 之后的路线，
明确并行轨衔接契约、主线里程碑与外部授权轨的编排。

## What Changed

- `docs/development-roadmap-post-m106.md`：新增路线文档，包含——
  - 并行轨 A（前端优化）：主题收敛、截图基线、响应式审计、交互统一、性能预算；衔接契约
    （门禁、只改 frontend、OpenAPI 变更登记、归档要求、与 M107 的 IncidentsView 冲突规避）。
  - 主线 M107 事故协作闭环：复盘视图、SLA 仪表、交接与关注者、五源统一证据时间线。
  - 主线 M108 关联归一：correlation → incident 第 6 来源、case_key 防风暴去重、双向深链、
    demo-drill 演练。
  - 主线 M109 工程卓越：性能门禁 fail-closed、incident 旅程 E2E、覆盖率 65%、fuzz 扩展。
  - 主线 M110 RC-6 刷新：双架构镜像、离线包、SBOM/签名、全新环境演练。
  - 授权轨 M89/M90（Deferred）与 GA Gate D；编排与门禁总表。

## Verification

- `git status --porcelain` 干净（仅本次文档改动待归档）。
- 路线与 `docs/long-term-roadmap.md`、`docs/polish-plan.md`、`docs/authorization-gate-prep.md`
  对齐，未推翻既有架构决策。

## Risks / Notes

- 规划为指引性文档，里程碑编号/范围在实施时可随实际进度调整，但须保持归档与基线节奏。
- 前端并行轨与 M107 在 `IncidentsView`/`console-theme.css` 存在潜在接触面，已约定先沟通再改。
