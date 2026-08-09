# 归档体系建立：AGENTS.md + 归档手册 + change-record 模板

- Date: 2026-08-09
- Status: Complete
- Scope: 为仓库建立"所有改动必须归档"的强制规范与工具模板，并还原工作树中的游离改动。

## Context

回顾中发现：前端曾出现大量未提交、未归档的改动（Aurora 主题、组件重构等），因缺
强制归档规范而只能整体回退。为避免再次发生"做完不登记、会话结束即丢失"，落地了
一套强制归档体系。

## What Changed

- 新增 `AGENTS.md`（根目录）：定义"所有改动必须归档"铁律、推荐工作流、提交前
  归档完整性检查表，以及违反后果。对后续所有代码/文档改动生效。
- 新增 `docs/ARCHIVING.md`（归档手册）：四层归档体系（change-record /
  CHANGELOG / ADR / baseline tag）、文件命名、change-record 模板、提交与标签流程、
  完整性检查表、常见场景速记、例外边界。
- 新增 `docs/changes/TEMPLATE.md`：change-record 标准模板，供复制填写。
- 还原游离改动：`frontend/src/App.vue`（路由过渡）、`frontend/src/styles/console-theme.css`
  与 `frontend/src/styles/motion.css` 中移除 UI 动效的内容来源不明、未归档，
  已还原到 HEAD 已交付基线；如需重新回退动效，请先开 PR 并归档。

## Verification

- `git status --porcelain`：仅本次新增文件（未提交）；无其他游离改动。
- 三份新文档均为 UTF-8 无 BOM，中文正常。

## Risks / Notes

- AGENTS.md 会在仓库根目录被所有运行于此目录的 Agent 读取，归档规范即刻生效。
- 已还原的 3 处动效改动若确属业务需要，请以正式提交 + change-record 重新落地。